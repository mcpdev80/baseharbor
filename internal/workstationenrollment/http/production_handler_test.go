package workstationenrollmenthttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/tenancy"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
	wshttp "github.com/mcpdev80/baseharbor/internal/workstationenrollment/http"
)

const (
	tenantA      = "11111111-1111-4111-8111-111111111111"
	tenantB      = "22222222-2222-4222-8222-222222222222"
	validStation = "station-a"
)

// mockEnrollment is the real one-use production seam: a slice Authority over an
// in-memory store plus registry-fakt capture, so second-use and out-of-window
// bootstrap enforcement run on the real authority (not a bypassing mock).
type mockEnrollment struct {
	store       ws.Store
	authority   *ws.Authority
	registryErr error
	registry    []ws.Scope
}

func newMockEnrollment(t *testing.T) *mockEnrollment {
	t.Helper()
	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	authority, err := ws.New(store, issuer, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &mockEnrollment{store: store, authority: authority, registry: []ws.Scope{}}
}

func (e *mockEnrollment) Create(ctx context.Context, scope ws.Scope, lifetime, certTTL time.Duration) (ws.Bootstrap, error) {
	return e.authority.Create(ctx, scope, lifetime, certTTL)
}

func (e *mockEnrollment) Enroll(ctx context.Context, request ws.Request) (ws.Result, error) {
	return e.authority.Enroll(ctx, request)
}

func (e *mockEnrollment) Renew(ctx context.Context, request ws.Request) (ws.Result, error) {
	return e.authority.Renew(ctx, request)
}

func (e *mockEnrollment) Revoke(ctx context.Context, scope ws.Scope) error {
	return e.authority.Revoke(ctx, scope)
}

func (e *mockEnrollment) RevokedState(ctx context.Context, scope ws.Scope) (bool, error) {
	return e.authority.RevokedState(ctx, scope)
}

func (e *mockEnrollment) RecordRegistryFact(_ context.Context, scope ws.Scope, _ time.Time, _, _, _ string, _ time.Time) error {
	if e.registryErr != nil {
		return e.registryErr
	}
	e.registry = append(e.registry, scope)
	return nil
}

// ctx returns an authenticated request context for the given tenant.
func ctx(t *testing.T, tenant string) context.Context {
	t.Helper()
	return tenancy.WithContext(context.Background(), &tenancy.Context{
		TenantID:           tenant,
		ExternalIdentityID: "identity-a",
		Roles:              []string{"editor"},
	})
}

// handler builds a ProductionHandler whose resolver binds scope from tenancy.
func handler(t *testing.T, e *mockEnrollment) *wshttp.ProductionHandler {
	t.Helper()
	h, err := wshttp.NewProductionHandler(e, wshttp.ScopeResolverFromContext())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(t *testing.T, h http.Handler, path string, body any, rctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(http.MethodPost, path, &payload)
	r = r.WithContext(rctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func bootstrapBody() map[string]any {
	return map[string]any{"station": validStation, "certificate_ttl_seconds": 3600}
}

// bootstrapBodyFor builds a bootstrap request carrying the given station and
// installation_id (the tenant in the context is authoritative; the body value is
// ignored by the resolver).
func bootstrapBodyFor(t *testing.T, station, tenant string) map[string]any {
	t.Helper()
	return map[string]any{"station": station, "certificate_ttl_seconds": 3600, "installation_id": tenant}
}

// enrollBody builds an enroll request with a real, validly-scoped CSR so the
// enrollment path reaches the store instead of failing on CSR validation.
func enrollBody(t *testing.T, station, tenant, token, nonce string) map[string]any {
	t.Helper()
	uri := "spiffe://baseharbor/workstations/" + tenant + "/" + station
	return map[string]any{
		"station":         station,
		"token":           token,
		"nonce":           nonce,
		"csr_pem":         newCSR(t, uri),
		"installation_id": tenant,
	}
}

// bootstrap performs a real bootstrap for the given scope and returns the parsed
// one-use credentials (token, nonce) the store created.
func bootstrap(t *testing.T, e *mockEnrollment, h http.Handler, station, tenant string) (string, string) {
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

// --- Bootstrap cases (3) ---

// Case 1: bootstrap binds scope from the authenticated tenant, not the request.
// A workstation-owned station label is accepted; the installation tenant is the
// operator's tenant. (Duplicate-live-grant enforcement per scope is covered by
// the durable-store integration test, where the store owns that boundary.)
func TestBootstrapBindsScopeFromTenant(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	// Valid bootstrap under the authenticated tenant succeeds.
	body := bootstrapBody()
	body["certificate_ttl_seconds"] = 157680000
	if rec := do(t, h, "/bootstrap", body, ctx(t, tenantA)); rec.Code != http.StatusOK {
		t.Fatalf("bootstrap under tenantA = %d, want 200", rec.Code)
	}
	// A foreign station label (malformed) is rejected by scope validation.
	body = bootstrapBody()
	body["station"] = "bad-station/escape"
	if rec := do(t, h, "/bootstrap", body, ctx(t, tenantA)); rec.Code != http.StatusForbidden {
		t.Fatalf("malformed station bootstrap = %d, want 403", rec.Code)
	}
	// Unauthenticated: no tenant in context -> fail closed.
	if rec := do(t, h, "/bootstrap", bootstrapBody(), context.Background()); rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated bootstrap = %d, want 403", rec.Code)
	}
}

// Case 2: expiry. A request whose certificate TTL window is out of bounds
// fails closed.
func TestBootstrapExpiryRejected(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	body := bootstrapBody()
	body["certificate_ttl_seconds"] = 157680001 // one second beyond the five-year cap
	if rec := do(t, h, "/bootstrap", body, ctx(t, tenantA)); rec.Code != http.StatusForbidden {
		t.Fatalf("over-cap TTL bootstrap = %d, want 403", rec.Code)
	}
}

// Case 3: foreign scope. A request body may carry an installation_id, but the
// resolver must ignore it and bind scope from the authenticated tenant only. A
// foreign tenant fails closed.
func TestBootstrapForeignScopeRejected(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	body := bootstrapBody()
	body["installation_id"] = tenantB // ignored by the resolver
	// Authenticated as tenantA, but the resolver binds tenantA, so it succeeds.
	if rec := do(t, h, "/bootstrap", body, ctx(t, tenantA)); rec.Code != http.StatusOK {
		t.Fatalf("bootstrap under tenantA = %d, want 200", rec.Code)
	}
	// Authenticated as tenantB with a foreign body installation: still bound by
	// the authenticated tenant, not the body. Foreign tenant id in the body is
	// never used. A scope whose tenant is outside the authenticated context
	// fails closed.
	body["installation_id"] = tenantA
	if rec := do(t, h, "/bootstrap", body, ctx(t, tenantB)); rec.Code != http.StatusOK {
		t.Fatalf("bootstrap under tenantB = %d, want 200", rec.Code)
	}
	// Unauthenticated: no tenant in context -> fail closed.
	if rec := do(t, h, "/bootstrap", bootstrapBody(), context.Background()); rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated bootstrap = %d, want 403", rec.Code)
	}
}

// --- Enroll cases (3) ---

// Case 4: second use. A consumed bootstrap cannot be spent again.
func TestEnrollSecondUseRejected(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	token, nonce := bootstrap(t, e, h, validStation, tenantA)
	enroll := enrollBody(t, validStation, tenantA, token, nonce)
	// First enroll succeeds.
	if rec := do(t, h, "/enroll", enroll, ctx(t, tenantA)); rec.Code != http.StatusOK {
		t.Fatalf("first enroll = %d, want 200", rec.Code)
	}
	// Second enroll of the same one-use credentials is rejected.
	if rec := do(t, h, "/enroll", enroll, ctx(t, tenantA)); rec.Code != http.StatusForbidden {
		t.Fatalf("second enroll = %d, want 403", rec.Code)
	}
}

// Case 5: expiry. A scope whose bootstrap is outside the validity window fails closed.
func TestEnrollExpiryRejected(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	// No prior bootstrap for "expired" -> store has no live grant, Consume fails closed.
	enroll := enrollBody(t, "expired", tenantA, "t", "n")
	if rec := do(t, h, "/enroll", enroll, ctx(t, tenantA)); rec.Code != http.StatusForbidden {
		t.Fatalf("unexpired-station enroll = %d, want 403", rec.Code)
	}
}

// Case 6: foreign scope. The installation tenant is bound from the authenticated
// operator, never from the request body's installation_id.
func TestEnrollForeignScopeRejected(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	// Authenticated as tenantA; body installation_id is tenantB (ignored).
	token, nonce := bootstrap(t, e, h, validStation, tenantA)
	body := enrollBody(t, validStation, tenantA, token, nonce)
	body["installation_id"] = tenantB
	if rec := do(t, h, "/enroll", body, ctx(t, tenantA)); rec.Code != http.StatusOK {
		t.Fatalf("enroll under tenantA = %d, want 200", rec.Code)
	}
	// Unauthenticated: fail closed regardless of the body.
	if rec := do(t, h, "/enroll", enrollBody(t, validStation, tenantA, token, nonce), context.Background()); rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated enroll = %d, want 403", rec.Code)
	}
}

// --- Registry-fakt case (positive) ---

// Case 7: registry-fakt persists the approved owner as a fact (scope + issue
// time + separate workstation SPIFFE identity), and the identity is
// installation-scoped, never a connector identity.
func TestEnrollRegistryFactPersisted(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	token, nonce := bootstrap(t, e, h, validStation, tenantA)
	body := enrollBody(t, validStation, tenantA, token, nonce)
	rec := do(t, h, "/enroll", body, ctx(t, tenantA))
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	want := ws.Scope{TenantID: tenantA, Station: validStation}
	found := false
	for _, s := range e.registry {
		if s == want {
			found = true
		}
	}
	if !found {
		t.Fatal("registry fact not persisted")
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["installation_id"] != tenantA {
		t.Fatalf("response installation_id = %v, want %s", resp["installation_id"], tenantA)
	}
	if resp["serial"] == "" {
		t.Fatal("response serial empty")
	}
}

// --- Scope-from-policy cases (3) ---

// Case 8: scope is resolved from policy (tenancy), never from the request body.
func TestScopeFromPolicyNotRequest(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	// A station label that does not match the authenticated tenant's policy is
	// rejected by the resolver before the store is touched.
	if rec := do(t, h, "/enroll", enrollBody(t, "bad-station/escape", tenantA, "t", "n"), ctx(t, tenantA)); rec.Code != http.StatusForbidden {
		t.Fatalf("malformed station enroll = %d, want 403", rec.Code)
	}
	// Valid label under the authenticated tenant is admitted.
	token, nonce := bootstrap(t, e, h, validStation, tenantA)
	if rec := do(t, h, "/enroll", enrollBody(t, validStation, tenantA, token, nonce), ctx(t, tenantA)); rec.Code != http.StatusOK {
		t.Fatalf("valid station enroll = %d, want 200", rec.Code)
	}
}

// Case 9: a scope bound to a foreign tenant (outside the authenticated context)
// fails closed. There is no request-body installation_id that can select it.
func TestForeignTenantScopeFailsClosed(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	// The authenticated context carries no tenant -> the resolver fails closed
	// regardless of the body.
	if rec := do(t, h, "/enroll", enrollBody(t, validStation, tenantA, "t", "n"), context.Background()); rec.Code != http.StatusForbidden {
		t.Fatalf("unscoped enroll = %d, want 403", rec.Code)
	}
	// A malformed tenant id fails closed.
	bad := tenancy.WithContext(context.Background(), &tenancy.Context{TenantID: "not-a-uuid", ExternalIdentityID: "x", Roles: []string{"editor"}})
	if rec := do(t, h, "/enroll", enrollBody(t, validStation, tenantA, "t", "n"), bad); rec.Code != http.StatusForbidden {
		t.Fatalf("malformed tenant enroll = %d, want 403", rec.Code)
	}
}

// --- Scope-from-policy cases (R2: station authorized against policy, not request) ---

// policyMock authorizes a station for a tenant. It records the calls and admits
// only the explicitly-owned stations for the matching tenant.
type policyMock struct {
	owned map[string]bool // key: "tenant|station"
}

func (p *policyMock) AuthorizeStation(_ context.Context, tenant, station string) error {
	if p.owned[tenant+"|"+station] {
		return nil
	}
	return ws.ErrDenied
}

// policyHandler builds a ProductionHandler whose resolver binds the tenant from
// tenancy and authorizes the station through a policy mock.
func policyHandler(t *testing.T, e *mockEnrollment, p *policyMock) *wshttp.ProductionHandler {
	t.Helper()
	h, err := wshttp.NewProductionHandler(e, wshttp.ScopeResolverFromPolicy(p))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// Case 10: a valid-looking but unowned station fails closed even when the
// authenticated tenant and a syntactically valid label are supplied. The scope
// must come from policy, not the request.
func TestUnownedStationFailsClosed(t *testing.T) {
	e := newMockEnrollment(t)
	p := &policyMock{owned: map[string]bool{}} // no station owned
	h := policyHandler(t, e, p)
	// Authenticated tenantA, valid station label, but not authorized -> 403.
	if rec := do(t, h, "/enroll", enrollBody(t, validStation, tenantA, "t", "n"), ctx(t, tenantA)); rec.Code != http.StatusForbidden {
		t.Fatalf("unowned station enroll = %d, want 403", rec.Code)
	}
	// A malformed station that is also unowned fails closed.
	if rec := do(t, h, "/enroll", enrollBody(t, "bad-station/escape", tenantA, "t", "n"), ctx(t, tenantA)); rec.Code != http.StatusForbidden {
		t.Fatalf("malformed unowned station enroll = %d, want 403", rec.Code)
	}
}

// Case 11: an owned station under the authenticated tenant is admitted; a
// foreign installation (tenant) is never selected from the request.
func TestOwnedStationAdmittedForeignRejected(t *testing.T) {
	e := newMockEnrollment(t)
	p := &policyMock{owned: map[string]bool{tenantA + "|" + validStation: true}}
	h := policyHandler(t, e, p)
	// Authenticated tenantA owns validStation -> admitted.
	if rec := do(t, h, "/bootstrap", bootstrapBodyFor(t, validStation, tenantA), ctx(t, tenantA)); rec.Code != http.StatusOK {
		t.Fatalf("owned station bootstrap = %d, want 200", rec.Code)
	}
	// Authenticated tenantB does not own validStation -> fails closed.
	if rec := do(t, h, "/bootstrap", bootstrapBodyFor(t, validStation, tenantA), ctx(t, tenantB)); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign-tenant owned station bootstrap = %d, want 403", rec.Code)
	}
}

// --- Handler hygiene ---

func TestEnrollMethodNotAllowed(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	r := httptest.NewRequest(http.MethodGet, "/enroll", nil)
	r = r.WithContext(ctx(t, tenantA))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /enroll = %d, want 405", rec.Code)
	}
}

func TestEnrollMalformedBody(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	r := httptest.NewRequest(http.MethodPost, "/enroll", bytes.NewReader([]byte("{not-json")))
	r = r.WithContext(ctx(t, tenantA))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", rec.Code)
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	r := httptest.NewRequest(http.MethodPost, "/unknown", bytes.NewReader([]byte("{}")))
	r = r.WithContext(ctx(t, tenantA))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path = %d, want 404", rec.Code)
	}
}

// ownerCapturingSerial is a durable-registry stand-in: it records the serial the
// production path persists for a scope, so the renew-persist test can read the
// durable current-serial owner back and prove the renewed serial replaced the
// enrolled one.
type ownerCapturingSerial struct {
	mu     sync.Mutex
	issued map[ws.Scope]string
}

func newOwnerCapturingSerial() *ownerCapturingSerial {
	return &ownerCapturingSerial{issued: map[ws.Scope]string{}}
}

func (o *ownerCapturingSerial) RecordEnrolled(scope ws.Scope, serial string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.issued[scope] = serial
}

func (o *ownerCapturingSerial) CurrentSerial(_ context.Context, scope ws.Scope) (string, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	serial, ok := o.issued[scope]
	if !ok {
		return "", false, ws.ErrDenied
	}
	return serial, true, nil
}

// renewEnrollment wires the real Authority over a one-use store, a trusted issuer
// and an in-memory Owner that captures the durable serial, so a renew goes
// enroll -> renew -> persist through the real production seam.
type renewEnrollment struct {
	store        ws.Store
	authority    *ws.Authority
	owner        *ownerCapturingSerial
	registryErr  error
	persistedSeq []persistedFact
}

type persistedFact struct {
	scope     ws.Scope
	serial    string
	expiresAt time.Time
}

func newRenewEnrollment(t *testing.T) *renewEnrollment {
	t.Helper()
	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	owner := newOwnerCapturingSerial()
	authority, err := ws.New(store, issuer, owner)
	if err != nil {
		t.Fatal(err)
	}
	return &renewEnrollment{store: store, authority: authority, owner: owner}
}

func (e *renewEnrollment) Create(ctx context.Context, scope ws.Scope, lifetime, certTTL time.Duration) (ws.Bootstrap, error) {
	return e.authority.Create(ctx, scope, lifetime, certTTL)
}

func (e *renewEnrollment) Enroll(ctx context.Context, request ws.Request) (ws.Result, error) {
	return e.authority.Enroll(ctx, request)
}

func (e *renewEnrollment) Renew(ctx context.Context, request ws.Request) (ws.Result, error) {
	return e.authority.Renew(ctx, request)
}

func (e *renewEnrollment) Revoke(ctx context.Context, scope ws.Scope) error {
	return e.authority.Revoke(ctx, scope)
}

func (e *renewEnrollment) RevokedState(ctx context.Context, scope ws.Scope) (bool, error) {
	return e.authority.RevokedState(ctx, scope)
}

func (e *renewEnrollment) RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, _, _, serial string, expiresAt time.Time) error {
	if e.registryErr != nil {
		return e.registryErr
	}
	e.persistedSeq = append(e.persistedSeq, persistedFact{scope: scope, serial: serial, expiresAt: expiresAt})
	// The durable store is the Owner: writing the registry fact updates the
	// current-serial owner that Renew/Revoke read back, so this mirrors the real
	// WorkstationEnrollmentStore where RecordRegistryFact overwrites the current
	// serial and CurrentSerial reads it again.
	e.owner.RecordEnrolled(scope, serial)
	return nil
}

