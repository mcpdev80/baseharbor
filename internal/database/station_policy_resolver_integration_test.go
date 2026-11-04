package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestStationPolicyResolverAuthorizesOwnedRejectsForeign verifies the workstation
// scope-authorizer (review-1371a R2): a station is authorized only when the
// authenticated operator has a membership in the installation tenant AND the
// station is known (registry-owned) for that tenant. A valid-looking but unowned
// station, or a foreign installation, fails closed.
func TestStationPolicyResolverAuthorizesOwnedRejectsForeign(t *testing.T) {
	dsn := "postgres://postgres:postgres@127.0.0.1:5433/baseharbor_test?sslmode=disable"
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

	const (
		tenantA     = "11111111-1111-4111-8111-111111111111"
		tenantB     = "22222222-2222-4222-8222-222222222222"
		identityA   = "33333333-3333-4333-8333-333333333333"
		identityB   = "44444444-4444-4444-8444-444444444444"
		stationA    = "station-a"
		stationUnow = "station-unowned"
	)
	if _, err := admin.Exec(ctx, `INSERT INTO tenants (id,slug,name) VALUES
($1,'tenant-a','Tenant A'), ($2,'tenant-b','Tenant B')`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO external_identities (id,issuer,subject) VALUES
($1,'iss','sub-a'), ($2,'iss','sub-b')`, identityA, identityB); err != nil {
		t.Fatal(err)
	}
	// tenantA: identityA owns station-a. tenantB: identityB has a membership but
	// no station is owned for it.
	if _, err := admin.Exec(ctx, `INSERT INTO memberships (id,tenant_id,external_identity_id,role) VALUES
($1,$2,$3,'editor')`, "66666666-6666-4666-8666-666666666661", tenantA, identityA); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO memberships (id,tenant_id,external_identity_id,role) VALUES
($1,$2,$3,'viewer')`, "66666666-6666-4666-8666-666666666662", tenantB, identityB); err != nil {
		t.Fatal(err)
	}
	recipient := fmt.Sprintf("spiffe://baseharbor/workstations/%s/station-a", tenantA)
	if _, err := admin.Exec(ctx, `INSERT INTO workstation_enrollment_registry (tenant_id,station,issued_at,recipient,scope,serial,expires_at)
VALUES ($1,$2,NOW(), $3, 'ws', 'serial-a', NOW() + INTERVAL '1 hour')`, tenantA, stationA, recipient); err != nil {
		t.Fatal(err)
	}

	resolver := NewStationPolicyResolver(admin)

	// Owned station under the authenticated tenant -> authorized.
	if err := resolver.AuthorizeStation(ctx, tenantA, stationA); err != nil {
		t.Fatalf("owned station authorized: %v", err)
	}
	// Valid but unowned station in an authenticated tenant -> fails closed.
	if err := resolver.AuthorizeStation(ctx, tenantA, stationUnow); err == nil {
		t.Fatal("unowned station was authorized")
	}
	// Foreign installation (tenantB owns no station) -> fails closed.
	if err := resolver.AuthorizeStation(ctx, tenantB, stationA); err == nil {
		t.Fatal("foreign-tenant station was authorized")
	}
}
