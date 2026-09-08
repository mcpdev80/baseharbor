package database

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationRollbackOwnsOnlyLatestChange(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASEHARBOR_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := resetTestDatabase(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback identity resolution migration: %v", err)
	}
	var identityPolicyExists bool
	if err := pool.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM pg_policies
    WHERE schemaname = 'public'
      AND tablename = 'memberships'
      AND policyname = 'memberships_identity_resolution'
)
`).Scan(&identityPolicyExists); err != nil {
		t.Fatal(err)
	}
	if identityPolicyExists {
		t.Fatal("identity resolution policy still exists after rolling back its owning migration")
	}

	var ownershipExists, tenantsExists, membershipsExists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.application_ownerships') IS NOT NULL").Scan(&ownershipExists); err != nil {
		t.Fatal(err)
	}
	if !ownershipExists {
		t.Fatal("rolling back identity resolution removed application ownership schema")
	}

	var rlsEnabled bool
	if err := pool.QueryRow(ctx, "SELECT relrowsecurity FROM pg_class WHERE oid = 'memberships'::regclass").Scan(&rlsEnabled); err != nil {
		t.Fatal(err)
	}
	if !rlsEnabled {
		t.Fatal("rolling back identity resolution unexpectedly disabled membership RLS")
	}

	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback application ownership migration: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.application_ownerships') IS NOT NULL").Scan(&ownershipExists); err != nil {
		t.Fatal(err)
	}
	if ownershipExists {
		t.Fatal("application ownership schema still exists after rolling back its owning migration")
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.tenants') IS NOT NULL").Scan(&tenantsExists); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.memberships') IS NOT NULL").Scan(&membershipsExists); err != nil {
		t.Fatal(err)
	}
	if !tenantsExists || !membershipsExists {
		t.Fatal("rolling back application ownership removed earlier identity schema")
	}
	if err := pool.QueryRow(ctx, "SELECT relrowsecurity FROM pg_class WHERE oid = 'memberships'::regclass").Scan(&rlsEnabled); err != nil {
		t.Fatal(err)
	}
	if !rlsEnabled {
		t.Fatal("rolling back application ownership unexpectedly disabled membership RLS")
	}

	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback RLS migration: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT relrowsecurity FROM pg_class WHERE oid = 'memberships'::regclass").Scan(&rlsEnabled); err != nil {
		t.Fatal(err)
	}
	if rlsEnabled {
		t.Fatal("RLS should be disabled after rolling back the RLS migration")
	}

	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback core migration: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.tenants') IS NOT NULL").Scan(&tenantsExists); err != nil {
		t.Fatal(err)
	}
	if tenantsExists {
		t.Fatal("core identity schema still exists after rolling back its owning migration")
	}
}
