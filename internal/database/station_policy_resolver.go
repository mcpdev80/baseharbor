package database

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// StationPolicyResolver authorizes a workstation-owned station label against the
// authenticated operator's installation. It answers only "is this station
// permitted for this tenant?" — it never derives the installation from the
// request. Authorization requires both an authenticated membership in the
// installation tenant and a known (owned) station for that tenant: a valid-looking
// but unowned station, or a foreign installation, fails closed.
//
// This is distinct from internal/targetenrollment and never touches connector or
// node tables; it only reads tenant-isolated memberships and the
// workstation_enrollment_registry behind RLS. The tenant is bound by the resolver
// itself via a tenant-scoped transaction, so the caller must still carry the
// authenticated identity (tenancy) — the resolver never trusts the request to
// select the installation.
type StationPolicyResolver struct {
	pool *pgxpool.Pool
}

// NewStationPolicyResolver returns a StationPolicyResolver over the given
// tenant-scoped connection pool.
func NewStationPolicyResolver(pool *pgxpool.Pool) *StationPolicyResolver {
	return &StationPolicyResolver{pool: pool}
}

// AuthorizeStation reports whether the workstation-owned station label is
// authorized for the authenticated installation identified by tenantID. It binds
// the tenant itself with a read-only, tenant-scoped transaction so RLS filters to
// exactly one installation.
//
// Authorization has two valid sources, both tenant-scoped and RLS-filtered by the
// transaction:
//
//	(a) a registry fact in workstation_enrollment_registry persisting the approved
//	    installation/station owner (the ongoing source after enrollment), or
//	(b) a valid, not-yet-consumed, unexpired one-use bootstrap grant in
//	    workstation_enrollment_grants (migration 0009's "one-use bootstrap
//	    authorization"). The grant source authorizes the very first bootstrap on a
//	    clean installation, when no registry row can yet exist; once enrollment
//	    succeeds the registry fact becomes the ongoing source.
//
// A valid-looking but unowned station has neither a registry row nor a live grant
// and fails closed. A foreign installation (no row for tenantID) fails closed the
// same way. The membership boundary is enforced upstream by the authenticated
// tenancy that supplies tenantID; this resolver never trusts the request to
// select the installation.
func (r *StationPolicyResolver) AuthorizeStation(ctx context.Context, tenantID, station string) error {
	if r == nil || r.pool == nil {
		return errors.New("station policy resolver pool is nil")
	}
	if tenantID == "" || strings.TrimSpace(station) == "" {
		return tenancy.ErrInvalidMembership
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"SELECT set_config('baseharbor.tenant_id', $1, true)",
		tenantID,
	); err != nil {
		return err
	}

	// Ownership: the station must be authorized for the installation. Authorization
	// has two valid sources, both tenant-scoped and RLS-filtered by the tx:
	//
	//  (a) a registry fact persisting the approved installation/station owner, or
	//  (b) a valid, not-yet-consumed, unexpired one-use bootstrap grant in
	//      workstation_enrollment_grants (the migration-0009 "one-use bootstrap
	//      authorization"). The grant source authorizes the very first bootstrap on
	//      a clean installation, when no registry row can yet exist, and then the
	//      registry fact becomes the ongoing source after enrollment succeeds.
	//
	// A valid-looking but unowned station has neither a registry row nor a live
	// grant and fails closed here, before the store or issuer is touched. A foreign
	// installation (no row for tenantID) fails closed the same way.
	var owned int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM
(SELECT 1 FROM workstation_enrollment_registry
 WHERE tenant_id = $1 AND station = $2
 UNION
 SELECT 1 FROM workstation_enrollment_grants
 WHERE tenant_id = $1 AND station = $2
   AND consumed_at IS NULL
   AND expires_at > clock_timestamp()) auth`,
		tenantID, station).Scan(&owned); err != nil {
		return err
	}
	if owned == 0 {
		return tenancy.ErrNoMembership
	}

	return tx.Commit(ctx)
}
