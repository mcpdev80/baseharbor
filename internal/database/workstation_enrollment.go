package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// WorkstationEnrollmentStore persists one-use workstation-enrollment bootstrap
// grants and the approved registry owner behind a durable, tenant-scoped
// PostgreSQL store. It implements internal/workstationenrollment.Store for the
// production path: consumption is persisted before success, so a restart cannot
// replay a consumed one-use bootstrap (fail-closed), and the registry row
// persists the installation-scoped scope, the issuance time, and the separate
// workstation SPIFFE identity as a fact.
//
// This is a distinct namespace from internal/targetenrollment: the workstation
// tables (workstation_enrollment_grants, workstation_enrollment_registry) are
// owned here and never share a row, index, or policy with the connector tables.
type WorkstationEnrollmentStore struct{ pool *pgxpool.Pool }

// WorkstationRegistryFact is the non-secret inventory of one enrolled
// workstation. Serial numbers are intentionally excluded from CLI output.
type WorkstationRegistryFact struct {
	Station    string
	OwnerLogin string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}

// WorkstationUser is the non-secret directory projection linked to the owning
// Keycloak account. Authentication remains owned by Keycloak.
type WorkstationUser struct {
	Login        string
	DisplayName  string
	Email        string
	KeycloakID   string
	Status       string
	CreatedAt    time.Time
	OffboardedAt *time.Time
}

// NewWorkstationEnrollmentStore returns a durable workstation-enrollment store
// over the given tenant-scoped connection pool.
func NewWorkstationEnrollmentStore(pool *pgxpool.Pool) *WorkstationEnrollmentStore {
	return &WorkstationEnrollmentStore{pool: pool}
}

var _ workstationenrollment.Store = (*WorkstationEnrollmentStore)(nil)

// Create persists a one-use workstation bootstrap authorization for a valid
// scope. A valid, unexpired, unconsumed grant already outstanding for the same
// installation/station fails closed (a workstation must present exactly one
// live bootstrap), so this rejects the duplicate before persisting.
func (s *WorkstationEnrollmentStore) Create(ctx context.Context, b workstationenrollment.Bootstrap) error {
	if s == nil || s.pool == nil || b.RequestScope.Validate() != nil {
		return workstationenrollment.ErrDenied
	}
	if b.ExpiresAt.IsZero() || b.CertificateTTL <= 0 || b.CertificateTTL > workstationenrollment.MaxCertificateTTL {
		return workstationenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, b.RequestScope.TenantID, func(tx pgx.Tx) error {
		var occupied bool
		if err := tx.QueryRow(ctx, `SELECT
EXISTS(SELECT 1 FROM workstation_enrollment_grants WHERE tenant_id=$1 AND station=$2 AND consumed_at IS NULL AND expires_at > clock_timestamp())
OR EXISTS(SELECT 1 FROM workstation_enrollment_registry WHERE tenant_id=$1 AND station=$2)`,
			b.RequestScope.TenantID, b.RequestScope.Station).Scan(&occupied); err != nil {
			return err
		}
		if occupied {
			return workstationenrollment.ErrDenied
		}
		if b.OwnerLogin != "" {
			var status string
			if err := tx.QueryRow(ctx, `SELECT status FROM workstation_users WHERE tenant_id=$1 AND login=$2 FOR UPDATE`, b.RequestScope.TenantID, b.OwnerLogin).Scan(&status); err != nil || status != "active" {
				return workstationenrollment.ErrDenied
			}
		}
		return insertWorkstationGrant(ctx, tx, b)
	})
}

// EnsureWorkstationNameAvailable preflights a device label before Keycloak
// account creation. Create repeats this check transactionally before grant minting.
func (s *WorkstationEnrollmentStore) EnsureWorkstationNameAvailable(ctx context.Context, scope workstationenrollment.Scope) error {
	if s == nil || s.pool == nil || scope.Validate() != nil {
		return workstationenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		var occupied bool
		if err := tx.QueryRow(ctx, `SELECT
EXISTS(SELECT 1 FROM workstation_enrollment_grants WHERE tenant_id=$1 AND station=$2 AND consumed_at IS NULL AND expires_at > clock_timestamp())
OR EXISTS(SELECT 1 FROM workstation_enrollment_registry WHERE tenant_id=$1 AND station=$2)`, scope.TenantID, scope.Station).Scan(&occupied); err != nil {
			return err
		}
		if occupied {
			return workstationenrollment.ErrDenied
		}
		return nil
	})
}

