package database

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

var ErrInvalidPrincipal = errors.New("invalid principal")

// IdentityTenantResolver resolves memberships for one verified external
// identity before a tenant scope exists. Access remains constrained by the
// identity-resolution RLS policy; no BYPASSRLS or superuser privilege is used.
type IdentityTenantResolver struct {
	pool *pgxpool.Pool
}

func NewIdentityTenantResolver(pool *pgxpool.Pool) *IdentityTenantResolver {
	return &IdentityTenantResolver{pool: pool}
}

func (r *IdentityTenantResolver) ResolveTenant(ctx context.Context, principal *identity.Principal) (*tenancy.Context, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool is nil")
	}
	if principal == nil || strings.TrimSpace(principal.Issuer) == "" || strings.TrimSpace(principal.Subject) == "" {
		return nil, ErrInvalidPrincipal
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"SELECT set_config('baseharbor.identity_issuer', $1, true), set_config('baseharbor.identity_subject', $2, true)",
		strings.TrimSpace(principal.Issuer), strings.TrimSpace(principal.Subject),
	); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
SELECT m.external_identity_id::text, m.tenant_id::text, m.role
FROM memberships AS m
JOIN external_identities AS e ON e.id = m.external_identity_id
WHERE e.issuer = $1 AND e.subject = $2
ORDER BY m.tenant_id, m.role
`, strings.TrimSpace(principal.Issuer), strings.TrimSpace(principal.Subject))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var externalIdentityID string
	memberships := make([]tenancy.Membership, 0, 2)
	for rows.Next() {
		var identityID, tenantID, role string
		if err := rows.Scan(&identityID, &tenantID, &role); err != nil {
			return nil, err
		}
		if externalIdentityID == "" {
			externalIdentityID = identityID
		} else if identityID != externalIdentityID {
			return nil, tenancy.ErrInvalidMembership
		}
		memberships = append(memberships, tenancy.Membership{TenantID: tenantID, Role: role})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, tenancy.ErrNoMembership
	}

	resolved, err := tenancy.Resolve(externalIdentityID, memberships)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return resolved, nil
}

// ResolveClaimMembership returns current memberships whose roles are present in
// the verified principal's group or app-role claims. The identity-scoped
// transaction reads the authoritative membership rows on each call, so a
// removed membership takes effect on the next authorization decision.
func (r *IdentityTenantResolver) ResolveClaimMembership(ctx context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool is nil")
	}
	if principal == nil || !principal.IssuerVerified || strings.TrimSpace(principal.Issuer) == "" || strings.TrimSpace(principal.Subject) == "" {
		return nil, ErrInvalidPrincipal
	}

	claimRoles := make([]string, 0, len(principal.ClaimSets()))
	for _, claim := range principal.ClaimSets() {
		role := ""
		switch {
		case strings.HasPrefix(claim, "group:"):
			role = strings.TrimPrefix(claim, "group:")
		case strings.HasPrefix(claim, "app-role:"):
			role = strings.TrimPrefix(claim, "app-role:")
		}
		if role != "" {
			claimRoles = append(claimRoles, role)
		}
	}
	if len(claimRoles) == 0 {
		return nil, tenancy.ErrNoMembership
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		"SELECT set_config('baseharbor.identity_issuer', $1, true), set_config('baseharbor.identity_subject', $2, true)",
		strings.TrimSpace(principal.Issuer), strings.TrimSpace(principal.Subject),
	); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
SELECT m.tenant_id::text, m.role
FROM memberships AS m
JOIN external_identities AS e ON e.id = m.external_identity_id
WHERE e.issuer = $1 AND e.subject = $2 AND lower(m.role) = ANY($3::text[])
ORDER BY m.tenant_id, m.role
`, strings.TrimSpace(principal.Issuer), strings.TrimSpace(principal.Subject), claimRoles)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := make([]tenancy.Membership, 0, len(claimRoles))
	for rows.Next() {
		var tenantID, role string
		if err := rows.Scan(&tenantID, &role); err != nil {
			return nil, err
		}
		memberships = append(memberships, tenancy.Membership{TenantID: tenantID, Role: role})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, tenancy.ErrNoMembership
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return memberships, nil
}
