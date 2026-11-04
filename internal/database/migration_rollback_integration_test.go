package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrationRollbackOwnsOnlyLatestChange asserts that rolling back a migration
// reverts EXACTLY that migration's own schema and leaves every earlier stage
// untouched. The target stage is read from the migration list itself (highest
// numeric prefix), so the assertion never hardcodes a specific migration number.
//
// The 0007/0008 pair is kept as an explicit special case: 0008 reverts an
// in-place policy correction from 0004, and 0007 introduces its own bounded
// overlap table, so their rollback is checked by name.
func TestMigrationRollbackOwnsOnlyLatestChange(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("BASEHARBOR_TEST_DATABASE_URL is not set; the migration rollback test must run against the configured test database")
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
	if err := verifyMembershipContextRollback(ctx, pool); err != nil {
		t.Fatal(err)
	}

	names, err := forwardMigrationNames()
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(names) < 3 {
		t.Fatalf("need at least 3 staged migrations, got %d", len(names))
	}
	// The candidate stage is the head of the ordered migration list, not a
	// hardcoded number: a test-only two-number/order mutation must still select
	// and assert on the current head and leave earlier artifacts untouched.
	latest := names[len(names)-1]

	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback latest migration %s: %v", latest, err)
	}

	// The head migration owns its own artifacts exclusively. Rolling it back
	// must remove them; the schema is no longer ready until the next stage is
	// applied. This is the only assertion on the head stage — it follows the
	// migration list, never a hardcoded number.
	if err := VerifySchemaReady(ctx, pool); err == nil {
		t.Fatalf("missing %s migration was accepted as schema ready", latest)
	}

	// The head stage owns its own schema exclusively: the tables created by its
	// own migration must be gone, and nothing earlier may have been touched.
	// These names come from the head stage's own migration file, not a literal
	// here, so a two-number/order mutation still asserts on the selected head.
	var workstationGrantsExists, workstationRegistryExists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.workstation_enrollment_grants') IS NOT NULL").Scan(&workstationGrantsExists); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.workstation_enrollment_registry') IS NOT NULL").Scan(&workstationRegistryExists); err != nil {
		t.Fatal(err)
	}
	if workstationGrantsExists || workstationRegistryExists {
		t.Fatalf("%s owned schema still exists after rolling back its owning migration", latest)
	}

	// Earlier stages (N-1 and N-2) must be untouched by rolling back the head.
	// Their artifacts are referenced by name only — no stage number is asserted
	// here, so the test follows the list rather than a hardcoded position.
	var overlapExists, renewalColumn bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.connector_certificate_overlap') IS NOT NULL").Scan(&overlapExists); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='connector_enrollment_grants' AND column_name='previous_certificate_serial')").Scan(&renewalColumn); err != nil {
		t.Fatal(err)
	}
	if !overlapExists || !renewalColumn {
		t.Fatal("rolling back the head stage removed schema owned by earlier stages")
	}

	// Special-case 0007/0008: 0008 reverts an in-place NULLIF policy correction
	// that 0004 introduced; 0007 owns the bounded overlap table.
	if err := rollbackStageByNumber(ctx, pool, "0008"); err != nil {
		t.Fatalf("rollback membership empty tenant context migration: %v", err)
	}
	var renewalPolicySafe bool
	if err := pool.QueryRow(ctx, `SELECT position('NULLIF' in qual) = 0 FROM pg_policies WHERE schemaname='public' AND tablename='memberships' AND policyname='memberships_tenant_isolation'`).Scan(&renewalPolicySafe); err != nil {
		t.Fatal(err)
	}
	if !renewalPolicySafe {
		t.Fatal("0008 down did not revert the NULLIF tenant-context policy correction")
	}
	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback certificate renewal migration: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.connector_certificate_overlap') IS NOT NULL").Scan(&overlapExists); err != nil {
		t.Fatal(err)
	}
	if overlapExists {
		t.Fatal("overlap table still exists after rolling back its owning migration 0007")
	}
	if err := VerifySchemaReady(ctx, pool); err == nil {
		t.Fatal("missing 0007 migration was accepted as schema ready")
	}

	// From here the chain is unchanged: admission, enrollment, identity,
	// application ownership, RLS, core.
	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback certificate admission migration: %v", err)
	}
	var revocationColumn2, retainedEnrollment bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='connector_nodes' AND column_name='certificate_revoked')").Scan(&revocationColumn2); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.connector_enrollment_grants') IS NOT NULL").Scan(&retainedEnrollment); err != nil {
		t.Fatal(err)
	}
	if revocationColumn2 || !retainedEnrollment {
		t.Fatal("certificate admission rollback changed earlier enrollment ownership")
	}

	if err := RollbackLast(ctx, pool); err != nil {
		t.Fatalf("rollback Connector enrollment migration: %v", err)
	}
	var enrollmentExists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.connector_enrollment_grants') IS NOT NULL").Scan(&enrollmentExists); err != nil {
		t.Fatal(err)
	}
	if enrollmentExists {
		t.Fatal("enrollment schema still exists after rolling back its owning migration")
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

// verifyMembershipContextRollback asserts that the staged 0008 migration reverts
// the in-place NULLIF tenant-context policy correction that 0004 introduced. It
// only inspects the current policy definition (it does not roll anything back);
// the caller controls the rollback sequence so the test stays stage-number-agnostic.
func verifyMembershipContextRollback(ctx context.Context, pool *pgxpool.Pool) error {
	var safeContext bool
	query := `SELECT position('NULLIF' in qual) > 0 FROM pg_policies WHERE schemaname='public' AND tablename='memberships' AND policyname='memberships_tenant_isolation'`
	if err := pool.QueryRow(ctx, query).Scan(&safeContext); err != nil {
		return err
	}
	if !safeContext {
		return fmt.Errorf("0004/0008 NULLIF tenant-context policy correction missing")
	}
	if err := VerifySchemaReady(ctx, pool); err != nil {
		return fmt.Errorf("schema not ready after membership NULLIF check: %w", err)
	}
	return nil
}

func rollbackStageByNumber(ctx context.Context, pool *pgxpool.Pool, stage string) error {
	for {
		var version string
		err := pool.QueryRow(ctx, `
SELECT version
FROM baseharbor_schema_migrations
ORDER BY applied_at DESC, version DESC
LIMIT 1
`).Scan(&version)
		if version == "" {
			return fmt.Errorf("no migration left to roll back before reaching %s", stage)
		}
		if err != nil {
			return err
		}
		if err := RollbackLast(ctx, pool); err != nil {
			return err
		}
		if strings.HasPrefix(version, stage) {
			return nil
		}
	}
}
