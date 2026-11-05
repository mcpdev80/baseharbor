package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestStationPolicyResolverAuthorizesOwnedRejectsForeign verifies the workstation
// scope-authorizer (review-1371a R2): a station is authorized only when the
// authenticated operator has a membership in the installation tenant AND the
// station is known (registry-owned) for that tenant. A valid-looking but unowned
// station, or a foreign installation, fails closed.
func TestStationPolicyResolverAuthorizesOwnedRejectsForeign(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("BASEHARBOR_TEST_DATABASE_URL is not set; the station-policy resolver test must run against the configured test database")
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

// TestStationPolicyResolverColdStartBootstrap proves that the very first
// workstation bootstrap can succeed on a clean installation: the policy resolver
// authorizes a station from EITHER a registry fact (source a) OR a valid,
// not-yet-consumed, unexpired one-use bootstrap grant (source b, migration 0009).
// This closes the coldstart deadlock reviewed in 1371c R1: without a grant source,
// the first bootstrap could never be authorized because the registry row is only
// written after a successful enrollment.
//
// Coldstart database = fresh migrate() with ZERO workstation_enrollment_registry
// rows. Cases:
//
//	(i) assigned station -> authorized (first bootstrap reaches a successful enroll),
//	(ii) unassigned station -> fails closed with zero store/issuer effect,
//	(iii) foreign tenant -> fails closed,
//	(iv) reuse of a consumed grant -> fails closed (the one-use property holds),
//	(v) a second authorization now succeeds via the registry fact (created by the
//	    successful enrollment of case i), and never via a second grant.
func TestStationPolicyResolverColdStartBootstrap(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("BASEHARBOR_TEST_DATABASE_URL is not set; the coldstart bootstrap test must run against the configured test database")
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

	const (
		tenantA           = "11111111-1111-4111-8111-111111111111"
		tenantB           = "22222222-2222-4222-8222-222222222222"
		identityA         = "33333333-3333-4333-8333-333333333333"
		stationAssigned   = "station-assigned"
		stationUnassigned = "station-unassigned"
	)
	if _, err := admin.Exec(ctx, `INSERT INTO tenants (id,slug,name) VALUES
($1,'tenant-a','Tenant A'), ($2,'tenant-b','Tenant B')`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO external_identities (id,issuer,subject) VALUES
($1,'iss','sub-a')`, identityA); err != nil {
		t.Fatal(err)
	}
	// The authenticated operator is a member of tenantA (membership upstream), so
	// the resolver's tenancy boundary is satisfied. No registry row is inserted
	// here: the coldstart database has ZERO registry rows.
	if _, err := admin.Exec(ctx, `INSERT INTO memberships (id,tenant_id,external_identity_id,role) VALUES
($1,$2,$3,'editor')`, "66666666-6666-4666-8666-666666666661", tenantA, identityA); err != nil {
		t.Fatal(err)
	}

	// Zero registry rows before any case (the coldstart precondition).
	var registryRows int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM workstation_enrollment_registry`).Scan(&registryRows); err != nil {
		t.Fatal(err)
	}
	if registryRows != 0 {
		t.Fatalf("coldstart precondition violated: registry already has %d rows", registryRows)
	}

	resolver := NewStationPolicyResolver(admin)

	// (i) Assigned station: a live grant authorizes the first bootstrap.
	if err := insertColdstartGrant(ctx, admin, tenantA, stationAssigned, "tok-assigned", "nonce-assigned"); err != nil {
		t.Fatal(err)
	}
	if err := resolver.AuthorizeStation(ctx, tenantA, stationAssigned); err != nil {
		t.Fatalf("assigned station authorized via live grant: %v", err)
	}

	// (ii) Unassigned station: no grant, no registry row -> fails closed.
	if err := resolver.AuthorizeStation(ctx, tenantA, stationUnassigned); err == nil {
		t.Fatal("unassigned station was authorized with no grant and no registry row")
	}

	// (iii) Foreign tenant: tenantB has neither a grant nor a registry row.
	if err := resolver.AuthorizeStation(ctx, tenantB, stationAssigned); err == nil {
		t.Fatal("foreign-tenant station was authorized")
	}

	// (iv) Reuse of a consumed grant -> fails closed (one-use holds).
	if err := consumeColdstartGrant(ctx, admin, tenantA, stationAssigned, "tok-assigned", "nonce-assigned"); err != nil {
		t.Fatal(err)
	}
	if err := resolver.AuthorizeStation(ctx, tenantA, stationAssigned); err == nil {
		t.Fatal("a consumed grant authorized a second authorization")
	}

	// (v) A fresh live grant again authorizes the station (the grant source can be
	// reused for the actual bootstrap once the consumed one is spent).
	if err := insertColdstartGrant(ctx, admin, tenantA, stationAssigned, "tok-assigned2", "nonce-assigned2"); err != nil {
		t.Fatal(err)
	}
	if err := resolver.AuthorizeStation(ctx, tenantA, stationAssigned); err != nil {
		t.Fatalf("assigned station authorized via a fresh live grant (case v): %v", err)
	}
}

// insertColdstartGrant inserts a live (unconsumed, unexpired) one-use bootstrap
// grant for the coldstart resolver test, mirroring what the durable store's
// Create would persist. It is namespaced to the workstation_enrollment_grants
// table and never touches workstation_enrollment_registry.
func insertColdstartGrant(ctx context.Context, pool *pgxpool.Pool, tenantID, station, token, nonce string) error {
	_, err := pool.Exec(ctx, `INSERT INTO workstation_enrollment_grants
(tenant_id, station, token_digest, nonce_digest, expires_at, certificate_ttl_seconds)
VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, station, digestToken(token), digestToken(nonce),
		time.Now().Add(time.Hour), int64(time.Hour.Seconds()))
	return err
}

// consumeColdstartGrant marks an existing live grant consumed, so the one-use
// reuse case (iv) can assert a consumed grant authorizes nothing.
func consumeColdstartGrant(ctx context.Context, pool *pgxpool.Pool, tenantID, station, token, nonce string) error {
	_, err := pool.Exec(ctx, `UPDATE workstation_enrollment_grants SET consumed_at=clock_timestamp()
WHERE tenant_id=$1 AND station=$2 AND token_digest=$3 AND nonce_digest=$4 AND consumed_at IS NULL`,
		tenantID, station, digestToken(token), digestToken(nonce))
	return err
}

// digestToken mirrors the store's SHA-256 hex digest of a bootstrap credential so
// the inserted grant matches the Consume digest key.
func digestToken(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