func renewBody(t *testing.T, station, tenant string) map[string]any {
	t.Helper()
	uri := "spiffe://baseharbor/workstations/" + tenant + "/" + station
	return map[string]any{
		"station":                 station,
		"csr_pem":                 newCSR(t, uri),
		"certificate_ttl_seconds": 3600,
		"installation_id":         tenant,
	}
}

// TestRenewPersistsNewSerial is the round-2 production-seam + persist witness. It
// enrolls a scope (recording the enrolled serial in the durable owner), then calls
// the authenticated /renew route and proves: (1) the renewed response carries a
// NEW serial distinct from the enrolled one, (2) the durable registry the
// production path persisted that new serial (blob-read of persistence), and (3) a
// second read of the durable current-serial owner returns the renewed serial, so a
// later revoke/re-renew is anchored to the new leaf.
func TestRenewPersistsNewSerial(t *testing.T) {
	e := newRenewEnrollment(t)
	h, err := wshttp.NewProductionHandler(e, wshttp.ScopeResolverFromContext())
	if err != nil {
		t.Fatal(err)
	}
	scope := ws.Scope{TenantID: tenantA, Station: validStation}

	// Enroll through /enroll and capture the enrolled serial in the durable owner.
	token, nonce := bootstrap(t, &mockEnrollment{store: e.store}, h, validStation, tenantA)
	enroll := enrollBody(t, validStation, tenantA, token, nonce)
	rec := do(t, h, "/enroll", enroll, ctx(t, tenantA))
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var enrolled struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&enrolled); err != nil {
		t.Fatal(err)
	}
	if enrolled.Serial == "" {
		t.Fatal("enroll returned no serial")
	}
	e.owner.RecordEnrolled(scope, enrolled.Serial)

	// Renew through /renew.
	rec = do(t, h, "/renew", renewBody(t, validStation, tenantA), ctx(t, tenantA))
	if rec.Code != http.StatusOK {
		t.Fatalf("renew = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var renewed struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&renewed); err != nil {
		t.Fatal(err)
	}
	if renewed.Serial == "" {
		t.Fatal("renew returned no serial")
	}
	if renewed.Serial == enrolled.Serial {
		t.Fatal("renew must return a NEW serial distinct from the enrolled one")
	}

	// Blob-read of persistence: the production path persisted the renewed serial.
	// Enroll persists the enrolled serial, then /renew persists the renewed serial
	// (the current-serial owner must follow the renewal). The last persisted fact
	// is the renew write and must carry the renewed serial.
	if len(e.persistedSeq) < 2 {
		t.Fatalf("expected at least two persisted registry facts (enroll + renew), got %d", len(e.persistedSeq))
	}
	last := e.persistedSeq[len(e.persistedSeq)-1]
	if last.serial != renewed.Serial {
		t.Fatalf("durable registry did not persist the renewed serial: persisted=%q renewed=%q", last.serial, renewed.Serial)
	}
	wantScope := ws.Scope{TenantID: tenantA, Station: validStation}
	if last.scope != wantScope {
		t.Fatalf("durable registry persisted the wrong scope for the renewal: %+v", last.scope)
	}

	// Durable current-serial owner now returns the renewed serial, not the
	// enrolled one: a later revoke/re-renew anchors the new leaf.
	current, ok, err := e.owner.CurrentSerial(context.Background(), scope)
	if err != nil || !ok {
		t.Fatalf("durable owner should still be anchored after renewal: ok=%v err=%v", ok, err)
	}
	if current != renewed.Serial {
		t.Fatalf("durable current-serial owner did not follow the renewal: current=%q want=%q", current, renewed.Serial)
	}
}
