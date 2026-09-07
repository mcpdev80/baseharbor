package database

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTenantRLSIsolation(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASEHARBOR_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	if err := resetTestDatabase(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, admin); err != nil {
		t.Fatal(err)
	}

	const runtimePassword = "baseharbor-ci-runtime"
	if _, err := admin.Exec(ctx, `
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'baseharbor_runtime_ci') THEN
        CREATE ROLE baseharbor_runtime_ci LOGIN PASSWORD 'baseharbor-ci-runtime'
            NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
END
$$;
GRANT USAGE ON SCHEMA public TO baseharbor_runtime_ci;
GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, external_identities, memberships TO baseharbor_runtime_ci;
`); err != nil {
		t.Fatal(err)
	}

	const (
		tenantA  = "11111111-1111-4111-8111-111111111111"
		tenantB  = "22222222-2222-4222-8222-222222222222"
		identity = "33333333-3333-4333-8333-333333333333"
		memberA  = "44444444-4444-4444-8444-444444444444"
		memberB  = "55555555-5555-4555-8555-555555555555"
	)

	if _, err := admin.Exec(ctx, `
INSERT INTO tenants (id, slug, name) VALUES
    ($1, 'tenant-a', 'Tenant A'),
    ($2, 'tenant-b', 'Tenant B');
INSERT INTO external_identities (id, issuer, subject)
    VALUES ($3, 'https://issuer.example', 'subject-1');
INSERT INTO memberships (id, tenant_id, external_identity_id, role) VALUES
    ($4, $1, $3, 'editor'),
    ($5, $2, $3, 'viewer');
`, tenantA, tenantB, identity, memberA, memberB); err != nil {
		t.Fatal(err)
	}

	runtimeDSN := "postgres://baseharbor_runtime_ci:" + runtimePassword + "@127.0.0.1:5432/baseharbor_test?sslmode=disable"
	runtime, err := pgxpool.New(ctx, runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	var count int
	if err := runtime.QueryRow(ctx, "SELECT count(*) FROM memberships").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("missing tenant context must expose zero memberships, got %d", count)
	}

	if err := WithTenantTx(ctx, runtime, tenantA, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM memberships").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("tenant A must see exactly one membership, got %d", count)
		}

		var visible bool
		if err := tx.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM memberships WHERE id = $1)",
			memberB,
		).Scan(&visible); err != nil {
			return err
		}
		if visible {
			t.Fatal("tenant A can see tenant B membership")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	err = WithTenantTx(ctx, runtime, tenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			"INSERT INTO memberships (id, tenant_id, external_identity_id, role) VALUES ($1, $2, $3, 'viewer')",
			"66666666-6666-4666-8666-666666666666", tenantB, identity,
		)
		return err
	})
	if err == nil {
		t.Fatal("tenant A was allowed to insert a tenant B membership")
	}

	if !errors.Is(WithTenantTx(ctx, runtime, "not-a-uuid", func(pgx.Tx) error { return nil }), ErrInvalidTenantID) {
		t.Fatal("invalid tenant id was not rejected before opening tenant scope")
	}
}

func resetTestDatabase(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
DROP TABLE IF EXISTS memberships CASCADE;
DROP TABLE IF EXISTS external_identities CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;
DROP TABLE IF EXISTS baseharbor_schema_migrations CASCADE;
`)
	return err
}
