package database

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

const (
	tenantA = "11111111-1111-4111-8111-111111111111"
	tenantB = "22222222-2222-4222-8222-222222222222"
)

// TestWorkstationEnrollmentDurableStore covers the durable one-use store:
// fail-closed consume, single live grant per scope, restart-safe consumption,
// and the registry-fakt persistence. All cases run against a real tenant-scoped
// PostgreSQL store (BASEHARBOR_TEST_DATABASE_URL), mirroring the connector
// integration contract without touching any connector_* table.
func TestWorkstationEnrollmentDurableStore(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("BASEHARBOR_TEST_DATABASE_URL is not set; the workstation enrollment store test must run against the configured test database")
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
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id,slug,name) VALUES
($1,'ws','Workstation'), ($2,'foreign','Foreign')`,
		tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	scope := workstationenrollment.Scope{TenantID: tenantA, Station: "station-a"}
	if err := scope.Validate(); err != nil {
		t.Fatal(err)
	}
	store := NewWorkstationEnrollmentStore(pool)

	// Create + duplicate rejected (exactly one live bootstrap per scope).
	if err := store.Create(ctx, bootstrapWith(scope, "token", "nonce")); err != nil {
		t.Fatal(err)
	}
	longLivedScope := workstationenrollment.Scope{TenantID: tenantA, Station: "five-year"}
	longLivedGrant := bootstrapWith(longLivedScope, "long-token", "long-nonce")
	longLivedGrant.CertificateTTL = workstationenrollment.MaxCertificateTTL
	if err := store.Create(ctx, longLivedGrant); err != nil {
		t.Fatalf("create five-year workstation grant: %v", err)
	}
	consumed, err := store.Consume(ctx, longLivedScope, "long-token", "long-nonce", workstationenrollment.MaxCertificateTTL)
	if err != nil || consumed.CertificateTTL != workstationenrollment.MaxCertificateTTL {
		t.Fatalf("consume five-year workstation grant: ttl=%s err=%v", consumed.CertificateTTL, err)
	}
	if err := store.Create(ctx, bootstrapWith(scope, "token2", "nonce2")); !errors.Is(err, workstationenrollment.ErrDenied) {
		t.Fatal("duplicate live bootstrap admitted")
	}

	// Consume once succeeds; second consume fails closed (single use).
	if _, err := store.Consume(ctx, scope, "token", "nonce", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(ctx, scope, "token", "nonce", time.Hour); !errors.Is(err, workstationenrollment.ErrDenied) {
		t.Fatal("second consume admitted")
	}

	// Foreign tenant / station / wrong token all fail closed.
	for _, scope := range []workstationenrollment.Scope{
		{TenantID: tenantB, Station: "station-a"},
		{TenantID: tenantA, Station: "other"},
	} {
		if _, err := store.Consume(ctx, scope, "token", "nonce", time.Hour); !errors.Is(err, workstationenrollment.ErrDenied) {
			t.Fatalf("foreign scope admitted: %+v", scope)
		}
	}
	if _, err := store.Consume(ctx, scope, "wrong", "nonce", time.Hour); !errors.Is(err, workstationenrollment.ErrDenied) {
		t.Fatal("wrong token digest admitted")
	}

	// Concurrency: exactly one of many simultaneous consumes wins. A fresh scope
	// is used so the grant is live (the previous scope was already consumed).
	concurrentScope := workstationenrollment.Scope{TenantID: tenantA, Station: "concurrency"}
	if err := store.Create(ctx, bootstrapWith(concurrentScope, "token", "nonce")); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			if _, err := store.Consume(ctx, concurrentScope, "token", "nonce", time.Hour); err == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("simultaneous DB successes = %d", successes.Load())
	}

	// Restart: a fresh pool cannot spend the same one-use grant twice (fail-closed
	// replay after process restart). The replay uses the already-consumed
	// {tenantA, station-a} scope captured before the foreign-scope loop.
	consumedScope := workstationenrollment.Scope{TenantID: tenantA, Station: "station-a"}
	reopened, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := NewWorkstationEnrollmentStore(reopened).Consume(ctx, consumedScope, "token", "nonce", time.Hour); !errors.Is(err, workstationenrollment.ErrDenied) {
		t.Fatal("replay after pool restart: grant re-deletable")
	}
	pool.Close()

	// Registry-fakt: persist + read the approved owner; a missing scope fails closed.
	// All calls here use the reopened store (the original pool is now closed).
	factScope := workstationenrollment.Scope{TenantID: tenantA, Station: "registry"}
	reopenedStore := NewWorkstationEnrollmentStore(reopened)
	if err := reopenedStore.Create(ctx, bootstrapWith(factScope, "rtoken", "rnonce")); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	if err := reopenedStore.RecordRegistryFact(ctx, factScope, time.Now().UTC(), "spiffe://baseharbor/workstations/"+tenantA+"/registry", "registry", "abcd", expires); err != nil {
		t.Fatal(err)
	}
	gotIssued, gotRecipient, gotScopeID, gotSerial, gotExpires, ok, err := reopenedStore.ReadRegistryFact(ctx, factScope)
	if err != nil || !ok {
		t.Fatalf("registry read failed: ok=%v err=%v", ok, err)
	}
	if gotRecipient != "spiffe://baseharbor/workstations/"+tenantA+"/registry" || gotSerial != "abcd" {
		t.Fatalf("registry mismatch: recipient=%q serial=%q", gotRecipient, gotSerial)
	}
	if gotIssued.IsZero() || gotExpires.Before(time.Now()) || gotScopeID == "" {
		t.Fatal("registry fact fields empty")
	}
	facts, err := reopenedStore.ListRegistryFacts(ctx, tenantA)
	if err != nil || len(facts) != 1 {
		t.Fatalf("tenant workstation list failed: facts=%+v err=%v", facts, err)
	}
	if facts[0].Station != factScope.Station || !facts[0].ExpiresAt.Equal(gotExpires) {
		t.Fatalf("listed workstation differs from registry: %+v", facts[0])
	}
	if _, err := reopenedStore.ListRegistryFacts(ctx, tenantB); err != nil {
		t.Fatalf("empty foreign tenant list should succeed without exposing tenant A: %v", err)
	}
	if _, _, _, _, _, ok, err := reopenedStore.ReadRegistryFact(ctx, workstationenrollment.Scope{TenantID: tenantB, Station: "registry"}); ok || err == nil {
		t.Fatal("foreign tenant registry read admitted")
	}
}

func bootstrapWith(scope workstationenrollment.Scope, token, nonce string) workstationenrollment.Bootstrap {
	return workstationenrollment.Bootstrap{
		RequestScope:   scope,
		Token:          token,
		Nonce:          nonce,
		ExpiresAt:      time.Now().Add(time.Minute),
		CertificateTTL: time.Hour,
	}
}

// TestWorkstationEnrollmentDuplicateLiveBootstrapRejected verifies the durable
// store rejects a second live bootstrap for the same scope exactly once, so a
// workstation can never hold two live authorizations.
func TestWorkstationEnrollmentDuplicateLiveBootstrapRejected(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("BASEHARBOR_TEST_DATABASE_URL is not set; the workstation enrollment store test must run against the configured test database")
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
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id,slug,name) VALUES ($1,'ws','Workstation')`, tenantA); err != nil {
		t.Fatal(err)
	}
	store := NewWorkstationEnrollmentStore(pool)
	scope := workstationenrollment.Scope{TenantID: tenantA, Station: "dup"}
	if err := store.Create(ctx, bootstrapWith(scope, "token", "nonce")); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, bootstrapWith(scope, "other", "nonce2")); !errors.Is(err, workstationenrollment.ErrDenied) {
		t.Fatal("second live bootstrap for same scope was admitted")
	}
	// A different station is a different scope and is admitted.
	other := workstationenrollment.Scope{TenantID: tenantA, Station: "dup-2"}
	if err := store.Create(ctx, bootstrapWith(other, "token", "nonce")); err != nil {
		t.Fatal(err)
	}
}