// Consume returns the stored one-use authorization exactly once, then marks it
// consumed in the same tenant-local transaction before returning. A missing,
// foreign, already-consumed, or already-expired grant fails closed. The ttl is
// the requested certificate TTL used to reject a grant created with a disallowed
// window. Because consumption is persisted atomically, a reopened store (process
// restart) cannot spend the same bootstrap twice.
func (s *WorkstationEnrollmentStore) Consume(ctx context.Context, scope workstationenrollment.Scope, token, nonce string, ttl time.Duration) (workstationenrollment.Bootstrap, error) {
	bootstrap := workstationenrollment.Bootstrap{RequestScope: scope}
	if s == nil || s.pool == nil || scope.Validate() != nil {
		return bootstrap, workstationenrollment.ErrDenied
	}
	var expiresAt time.Time
	var seconds int64
	var ownerLogin *string
	err := WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		var err error
		err = tx.QueryRow(ctx, `UPDATE workstation_enrollment_grants SET consumed_at=clock_timestamp()
WHERE tenant_id=$1 AND station=$2 AND token_digest=$3 AND nonce_digest=$4
AND consumed_at IS NULL AND expires_at > GREATEST($5, clock_timestamp())
RETURNING expires_at, certificate_ttl_seconds, owner_login`,
			scope.TenantID, scope.Station, workstationenrollment.DigestForConsume(token),
			workstationenrollment.DigestForConsume(nonce), expiresAt).Scan(&expiresAt, &seconds, &ownerLogin)
		if errors.Is(err, pgx.ErrNoRows) {
			return workstationenrollment.ErrDenied
		}
		if err != nil {
			return err
		}
		bootstrap.Token = token
		bootstrap.Nonce = nonce
		bootstrap.ExpiresAt = expiresAt
		bootstrap.CertificateTTL = time.Duration(seconds) * time.Second
		if ownerLogin != nil {
			bootstrap.OwnerLogin = *ownerLogin
		}
		return nil
	})
	if err != nil {
		return workstationenrollment.Bootstrap{}, workstationenrollment.ErrDenied
	}
	if bootstrap.CertificateTTL <= 0 || bootstrap.CertificateTTL > workstationenrollment.MaxCertificateTTL {
		return workstationenrollment.Bootstrap{}, workstationenrollment.ErrDenied
	}
	if ttl <= 0 || ttl > workstationenrollment.MaxCertificateTTL {
		return workstationenrollment.Bootstrap{}, workstationenrollment.ErrDenied
	}
	return bootstrap, nil
}

// RecordRegistryFact persists the approved registry owner as a fact: the
// installation-scoped scope, the issuance time, and the separate workstation
// SPIFFE identity issued by the enrollment (bound to the serial + expiry the
// issuer reported). It overwrites the prior registry row for the same
// installation/station, so the latest issued identity is authoritative.
func (s *WorkstationEnrollmentStore) RecordRegistryFact(ctx context.Context, scope workstationenrollment.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error {
	return s.RecordOwnedRegistryFact(ctx, scope, issuedAt, recipient, scopeID, serial, expiresAt, "")
}

// RecordOwnedRegistryFact persists an issued leaf and its person association.
// Empty owner information during renewal preserves the stored association.
func (s *WorkstationEnrollmentStore) RecordOwnedRegistryFact(ctx context.Context, scope workstationenrollment.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time, ownerLogin string) error {
	if s == nil || s.pool == nil || scope.Validate() != nil || issuedAt.IsZero() || expiresAt.IsZero() {
		return workstationenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		boundLogin := ownerLogin
		if boundLogin == "" {
			// A renewal must retain the previously bound owner and must not
			// revive a certificate after that person is offboarded.
			_ = tx.QueryRow(ctx, `SELECT COALESCE(owner_login,'') FROM workstation_enrollment_registry WHERE tenant_id=$1 AND station=$2 FOR UPDATE`, scope.TenantID, scope.Station).Scan(&boundLogin)
		}
		if boundLogin != "" {
			var status string
			if err := tx.QueryRow(ctx, `SELECT status FROM workstation_users WHERE tenant_id=$1 AND login=$2 FOR UPDATE`, scope.TenantID, boundLogin).Scan(&status); err != nil || status != "active" {
				return workstationenrollment.ErrDenied
			}
		}
		command, err := tx.Exec(ctx, `INSERT INTO workstation_enrollment_registry
(tenant_id, station, issued_at, recipient, scope, serial, expires_at, owner_login)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, station) DO UPDATE SET
	issued_at = EXCLUDED.issued_at,
	recipient = EXCLUDED.recipient,
	scope = EXCLUDED.scope,
	serial = EXCLUDED.serial,
	expires_at = EXCLUDED.expires_at,
	owner_login = COALESCE(EXCLUDED.owner_login, workstation_enrollment_registry.owner_login)`,
			scope.TenantID, scope.Station, issuedAt, recipient, scopeID, serial, expiresAt, nullableString(ownerLogin))
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return workstationenrollment.ErrDenied
		}
		return nil
	})
}

