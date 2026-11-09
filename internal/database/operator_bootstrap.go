package database

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

var (
	ErrInitialOperatorUnverified  = errors.New("initial operator requires a verified, unexpired OIDC principal")
	ErrInitialTenantExists        = errors.New("initial tenant already belongs to an existing installation")
	ErrInitialTenantOwnerMismatch = errors.New("initial tenant is not owned by the authenticated operator")
)

var initialTenantSlug = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// InitialOperatorTenant identifies the installation's first tenant and
// authenticated operator. It contains no token or provider-admin secrets.
type InitialOperatorTenant struct {
	TenantID           string
	ExternalIdentityID string
	Created            bool
}

// ValidateInitialOperatorBootstrap rejects unverified or expired principals.
// The caller must independently prove local installation ownership before
// giving this operation access to the privileged bootstrap database pool.
func ValidateInitialOperatorBootstrap(principal *identity.Principal, slug, name string, now time.Time) error {
	if principal == nil || !principal.IssuerVerified ||
		principal.Issuer == "" || principal.Subject == "" ||
		strings.TrimSpace(principal.Issuer) != principal.Issuer ||
		strings.TrimSpace(principal.Subject) != principal.Subject {
		return ErrInitialOperatorUnverified
	}
	u, err := url.Parse(principal.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Fragment != "" || u.RawQuery != "" {
		return ErrInitialOperatorUnverified
	}
	if principal.ExpiresAt == nil || !principal.ExpiresAt.After(now) {
		return ErrInitialOperatorUnverified
	}
	if len(slug) > 63 || !initialTenantSlug.MatchString(slug) {
		return errors.New("initial tenant slug must be a lowercase DNS-style name between 2 and 63 characters")
	}
	if strings.TrimSpace(name) != name || len(name) == 0 || len(name) > 120 {
		return errors.New("initial tenant name must contain 1 to 120 characters without surrounding whitespace")
	}
	return nil
}

// BootstrapInitialOperatorTenant atomically creates the first tenant and
// its verified editor membership. Only an installation-owned bootstrap
// connection is permitted: never expose this as an unauthenticated API or use
// the normal runtime/HTTP database principal. Repeated calls with the same
// owner are idempotent; competing or foreign initialization fails closed.
func BootstrapInitialOperatorTenant(ctx context.Context, bootstrapDB *pgxpool.Pool, principal *identity.Principal, slug, name string) (InitialOperatorTenant, error) {
	if err := ValidateInitialOperatorBootstrap(principal, slug, name, time.Now()); err != nil {
		return InitialOperatorTenant{}, err
	}
	if bootstrapDB == nil {
		return InitialOperatorTenant{}, errors.New("initial operator bootstrap database is missing")
	}
	tx, err := bootstrapDB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("begin initial operator bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(4776434171114316877)); err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("lock initial operator bootstrap: %w", err)
	}
	var tenantID string
	err = tx.QueryRow(ctx, "SELECT id::text FROM tenants WHERE slug = $1", slug).Scan(&tenantID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return InitialOperatorTenant{}, fmt.Errorf("inspect initial tenant: %w", err)
	}
	if err == nil {
		if _, err = tx.Exec(ctx, "SELECT set_config('baseharbor.tenant_id', $1, true)", tenantID); err != nil {
			return InitialOperatorTenant{}, err
		}
		var externalID string
		err = tx.QueryRow(ctx, `
SELECT e.id::text
FROM external_identities e
JOIN memberships m ON m.external_identity_id = e.id
WHERE e.issuer = $1 AND e.subject = $2
  AND m.tenant_id = $3::uuid AND m.role = 'editor'
`, principal.Issuer, principal.Subject, tenantID).Scan(&externalID)
		if errors.Is(err, pgx.ErrNoRows) {
			// A locally owned Core may have provisioned its tenant identity
			// without an OIDC operator so that CLI-only workstation pairing
			// works first. The first real *verified* OIDC owner may claim it,
			// but only when no existing membership exists anywhere inside
			// the initial tenant. Never transfer an established tenant.
			var count int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM memberships WHERE tenant_id=$1::uuid", tenantID).Scan(&count); err != nil {
				return InitialOperatorTenant{}, err
			}
			var tenants int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM tenants").Scan(&tenants); err != nil {
				return InitialOperatorTenant{}, err
			}
			if count != 0 || tenants != 1 {
				return InitialOperatorTenant{}, ErrInitialTenantOwnerMismatch
			}
			externalID, err = stableid.NewUUIDv4("initial operator")
			if err != nil { return InitialOperatorTenant{}, err }
			memberID, err := stableid.NewUUIDv4("initial membership")
			if err != nil { return InitialOperatorTenant{}, err }
			if _, err := tx.Exec(ctx, `
INSERT INTO external_identities(id, issuer, subject) VALUES ($1,$2,$3)
ON CONFLICT (issuer,subject) DO NOTHING
`, externalID, principal.Issuer, principal.Subject); err != nil {
				return InitialOperatorTenant{}, err
			}
			if err := tx.QueryRow(ctx,
				"SELECT id::text FROM external_identities WHERE issuer=$1 AND subject=$2",
				principal.Issuer, principal.Subject).Scan(&externalID); err != nil {
				return InitialOperatorTenant{}, err
			}
			if _, err := tx.Exec(ctx,
				"INSERT INTO memberships(id,tenant_id,external_identity_id,role) VALUES ($1,$2,$3,'editor')",
				memberID, tenantID, externalID); err != nil {
				return InitialOperatorTenant{}, err
			}
			if err := tx.Commit(ctx); err != nil { return InitialOperatorTenant{}, err }
			return InitialOperatorTenant{TenantID:tenantID, ExternalIdentityID:externalID,Created:false}, nil
		}
		if err != nil {
			return InitialOperatorTenant{}, fmt.Errorf("verify initial tenant owner: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return InitialOperatorTenant{}, err
		}
		return InitialOperatorTenant{TenantID: tenantID, ExternalIdentityID: externalID}, nil
	}

	var existing int
	if err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM tenants").Scan(&existing); err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("inspect installation tenants: %w", err)
	}
	if existing != 0 {
		return InitialOperatorTenant{}, ErrInitialTenantExists
	}

	tenantID, err = stableid.NewUUIDv4("initial tenant")
	if err != nil {
		return InitialOperatorTenant{}, err
	}
	externalID, err := stableid.NewUUIDv4("initial operator")
	if err != nil {
		return InitialOperatorTenant{}, err
	}
	memberID, err := stableid.NewUUIDv4("initial membership")
	if err != nil {
		return InitialOperatorTenant{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO tenants(id, slug, name) VALUES ($1, $2, $3)", tenantID, slug, name); err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("create initial tenant: %w", err)
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('baseharbor.tenant_id', $1, true)", tenantID); err != nil {
		return InitialOperatorTenant{}, err
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO external_identities(id, issuer, subject)
VALUES ($1, $2, $3)
ON CONFLICT (issuer, subject) DO NOTHING
`, externalID, principal.Issuer, principal.Subject); err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("create initial operator identity: %w", err)
	}
	if err = tx.QueryRow(ctx, "SELECT id::text FROM external_identities WHERE issuer = $1 AND subject = $2", principal.Issuer, principal.Subject).Scan(&externalID); err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("resolve initial operator identity: %w", err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO memberships(id, tenant_id, external_identity_id, role) VALUES ($1, $2, $3, 'editor')", memberID, tenantID, externalID); err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("bind initial operator to tenant: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return InitialOperatorTenant{}, fmt.Errorf("commit initial operator bootstrap: %w", err)
	}
	return InitialOperatorTenant{TenantID: tenantID, ExternalIdentityID: externalID, Created: true}, nil
}
