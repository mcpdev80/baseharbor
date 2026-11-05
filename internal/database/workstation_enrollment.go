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
	if b.ExpiresAt.IsZero() || b.CertificateTTL <= 0 || b.CertificateTTL > 24*time.Hour {
		return workstationenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, b.RequestScope.TenantID, func(tx pgx.Tx) error {
		var pending bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workstation_enrollment_grants
WHERE tenant_id=$1 AND station=$2 AND consumed_at IS NULL AND expires_at > clock_timestamp())`,
			b.RequestScope.TenantID, b.RequestScope.Station).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return workstationenrollment.ErrDenied
		}
		return insertWorkstationGrant(ctx, tx, b)
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
	err := WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		var err error
		err = tx.QueryRow(ctx, `UPDATE workstation_enrollment_grants SET consumed_at=clock_timestamp()
WHERE tenant_id=$1 AND station=$2 AND token_digest=$3 AND nonce_digest=$4
AND consumed_at IS NULL AND expires_at > GREATEST($5, clock_timestamp())
RETURNING expires_at, certificate_ttl_seconds`,
			scope.TenantID, scope.Station, workstationenrollment.DigestForConsume(token),
			workstationenrollment.DigestForConsume(nonce), expiresAt).Scan(&expiresAt, &seconds)
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
		return nil
	})
	if err != nil {
		return workstationenrollment.Bootstrap{}, workstationenrollment.ErrDenied
	}
	if bootstrap.CertificateTTL <= 0 || bootstrap.CertificateTTL > 24*time.Hour {
		return workstationenrollment.Bootstrap{}, workstationenrollment.ErrDenied
	}
	if ttl <= 0 || ttl > 24*time.Hour {
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
	if s == nil || s.pool == nil || scope.Validate() != nil || issuedAt.IsZero() || expiresAt.IsZero() {
		return workstationenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `INSERT INTO workstation_enrollment_registry
(tenant_id, station, issued_at, recipient, scope, serial, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, station) DO UPDATE SET
	issued_at = EXCLUDED.issued_at,
	recipient = EXCLUDED.recipient,
	scope = EXCLUDED.scope,
	serial = EXCLUDED.serial,
	expires_at = EXCLUDED.expires_at`,
			scope.TenantID, scope.Station, issuedAt, recipient, scopeID, serial, expiresAt)
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

func insertWorkstationGrant(ctx context.Context, tx pgx.Tx, b workstationenrollment.Bootstrap) error {
	_, err := tx.Exec(ctx, `INSERT INTO workstation_enrollment_grants
(tenant_id, station, token_digest, nonce_digest, expires_at, certificate_ttl_seconds)
VALUES ($1, $2, $3, $4, $5, $6)`,
		b.RequestScope.TenantID, b.RequestScope.Station,
		workstationenrollment.DigestForConsume(b.Token),
		workstationenrollment.DigestForConsume(b.Nonce),
		b.ExpiresAt, int64(b.CertificateTTL/time.Second))
	return err
}
