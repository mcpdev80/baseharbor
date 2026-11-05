package workstationenrollmenthttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
	wshttp "github.com/mcpdev80/baseharbor/internal/workstationenrollment/http"
)

// wsBootstrap performs a real bootstrap for the given scope over HTTP and returns
// the parsed one-use credentials the store created. It only needs the handler;
// the scope is bound from the authenticated tenant by the resolver.
func wsBootstrap(t *testing.T, h http.Handler, station, tenant string) (string, string) {
	t.Helper()
	rec := do(t, h, "/bootstrap", bootstrapBody(), ctx(t, tenant))
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap for %s/%s = %d: %s", tenant, station, rec.Code, rec.Body.String())
	}
	var bs struct {
		Token string `json:"token"`
		Nonce string `json:"nonce"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&bs); err != nil {
		t.Fatal(err)
	}
	if bs.Token == "" || bs.Nonce == "" {
		t.Fatal("bootstrap returned empty credentials")
	}
	return bs.Token, bs.Nonce
}

// productionEnrollmentOverStore wires the real one-use Authority over the durable
// WorkstationEnrollmentStore and the CRL-backed LocalIssuer, exposing the same
// ProductionEnrollment interface the ProductionHandler needs. RecordRegistryFact
// writes the current serial to the durable registry (overwriting the prior row)
// and the store becomes the Owner the Authority reads back through CurrentSerial,
// exactly mirroring production.
type productionEnrollmentOverStore struct {
	store     *database.WorkstationEnrollmentStore
	authority *ws.Authority
	local     *serviceaccess.LocalIssuer
}

func newProductionEnrollmentOverStore(t *testing.T, pool *pgxpool.Pool) *productionEnrollmentOverStore {
	t.Helper()
	// The CRL-backed LocalIssuer requires canonical private state (a 0700 PKI
	// directory with a lifecycle lock). t.TempDir() alone is not enough under a
	// busy shared /tmp that carries other lanes' leftover locks, so pin a private
	// subdirectory the way EnsureLocalIssuer does for production.
	pkidir := filepath.Join(t.TempDir(), "pki")
	if err := os.MkdirAll(pkidir, 0o700); err != nil {
		t.Fatal(err)
	}
	local, err := serviceaccess.EnsureLocalIssuer(pkidir)
	if err != nil {
		t.Fatal(err)
	}
	// Publish the first CRL so the consumption path has a revocation list to read
	// (a missing CRL fails closed). This mirrors the CRL-backed lifecycle tests,
	// which call local.Revoke to bootstrap the CRL before any enrollment.
	if err := local.Revoke(context.Background(), "01"); err != nil {
		t.Fatal(err)
	}
	store := database.NewWorkstationEnrollmentStore(pool)
	authority, err := ws.New(store, local, store)
	if err != nil {
		t.Fatal(err)
	}
	return &productionEnrollmentOverStore{store: store, authority: authority, local: local}
}

func (e *productionEnrollmentOverStore) Create(ctx context.Context, scope ws.Scope, lifetime, certTTL time.Duration) (ws.Bootstrap, error) {
	return e.authority.Create(ctx, scope, lifetime, certTTL)
}

func (e *productionEnrollmentOverStore) Enroll(ctx context.Context, request ws.Request) (ws.Result, error) {
	return e.authority.Enroll(ctx, request)
}

func (e *productionEnrollmentOverStore) Renew(ctx context.Context, request ws.Request) (ws.Result, error) {
	return e.authority.Renew(ctx, request)
}

func (e *productionEnrollmentOverStore) Revoke(ctx context.Context, scope ws.Scope) error {
	return e.authority.Revoke(ctx, scope)
}

func (e *productionEnrollmentOverStore) RevokedState(ctx context.Context, scope ws.Scope) (bool, error) {
	return e.authority.RevokedState(ctx, scope)
}

func (e *productionEnrollmentOverStore) RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error {
	return e.store.RecordRegistryFact(ctx, scope, issuedAt, recipient, scopeID, serial, expiresAt)
}

// resetTestDatabase drops the test schema the migration and RLS depend on, so
// every cell starts from a clean, migrated tenant namespace.
func resetTestDatabase(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
DROP TABLE IF EXISTS connector_enrollment_grants CASCADE;
DROP TABLE IF EXISTS connector_certificate_overlap CASCADE;
DROP TABLE IF EXISTS connector_nodes CASCADE;
DROP TABLE IF EXISTS application_ownerships CASCADE;
DROP TABLE IF EXISTS memberships CASCADE;
DROP TABLE IF EXISTS external_identities CASCADE;
DROP TABLE IF EXISTS workstation_enrollment_grants CASCADE;
DROP TABLE IF EXISTS workstation_enrollment_registry CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;
DROP TABLE IF EXISTS baseharbor_schema_migrations CASCADE;
`)
	return err
}

