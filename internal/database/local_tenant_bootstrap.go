package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// BootstrapLocalInstallationTenant provisions only the initial tenancy identity
// for a trusted, locally owned Core. It never creates an external identity,
// assigns editor rights, or claims an OIDC login has occurred. Authentication of
// the first browser operator remains a separate OIDC ceremony.
//
// Only a caller that already proved local Core ownership may open the protected
// bootstrap DB pool. The normal HTTP operator DB principal cannot invoke this.
func BootstrapLocalInstallationTenant(ctx context.Context, bootstrapDB *pgxpool.Pool, slug, name string) (string, bool, error) {
	if bootstrapDB == nil {
		return "", false, errors.New("local Core bootstrap database is required")
	}
	if len(slug) > 63 || !initialTenantSlug.MatchString(slug) || name == "" || len(name) > 120 {
		return "", false, errors.New("invalid initial local tenant name")
	}
	tx, err := bootstrapDB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", false, fmt.Errorf("begin local Core tenant bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(4776434171114316877)); err != nil {
		return "", false, err
	}
	var id, existingSlug string
	err = tx.QueryRow(ctx, "SELECT id::text, slug FROM tenants").Scan(&id, &existingSlug)
	if err == nil {
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM tenants").Scan(&count); err != nil {
			return "", false, err
		}
		if count != 1 || existingSlug != slug {
			return "", false, ErrInitialTenantExists
		}
		if err := tx.Commit(ctx); err != nil { return "", false, err }
		return id, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	id, err = stableid.NewUUIDv4("initial local Core tenant")
	if err != nil { return "", false, err }
	if _, err := tx.Exec(ctx, "INSERT INTO tenants(id, slug, name) VALUES ($1,$2,$3)", id, slug, name); err != nil {
		return "", false, fmt.Errorf("create locally owned tenant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil { return "", false, err }
	return id, true, nil
}