// ReadRegistryFact returns the persisted registry fact for a scope, or a
// missing/failed read fails closed. A registry owner that never enrolled (or
// whose row was rolled back) fails closed, so a caller can gate on it before
// trusting an installed workstation.
func (s *WorkstationEnrollmentStore) ReadRegistryFact(ctx context.Context, scope workstationenrollment.Scope) (issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time, ok bool, err error) {
	if s == nil || s.pool == nil || scope.Validate() != nil {
		return time.Time{}, "", "", "", time.Time{}, false, workstationenrollment.ErrDenied
	}
	err = WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		rowErr := tx.QueryRow(ctx, `SELECT issued_at, recipient, scope, serial, expires_at
FROM workstation_enrollment_registry WHERE tenant_id=$1 AND station=$2`,
			scope.TenantID, scope.Station).Scan(&issuedAt, &recipient, &scopeID, &serial, &expiresAt)
		if rowErr == nil {
			ok = true
			return nil
		}
		if errors.Is(rowErr, pgx.ErrNoRows) {
			return workstationenrollment.ErrDenied
		}
		return rowErr
	})
	if err != nil {
		return time.Time{}, "", "", "", time.Time{}, false, workstationenrollment.ErrDenied
	}
	return issuedAt, recipient, scopeID, serial, expiresAt, true, nil
}

// ListRegistryFacts returns every workstation registered to one installation.
// The tenant transaction preserves the table's row-level isolation boundary.
func (s *WorkstationEnrollmentStore) ListRegistryFacts(ctx context.Context, tenantID string) ([]WorkstationRegistryFact, error) {
	if s == nil || s.pool == nil || (workstationenrollment.Scope{TenantID: tenantID, Station: "validation"}).Validate() != nil {
		return nil, workstationenrollment.ErrDenied
	}
	var facts []WorkstationRegistryFact
	err := WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT station, COALESCE(owner_login,''), issued_at, expires_at
FROM workstation_enrollment_registry WHERE tenant_id=$1 ORDER BY station`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var fact WorkstationRegistryFact
			if err := rows.Scan(&fact.Station, &fact.OwnerLogin, &fact.IssuedAt, &fact.ExpiresAt); err != nil {
				return err
			}
			facts = append(facts, fact)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, workstationenrollment.ErrDenied
	}
	return facts, nil
}

// CurrentSerial implements the workstation-enrollment Owner seam for the device
// lifecycle. It returns the serial of the leaf currently authoritative for the
// scope by reading the durable registry fact; a missing row or a failed read
// fails closed (ok=false, err != nil), so a lifecycle operation on a workstation
// that never enrolled cannot touch anything. This is the read half that pairs
// with RecordRegistryFact: enrollment and a successful renewal write the current
// serial, and Renew/Revoke read it back to anchor exactly that leaf.
func (s *WorkstationEnrollmentStore) CurrentSerial(ctx context.Context, scope workstationenrollment.Scope) (string, bool, error) {
	_, _, _, serial, _, ok, err := s.ReadRegistryFact(ctx, scope)
	return serial, ok, err
}

func insertWorkstationGrant(ctx context.Context, tx pgx.Tx, b workstationenrollment.Bootstrap) error {
	_, err := tx.Exec(ctx, `INSERT INTO workstation_enrollment_grants
(tenant_id, station, token_digest, nonce_digest, expires_at, certificate_ttl_seconds, owner_login)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		b.RequestScope.TenantID, b.RequestScope.Station,
		workstationenrollment.DigestForConsume(b.Token),
		workstationenrollment.DigestForConsume(b.Nonce),
		b.ExpiresAt, int64(b.CertificateTTL/time.Second), nullableString(b.OwnerLogin))
	return err
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