// wsRestartDatabase spins up a real tenant-scoped PostgreSQL store, migrates it,
// and returns the pool plus the DSN a second, separate pool must reconnect on.
func wsRestartDatabase(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASEHARBOR_TEST_DATABASE_URL is required for restart-persistence qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := resetTestDatabase(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id,slug,name) VALUES ($1,'ws','Workstation')`, tenantA); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, dsn
}

// TestRenewPersistsNewSerialAcrossRealRestart is the round-3 durable-registry
// witness: it drives the real ProductionHandler over HTTP (bootstrap -> enroll ->
// renew) through a CRL-backed issuer and a durable WorkstationEnrollmentStore,
// then opens a fresh PostgreSQL pool on the same database (a genuine process/
// store-instance restart, not an in-memory double) and asserts the renewed serial
// is still current and no longer the enrolled one.
func TestRenewPersistsNewSerialAcrossRealRestart(t *testing.T) {
	rctx, pool, dsn := wsRestartDatabase(t)
	e := newProductionEnrollmentOverStore(t, pool)
	h, err := wshttp.NewProductionHandler(e, wshttp.ScopeResolverFromContext())
	if err != nil {
		t.Fatal(err)
	}
	scope := ws.Scope{TenantID: tenantA, Station: validStation}

	// Drive the real production seam over HTTP: authenticate through tenancy,
	// bootstrap, enroll and renew.
	token, nonce := wsBootstrap(t, h, validStation, tenantA)
	enroll := enrollBody(t, validStation, tenantA, token, nonce)
	rec := do(t, h, "/enroll", enroll, ctx(t, tenantA))
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll over HTTP = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var enrolled struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&enrolled); err != nil || enrolled.Serial == "" {
		t.Fatalf("enroll returned no serial: %v", err)
	}

	rec = do(t, h, "/renew", renewBody(t, validStation, tenantA), ctx(t, tenantA))
	if rec.Code != http.StatusOK {
		t.Fatalf("renew over HTTP = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var renewed struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&renewed); err != nil || renewed.Serial == "" {
		t.Fatalf("renew returned no serial: %v", err)
	}
	if renewed.Serial == enrolled.Serial {
		t.Fatal("renew must return a NEW serial distinct from the enrolled one")
	}

	// Alive-persistence witness: while the store is still open the renewed serial
	// is already current (the registry row was overwritten).
	alive, ok, err := e.store.CurrentSerial(rctx, scope)
	if err != nil || !ok {
		t.Fatalf("alive store CurrentSerial after renew failed: ok=%v err=%v", ok, err)
	}
	if alive != renewed.Serial {
		t.Fatalf("alive store did not follow the renewal: current=%q want=%q", alive, renewed.Serial)
	}

	// Real restart: a brand-new pool on the same database is a separate store
	// instance; an in-memory double cannot stand in here. The renewed serial must
	// still be current and must no longer be the enrolled one.
	reopened, err := pgxpool.New(rctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	afterRestart, ok, err := database.NewWorkstationEnrollmentStore(reopened).CurrentSerial(rctx, scope)
	if err != nil || !ok {
		t.Fatalf("reopened store CurrentSerial after restart failed: ok=%v err=%v", ok, err)
	}
	if afterRestart != renewed.Serial {
		t.Fatalf("durable registry forgot the renewed serial after a real restart: after=%q want=%q", afterRestart, renewed.Serial)
	}
	if afterRestart == enrolled.Serial {
		t.Fatalf("durable registry still reports the OLD enrolled serial after restart: %q", afterRestart)
	}

	// A later /renew after restart anchors the renewed leaf (the new serial is the
	// current one), proving the durable owner survived the restart end-to-end.
	renew2 := do(t, h, "/renew", renewBody(t, validStation, tenantA), ctx(t, tenantA))
	if renew2.Code != http.StatusOK {
		t.Fatalf("renew after restart = %d, want 200: %s", renew2.Code, renew2.Body.String())
	}
}

// TestRenewPersistsNewSerialDiscrimination proves the cell is discriminating:
// when the production path is NOT renewed the old serial stays current across a
// real restart, so a pass on the positive cell cannot be a false positive.
func TestRenewPersistsNewSerialDiscrimination(t *testing.T) {
	rctx, pool, dsn := wsRestartDatabase(t)
	e := newProductionEnrollmentOverStore(t, pool)
	h, err := wshttp.NewProductionHandler(e, wshttp.ScopeResolverFromContext())
	if err != nil {
		t.Fatal(err)
	}
	scope := ws.Scope{TenantID: tenantA, Station: validStation}

	token, nonce := wsBootstrap(t, h, validStation, tenantA)
	enroll := enrollBody(t, validStation, tenantA, token, nonce)
	rec := do(t, h, "/enroll", enroll, ctx(t, tenantA))
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll over HTTP = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var enrolled struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&enrolled); err != nil || enrolled.Serial == "" {
		t.Fatalf("enroll returned no serial: %v", err)
	}

	// No renew: the current serial stays the enrolled one, alive and after restart.
	alive, ok, err := e.store.CurrentSerial(rctx, scope)
	if err != nil || !ok {
		t.Fatalf("alive CurrentSerial failed: ok=%v err=%v", ok, err)
	}
	if alive != enrolled.Serial {
		t.Fatalf("current serial changed without a renewal: current=%q want=%q", alive, enrolled.Serial)
	}

	reopened, err := pgxpool.New(rctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	afterRestart, ok, err := database.NewWorkstationEnrollmentStore(reopened).CurrentSerial(rctx, scope)
	if err != nil || !ok {
		t.Fatalf("reopened CurrentSerial failed: ok=%v err=%v", ok, err)
	}
	if afterRestart != enrolled.Serial {
		t.Fatalf("expected the enrolled serial to survive restart without a renewal: got %q want %q", afterRestart, enrolled.Serial)
	}
}
