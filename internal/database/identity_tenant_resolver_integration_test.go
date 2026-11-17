package database

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func TestIdentityTenantResolverUsesIdentityScopedRLS(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("testumgebung nicht hergestellt: BASEHARBOR_TEST_DATABASE_URL fehlt")
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

	const runtimePassword = "baseharbor-ci-resolver"
	if _, err := admin.Exec(ctx, `
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'baseharbor_resolver_ci') THEN
        CREATE ROLE baseharbor_resolver_ci LOGIN PASSWORD 'baseharbor-ci-resolver'
            NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
END
$$;
GRANT USAGE ON SCHEMA public TO baseharbor_resolver_ci;
GRANT SELECT ON external_identities, memberships TO baseharbor_resolver_ci;
`); err != nil {
		t.Fatal(err)
	}

	const (
		tenantA     = "11111111-1111-4111-8111-111111111111"
		tenantB     = "22222222-2222-4222-8222-222222222222"
		identityA   = "33333333-3333-4333-8333-333333333333"
		identityB   = "44444444-4444-4444-8444-444444444444"
		identityAmb = "55555555-5555-4555-8555-555555555555"
	)

	if _, err := admin.Exec(ctx, `
INSERT INTO tenants (id, slug, name) VALUES
    ($1, 'tenant-a', 'Tenant A'),
    ($2, 'tenant-b', 'Tenant B')
`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `
INSERT INTO external_identities (id, issuer, subject) VALUES
    ($1, 'https://issuer.example', 'subject-a'),
    ($2, 'https://issuer.example', 'subject-b'),
    ($3, 'https://issuer.example', 'subject-ambiguous')
`, identityA, identityB, identityAmb); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `
INSERT INTO memberships (id, tenant_id, external_identity_id, role) VALUES
    ('66666666-6666-4666-8666-666666666661', $1, $3, 'editor'),
    ('66666666-6666-4666-8666-666666666662', $1, $3, 'viewer'),
    ('66666666-6666-4666-8666-666666666663', $2, $4, 'viewer'),
    ('66666666-6666-4666-8666-666666666664', $1, $5, 'viewer'),
    ('66666666-6666-4666-8666-666666666665', $2, $5, 'viewer')
`, tenantA, tenantB, identityA, identityB, identityAmb); err != nil {
		t.Fatal(err)
	}

	runtimeDSN := os.Getenv("BASEHARBOR_TEST_DATABASE_URL") +
		"&user=baseharbor_resolver_ci&password=" + runtimePassword +
		"&connect_timeout=5"
	runtimeConfig, err := pgxpool.ParseConfig(runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	// Force reuse of the connection that previously held a tenant-scoped
	// transaction; a fresh connection would hide the empty custom-setting bug.
	runtimeConfig.MaxConns = 1
	runtime, err := pgxpool.NewWithConfig(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	var count int
	if err := runtime.QueryRow(ctx, "SELECT count(*) FROM memberships").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("resolver role without identity context can see %d memberships", count)
	}
	if err := WithTenantTx(ctx, runtime, tenantB, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM memberships").Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("tenant-scoped resolver read exposed wrong memberships: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.QueryRow(ctx, "SELECT count(*) FROM memberships").Scan(&count); err != nil || count != 0 {
		t.Fatalf("reset tenant leaked memberships or broke unscoped RLS: %d %v", count, err)
	}

	resolver := NewIdentityTenantResolver(runtime)
	resolved, err := resolver.ResolveTenant(ctx, &identity.Principal{Issuer: "https://issuer.example", Subject: "subject-a"})
	if err != nil {
		t.Fatalf("resolve subject-a: %v", err)
	}
	if resolved.TenantID != tenantA || resolved.ExternalIdentityID != identityA {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
	if len(resolved.Roles) != 2 || resolved.Roles[0] != "editor" || resolved.Roles[1] != "viewer" {
		t.Fatalf("unexpected roles: %#v", resolved.Roles)
	}

	resolved, err = resolver.ResolveTenant(ctx, &identity.Principal{Issuer: "https://issuer.example", Subject: "subject-b"})
	if err != nil {
		t.Fatalf("resolve subject-b: %v", err)
	}
	if resolved.TenantID != tenantB || resolved.ExternalIdentityID != identityB {
		t.Fatalf("unexpected tenant B resolution: %#v", resolved)
	}

	if _, err := resolver.ResolveTenant(ctx, &identity.Principal{Issuer: "https://issuer.example", Subject: "subject-ambiguous"}); !errors.Is(err, tenancy.ErrAmbiguousTenant) {
		t.Fatalf("ambiguous identity error = %v, want ErrAmbiguousTenant", err)
	}
	if _, err := resolver.ResolveTenant(ctx, &identity.Principal{Issuer: "https://issuer.example", Subject: "unknown"}); !errors.Is(err, tenancy.ErrNoMembership) {
		t.Fatalf("unknown identity error = %v, want ErrNoMembership", err)
	}
	if _, err := resolver.ResolveTenant(ctx, &identity.Principal{}); !errors.Is(err, ErrInvalidPrincipal) {
		t.Fatalf("invalid principal error = %v, want ErrInvalidPrincipal", err)
	}

	claimPrincipal := &identity.Principal{
		Issuer: "https://issuer.example", Subject: "subject-a", IssuerVerified: true,
		Groups: []string{"editor"},
	}
	claimContext := operatorauth.WithVerifiedPrincipal(ctx, claimPrincipal)
	claimContext = operatorauth.WithMembershipProvider(claimContext, resolver)
	decision, err := operatorauth.AuthorizeMachineOperation(claimContext, operatorauth.AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   operatorauth.OperationContext{Environment: "prod", Target: "target-a"},
	})
	if err != nil || !decision.Allowed || decision.Actor.Subject != claimPrincipal.Subject {
		t.Fatalf("current DB-backed editor membership was not claim-mapped: %#v %v", decision, err)
	}

	ambiguousPrincipal := &identity.Principal{
		Issuer: "https://issuer.example", Subject: "subject-ambiguous", IssuerVerified: true,
		Groups: []string{"viewer"},
	}
	ambiguousContext := operatorauth.WithVerifiedPrincipal(ctx, ambiguousPrincipal)
	ambiguousContext = operatorauth.WithMembershipProvider(ambiguousContext, resolver)
	if decision, err := operatorauth.AuthorizeMachineOperation(ambiguousContext, operatorauth.AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   operatorauth.OperationContext{Environment: "prod", Target: "target-a"},
	}); err == nil || decision.Allowed {
		t.Fatalf("same claimed role spanning two teams was not denied: %#v %v", decision, err)
	}

	if _, err := admin.Exec(ctx, "DELETE FROM memberships WHERE external_identity_id = $1 AND role = 'editor'", identityA); err != nil {
		t.Fatalf("revoke current editor membership: %v", err)
	}
	if decision, err := operatorauth.AuthorizeMachineOperation(claimContext, operatorauth.AuthorizationRequest{
		Operation: machine.Operation{ID: "app.show", Safety: machine.SafetyReadOnly, PolicyRequired: true},
		Context:   operatorauth.OperationContext{Environment: "prod", Target: "target-a"},
	}); err == nil || decision.Allowed {
		t.Fatalf("revoked DB-backed membership remained authorized: %#v %v", decision, err)
	}
}
