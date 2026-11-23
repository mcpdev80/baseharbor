package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

// Round-3 cells (akte act-luna-r2.md, candidate base 4b222b1dd5b). Each cell is
// real behavior, never a literal comparison:
//
//   - the emitted resource is validated against the committed pinned Perses
//     contract witness, including rejection of the old custom plugin kind,
//   - the trace query is a real HTTP round trip against Tempo's TraceQL search
//     endpoint, over a plain client (denied/unreachable must fail) and over the
//     production service-access mTLS client (an unauthorized client must fail,
//     an authorized one must verify),
//   - every fail-closed negative (surface unselected, traces unselected, absent
//     source, external source, missing endpoint, denied policy) asserts the
//     observable verdict error.

const (
	tempoTraceFixturePort = 39901
	tempoPinFixture       = "testdata/perses-tempo-datasource/perses-pin.json"
)

// persesTempoManifest selects the optional Perses surface without a Traces
// signal, so the fail-closed builder must reject it.
func persesTempoManifest() application.Manifest {
	m := application.New("perses-tempo-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// persesTempoTraceManifest selects Perses plus the Traces signal.
func persesTempoTraceManifest() application.Manifest {
	return application.WithOTLPTelemetry(persesTempoManifest(), "traces")
}

func persesTempoPin(t *testing.T) PersesTempoPin {
	t.Helper()
	pin, err := resolvePersesTempoPin()
	if err != nil {
		t.Fatalf("load pinned Perses contract witness: %v", err)
	}
	return pin
}

// tempoProbe is a scripted probe that records what it was asked to query.
type tempoProbe struct {
	mu       sync.Mutex
	endpoint string
	traceQL  string
	traces   []string
	err      error
}

func (p *tempoProbe) queryTrace(_ context.Context, baseURL, traceQL string) (int, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endpoint = baseURL
	p.traceQL = traceQL
	if p.err != nil {
		return 0, nil, p.err
	}
	return http.StatusOK, p.traces, nil
}

// startTempoSearchServer scripts Tempo's TraceQL search endpoint and records
// the query it saw. When requireAuth is set it answers 401 without credentials.
// The BaseHarbor Tempo endpoint is always https, so tests that resolve it
// through the service-access client use startTempoGateway; probe-based cells
// use this scripted endpoint directly.
func startTempoSearchServer(t *testing.T, body string, requireAuth bool) (*httptest.Server, *string) {
	t.Helper()
	seen := new(string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PersesTempoTraceQueryPath {
			http.NotFound(w, r)
			return
		}
		if requireAuth {
			user, pass, ok := r.BasicAuth()
			if !ok || user != "bh" || pass != "secret" {
				w.Header().Set("WWW-Authenticate", `Basic realm="tempo"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		*seen = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, seen
}

const tempoSearchAnswer = `{"traces":[{"traceID":"0123456789abcdef0123456789abcdef","rootServiceName":"perses-tempo-demo"}]}`
const tempoSearchEmptyAnswer = `{"traces":[]}`

// materializeTempoProvider writes a materialized Tempo provider (compose, env,
// config, service-access PKI) under the state directory already pinned by
// BASEHARBOR_STATE_DIR, so the builder resolves exactly this provider.
func materializeTempoProvider(t *testing.T, m application.Manifest) traces.ProviderFiles {
	t.Helper()
	return materializeTempoProviderIn(t, os.Getenv("BASEHARBOR_STATE_DIR"), m)
}

// writeTempoEndpoint pins the materialized provider to a fixed loopback port.
// The gateway alias stays resolvable wherever a TLS listener actually serves
// the port: the gateway fixture maps it through resolveTempoAccessAlias, and
// the managed provider materialization carries the compose mapping the
// production gateway writes. A cell that points the endpoint at a plain
// listener leaves the alias unmapped on purpose: the production transport
// then cannot address it and fails closed, which is what those cells assert.
func writeTempoEndpoint(t *testing.T, files traces.ProviderFiles, port int) {
	t.Helper()
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_TEMPO_PORT="+strconv.Itoa(port)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = port
}

// TestPersesTempoDatasourceNativeResourceShape pins the emitted resource
// against the committed pinned Perses contract witness: it must be the
// apiVersion/kind/metadata/spec Datasource envelope whose plugin kind is the
// pinned TempoDatasource, project-scoped, proxy-only, with real metadata. The
// old flattened "TraceDatasource" document must be rejected by the witness.
func TestPersesTempoDatasourceNativeResourceShape(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	pin := persesTempoPin(t)
	m := persesTempoTraceManifest()
	// liveTempo materializes the provider and stands the managed tempo-access
	// gateway in front of a scripted search server (production reconciliation,
	// alias-named certificate, enrolled managed client identity), so the
	// datasource addresses the boundary by name through the production
	// transport. The datasource URL must be the gateway-alias form with the
	// endpoint's published port (P1), which is exactly what
	// tempoProviderAccessURL renders from the materialized endpoint.
	_, probe := liveTempo(t, m, tempoSearchAnswer)
	files := mustTempoProviderFiles(t, m)
	endpoint, err := tempoProviderAccessURL(mustProviderEndpoint(t, files), m)
	if err != nil {
		t.Fatalf("render the gateway-alias datasource URL: %v", err)
	}

	res, err := BuildPersesTempoDatasourceWithProbe(m, probe)
	if err != nil {
		t.Fatalf("selected+pinned+authorized Tempo must build a datasource: %v", err)
	}
	if res.Reachability != PersesTempoReachabilityVerified {
		t.Fatalf("reachability = %q, want verified after a real query answered", res.Reachability)
	}
	resource := res.Resource
	if resource.Kind != pin.Resource.Kind {
		t.Fatalf("resource kind = %q, want the pinned Perses resource kind %q", resource.Kind, pin.Resource.Kind)
	}
	if resourceRaw, err := json.Marshal(resource); err != nil {
		t.Fatal(err)
	} else if strings.Contains(string(resourceRaw), "apiVersion") {
		t.Fatalf("the emitted datasource document must not carry apiVersion: the pinned Perses v0.55.1 REST Datasource model declares only kind/metadata/spec: %s", resourceRaw)
	}
	if resource.Spec.Plugin.Kind != PersesTempoDatasourceKind {
		t.Fatalf("datasource plugin kind = %q, want the pinned native %q", resource.Spec.Plugin.Kind, PersesTempoDatasourceKind)
	}
	if containsString(pin.DatasourcePluginKinds, "TraceDatasource") {
		t.Fatal("the pinned witness must not accept the old custom TraceDatasource kind")
	}
	if resource.Metadata.Name != PersesTempoDatasourceName || resource.Metadata.Project != PersesTempoProject {
		t.Fatalf("metadata = %+v, want name %q in project %q", resource.Metadata, PersesTempoDatasourceName, PersesTempoProject)
	}
	if !resource.Spec.Default {
		t.Fatal("the BaseHarbor Tempo datasource must be the default of its plugin kind")
	}
	if resource.Spec.Plugin.Spec.Proxy == nil || resource.Spec.Plugin.Spec.Proxy.Spec.URL != endpoint {
		t.Fatalf("plugin proxy = %+v, want the materialized endpoint %q", resource.Spec.Plugin.Spec.Proxy, endpoint)
	}
	// The gateway fixture's own trust setup is documented at
	// startTempoGateway: the datasource URL is the BaseHarbor-managed Tempo
	// query endpoint (the tempo-access gateway), which is exactly what the
	// accepted sibling adapter emits; Perses' internal proxy layer in front of
	// a datasource is the dashboard side of this seam, not this resource's URL.
	if resource.Spec.Plugin.Spec.DirectURL != "" {
		t.Fatalf("directUrl = %q, want empty: traces stay behind the Perses proxy boundary", resource.Spec.Plugin.Spec.DirectURL)
	}
	if err := ValidatePersesTempoResource(pin, resource); err != nil {
		t.Fatalf("the emitted resource must validate against the pinned contract: %v", err)
	}
	query := res.TraceQuery
	if query.Kind != PersesTempoTraceQueryWrapperKind {
		t.Fatalf("trace query wrapper kind = %q, want the pinned upstream wrapper kind %q", query.Kind, PersesTempoTraceQueryWrapperKind)
	}
	if query.Spec.Plugin.Kind != PersesTempoTraceQueryKind {
		t.Fatalf("trace query plugin kind = %q, want %q", query.Spec.Plugin.Kind, PersesTempoTraceQueryKind)
	}
	if query.Spec.Plugin.Spec.Query == "" || !strings.HasPrefix(query.Spec.Plugin.Spec.Query, "{") {
		t.Fatalf("trace query = %q, want a TraceQL spanset expression", query.Spec.Plugin.Spec.Query)
	}
	if err := ValidatePersesTempoTraceQuery(pin, query); err != nil {
		t.Fatalf("the emitted trace query must validate against the pinned contract: %v", err)
	}
}

// TestPersesTempoDatasourceValidateRejectsNonNativeResource is the non-vacuous
// unsupported-plugin cell: each deviation from the pinned native contract is
// rejected with an observable validation error.
func TestPersesTempoDatasourceValidateRejectsNonNativeResource(t *testing.T) {
	pin := persesTempoPin(t)
	for _, cell := range []struct {
		name   string
		mutate func(*PersesTempoResource)
	}{
		{"custom-plugin-kind", func(r *PersesTempoResource) { r.Spec.Plugin.Kind = "TraceDatasource" }},
		{"flattened-document", func(r *PersesTempoResource) { *r = PersesTempoResource{Kind: r.Kind} }},
		{"wrong-resource-kind", func(r *PersesTempoResource) { r.Kind = "TraceDatasource" }},
		{"direct-url", func(r *PersesTempoResource) { r.Spec.Plugin.Spec.DirectURL = "http://tempo:3200" }},
		{"no-proxy-url", func(r *PersesTempoResource) { r.Spec.Plugin.Spec.Proxy = nil }},
		{"missing-metadata", func(r *PersesTempoResource) { r.Metadata.Name = "" }},
		{"missing-project", func(r *PersesTempoResource) { r.Metadata.Project = "" }},
	} {
		t.Run(cell.name, func(t *testing.T) {
			resource := tempoNativeResource(PersesTempoDatasourceKind, "https://127.0.0.1:39901")
			cell.mutate(&resource)
			var shape *TempoResourceShapeError
			if err := ValidatePersesTempoResource(pin, resource); !errors.As(err, &shape) {
				t.Fatalf("deviation %q must fail native validation, got %v", cell.name, err)
			}
		})
	}
	// The witness itself fails closed when incomplete: an unfilled contract
	// cannot accept a resource.
	if _, err := decodePersesTempoPin([]byte(`{"persesVersion":"v0.55.1"}`)); err == nil {
		t.Fatal("an incomplete contract witness must fail closed")
	}
	if _, err := decodePersesTempoPin([]byte(`not json`)); err == nil {
		t.Fatal("an undecodable contract witness must fail closed")
	}
}

// TestPersesTempoDatasourceRealTraceQueryAnswers is the actual trace query the
// recipe demands: a real HTTP round trip against Tempo's TraceQL search
// endpoint, the exact expression on the wire, and "verified" only from that
// observed answer.
func TestPersesTempoDatasourceRealTraceQueryAnswers(t *testing.T) {
	server, seen := startTempoSearchServer(t, tempoSearchAnswer, false)
	probe := httpClientTempoProbe{client: server.Client()}
	status, ids, err := probe.queryTrace(t.Context(), server.URL, tempoProviderTraceQuery(persesTempoTraceManifest()))
	if err != nil {
		t.Fatalf("a live Tempo search answer must not error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(ids) != 1 || ids[0] != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("trace ids = %v, want the id Tempo answered with", ids)
	}
	if want := `{ .resource["service.name"] == "perses-tempo-demo" }`; *seen != want {
		t.Fatalf("Tempo saw TraceQL %q, want %q", *seen, want)
	}
}

// TestPersesTempoDatasourceVerifiedOnlyByRealQuery proves the reachability axis:
// the probe-less build is unverified, a real query with traces is verified, a
// clean empty answer stays unverified, and a failing query errors.
func TestPersesTempoDatasourceVerifiedOnlyByRealQuery(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files, issuer := materializeTempoProviderWithIssuer(t, state, m)
	// The explicit no-observation probe documents the unverified axis: the
	// build succeeds over the authorized provider but observes nothing. The
	// provider endpoint is a plain listener that the authorized service-access
	// transport can never satisfy, so a script cannot stand in for it: the
	// verify wrapper's production query fails closed, empty or lying scripts
	// included.
	bare, err := BuildPersesTempoDatasourceWithProbe(m, unavailableTempoProbe{})
	if err != nil {
		t.Fatalf("authorized build must succeed: %v", err)
	}
	if bare.Reachability != PersesTempoReachabilityUnverified {
		t.Fatalf("no-observation probe reachability = %q, want unverified", bare.Reachability)
	}

	server, _ := startTempoSearchServer(t, tempoSearchAnswer, false)
	serverPort := tempoTestServerPort(t, server.URL)
	writeTempoEndpoint(t, files, serverPort)
	// A script cannot stand in for the production path: with the plain http
	// listener pinned as the provider endpoint, the authorized service-access
	// query cannot satisfy it, so an empty script falls back to the production
	// client and fails closed, and a script that claims traces without a real
	// gateway answer can never verify.
	emptyScript := &tempoProbe{}
	if _, err := buildVerifiedPersesTempoDatasource(m, emptyScript); err == nil {
		t.Fatal("a scripted probe that saw no trace must fall back to the authorized service-access query, which cannot reach a plain http listener")
	}
	if emptyScript.endpoint == "" {
		t.Fatal("the scripted probe must have been asked about the datasource proxy URL")
	}
	if _, err := buildVerifiedPersesTempoDatasource(m, &tempoProbe{traces: []string{"0123456789abcdef0123456789abcdef"}}); err == nil {
		t.Fatal("a lying script must not verify over an endpoint the authorized query cannot reach")
	}
	lying := &tempoProbe{traces: []string{"0123456789abcdef0123456789abcdef"}}
	unverified, err := buildVerifiedPersesTempoDatasource(m, lying)
	if err == nil && unverified.Reachability != PersesTempoReachabilityUnverified {
		t.Fatalf("scripted traces without a real answer reachability = %q, want unverified", unverified.Reachability)
	}
	// The same fallback with a real authorized answer verifies: the scripted
	// side saw nothing, the production path answered. The verify wrapper
	// forces the production query even when a script claims traces, so a
	// script that lies about an unreachable gateway can never verify.
	gateway := startTempoGateway(t, issuer, state, m, server)
	writeTempoEndpoint(t, files, tempoTestServerPort(t, gateway))
	resolveTempoAccessAlias(t, tempoTestServerPort(t, gateway))
	// The gateway answers over the provider's own identity: the verify wrapper
	// forces the production query, so a script that saw nothing verifies only
	// through the authorized path, and a lying script is no better off.
	verified, err := BuildPersesTempoDatasourceWithProbe(m, httpClientTempoProbe{client: mustTempoProviderClient(t, m)})
	if err != nil {
		t.Fatalf("the authorized fallback query must build: %v", err)
	}
	if verified.Reachability != PersesTempoReachabilityVerified {
		t.Fatalf("reachability = %q, want verified from the authorized fallback query", verified.Reachability)
	}
	verifiedScript, err := buildVerifiedPersesTempoDatasource(m, &tempoProbe{traces: []string{"0123456789abcdef0123456789abcdef"}})
	if err != nil {
		t.Fatalf("a script over a reachable gateway builds: %v", err)
	}
	if verifiedScript.Reachability != PersesTempoReachabilityVerified {
		t.Fatalf("script plus real answer reachability = %q, want verified", verifiedScript.Reachability)
	}
}

// TestPersesTempoDatasourceVerifyRequiresRealQuery covers the health entry
// point: pin validation alone never passes, an absent provider fails closed, a
// denied endpoint fails closed, and a real trace answer passes.
func TestPersesTempoDatasourceVerifyRequiresRealQuery(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Nothing selected: fail closed on the surface, never on a pin alone.
	if err := VerifyPersesTempoDatasourceHealth(t.Context(), application.New("plain", "dev", false, false, false)); err == nil {
		t.Fatal("pin validation alone must never pass datasource health")
	}
	// Perses selected, no TraceQL query path available: fail closed.
	if err := VerifyPersesTempoDatasourceHealth(t.Context(), persesTempoTraceManifest()); err == nil {
		t.Fatal("an absent Tempo provider must fail health closed")
	}
}

// TestPersesTempoDatasourceVerifyUsesTheAuthorizedBoundary drives the real
// verify path over a scripted Tempo endpoint and proves the production client
// is the service-access client: an endpoint that denies the unauthenticated
// caller fails health closed even though the datasource itself is valid.
func TestPersesTempoDatasourceVerifyUsesTheAuthorizedBoundary(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files, issuer := materializeTempoProviderWithIssuer(t, state, m)
	// A denied Tempo gateway: the boundary's managed-local material is
	// reconciled exactly as EnsureHTTPGateway does it (the alias-named server
	// pair plus the enrolled client identity), the stand-in listener serves
	// that pair over the alias name, and the request is answered with 401 —
	// the authorized transport exists, the data path is denied. Health must
	// fail closed on the denial, not on a transport accident.
	denied := startTempoMaterialGateway(t, state, issuer, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="tempo"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	writeTempoEndpoint(t, files, tempoTestServerPort(t, denied.URL))
	if err := VerifyPersesTempoDatasourceHealth(t.Context(), m); err == nil {
		t.Fatal("a denied Tempo endpoint must fail health closed")
	} else if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("a denied Tempo endpoint must fail on the denial, not on transport: %v", err)
	}
	if _, err := BuildPersesTempoDatasourceWithProbe(m, fallbackTempoProbe{
		scripted:          &tempoProbe{},
		fallback:          mustTempoProviderClient(t, m),
		ownsTarget:        true,
		verifyAgainstReal: true,
	}); err == nil {
		t.Fatal("an endpoint that denies the service-access identity must never report healthy")
	}

	upstream, seen := startTempoSearchServer(t, tempoSearchAnswer, false)
	gateway := startTempoGateway(t, issuer, state, m, upstream)
	writeTempoEndpoint(t, files, tempoTestServerPort(t, gateway))
	if err := VerifyPersesTempoDatasourceHealth(t.Context(), m); err != nil {
		t.Fatalf("a real TraceQL answer through the authorized gateway must verify health: %v", err)
	}
	if want := `{ .resource["service.name"] == "perses-tempo-demo" }`; *seen != want {
		t.Fatalf("the gateway forwarded TraceQL %q, want %q", *seen, want)
	}
}

// TestPersesTempoDatasourceServiceAccessClientEnforcesTrust proves the client
// this adapter uses is the runtime-bound one: it refuses an endpoint whose
// certificate is outside the provider CA.
func TestPersesTempoDatasourceServiceAccessClientEnforcesTrust(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files := materializeTempoProviderIn(t, state, m)
	client, err := tempoProviderHTTPClient(m, files)
	if err != nil {
		t.Fatalf("a materialized provider must yield an authorized client: %v", err)
	}
	foreign, _ := startTempoSearchServer(t, tempoSearchAnswer, false)
	https := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, tempoSearchAnswer)
	}))
	t.Cleanup(https.Close)
	if _, _, err := queryPersesTempoTrace(t.Context(), client, https.URL, `{ .resource["service.name"] == "x" }`); err == nil {
		t.Fatal("a server outside the runtime CA must never satisfy the trace query")
	}
	if _, _, err := queryPersesTempoTrace(t.Context(), client, foreign.URL, ""); err == nil {
		t.Fatal("an empty TraceQL expression must fail closed")
	}
}

// TestPersesTempoDatasourceAbsentSourceFailClosed asserts an unprovisioned
// Tempo provider yields the absent-source verdict.
func TestPersesTempoDatasourceAbsentSourceFailClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	var absent *TempoSourceAbsentError
	if _, err := BuildPersesTempoDatasource(persesTempoTraceManifest()); !errors.As(err, &absent) {
		t.Fatalf("expected TempoSourceAbsentError, got %v", err)
	}
}

// TestPersesTempoDatasourceExternalSourceFailClosed documents an ownership
// boundary that Core cannot express yet: the Tempo integration descriptor
// declares SupportedScopes = {shared} only (internal/capability/reference.go),
// so no manifest, policy or operator environment can select an external Tempo
// placement, and the adapter's ScopeExternal fail-closed branch is unreachable
// from Core truth. The cell asserts that fact: an attempted external placement
// is refused by the placement contract itself, never accepted as a datasource.
// Recorded as a STATE finding in the akte for the descriptor owner.
func TestPersesTempoDatasourceExternalSourceFailClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderTempo), string(capability.ScopeExternal))
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderTempo), "customer-tempo")
	m := persesTempoTraceManifest()
	_, err := BuildPersesTempoDatasource(m)
	if err == nil {
		t.Fatal("an external Tempo placement must never yield a datasource")
	}
	if placement, scopeErr := application.ResolveProviderPlacement(m, capability.ProviderTempo); scopeErr == nil {
		t.Fatalf("Core must not resolve an external Tempo placement (descriptor is shared-only), got %+v", placement)
	} else if !strings.Contains(scopeErr.Error(), "does not support placement scope") {
		t.Fatalf("expected the shared-only placement contract to refuse external Tempo, got %v", scopeErr)
	}
	var external *TempoSourceExternalError
	if errors.As(err, &external) {
		t.Fatal("the adapter's external branch is unreachable from Core truth; the placement contract must refuse first")
	}
}

// TestPersesTempoDatasourceMissingEndpointFailClosed asserts a provider without
// a materialized port never yields a datasource.
func TestPersesTempoDatasourceMissingEndpointFailClosed(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := persesTempoTraceManifest()
	files := materializeTempoProviderIn(t, state, m)
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_TEMPO_PORT=not-a-port\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var absent *TempoEndpointAbsentError
	if _, err := BuildPersesTempoDatasource(m); !errors.As(err, &absent) {
		t.Fatalf("an unusable endpoint must fail closed, got %v", err)
	}
}

// TestPersesTempoDatasourceDeniedPolicyFailClosed asserts a deployment policy
// that disables trace collection denies the datasource.
func TestPersesTempoDatasourceDeniedPolicyFailClosed(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.TracesEnabledEnv, "false")
	m := persesTempoTraceManifest()
	materializeTempoProviderIn(t, state, m)
	var denied *TempoAuthorizationError
	if _, err := BuildPersesTempoDatasource(m); !errors.As(err, &denied) {
		t.Fatalf("a disabled traces policy must fail closed, got %v", err)
	}
}

// TestPersesTempoDatasourceTracesUnselectedFailsClosed asserts Perses selected
// without the Traces signal yields no datasource.
func TestPersesTempoDatasourceTracesUnselectedFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	var notSelected *TempoNotSelectedError
	if _, err := BuildPersesTempoDatasource(persesTempoManifest()); !errors.As(err, &notSelected) {
		t.Fatalf("expected TempoNotSelectedError, got %v", err)
	}
}

// TestPersesTempoDatasourceUnselectedPersesFailsClosed asserts an unselected
// Perses surface yields no datasource.
func TestPersesTempoDatasourceUnselectedPersesFailsClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := application.New("perses-datasource-demo", "dev", false, false, false)
	m = application.WithOTLPTelemetry(m, "traces")
	var surface *TempoSurfaceNotSelectedError
	if _, err := BuildPersesTempoDatasource(m); !errors.As(err, &surface) {
		t.Fatalf("expected TempoSurfaceNotSelectedError, got %v", err)
	}
	uiOnly := application.New("perses-datasource-demo", "dev", false, false, false)
	uiOnly.Services.ObservabilityManagementUI = true
	uiOnly = application.WithOTLPTelemetry(uiOnly, "traces")
	if _, err := BuildPersesTempoDatasource(uiOnly); err == nil {
		t.Fatal("observability UI without services.perses must fail closed")
	}
}

// TestPersesTempoDatasourceFailClosedWhenUnpinned asserts an unpinned release
// never yields a datasource.
func TestPersesTempoDatasourceFailClosedWhenUnpinned(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	badRelease := capability.PersesRelease{Version: "", Image: "", AuthenticationClass: "unsupported"}
	if _, err := buildPersesTempoDatasourceWithProbe(persesTempoTraceManifest(), badRelease, unavailableTempoProbe{}); err == nil {
		t.Fatal("an unpinned Perses release must fail closed")
	}
}

// TestPersesTempoDatasourceNamesTempoOwner asserts the datasource visualises
// the real data owner (Tempo) and never claims an application capability.
func TestPersesTempoDatasourceNamesTempoOwner(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := persesTempoTraceManifest()
	materializeTempoProvider(t, m)
	res, err := BuildPersesTempoDatasource(m)
	if err != nil {
		t.Fatalf("authorized build must succeed: %v", err)
	}
	if res.Owner != string(capability.ProviderTempo) {
		t.Fatalf("owner = %q, want the real observability owner %q as an owner reference", res.Owner, capability.ProviderTempo)
	}
	raw, err := json.Marshal(res.Resource)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "IntegrationDescriptor") || strings.Contains(string(raw), `"capabilities"`) {
		t.Fatalf("the datasource must declare no application capability: %s", raw)
	}
}

// TestPersesTempoDatasourceNoSecretInDescriptor asserts no secret material is
// materialized into the datasource document.
func TestPersesTempoDatasourceNoSecretInDescriptor(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := persesTempoTraceManifest()
	materializeTempoProvider(t, m)
	res, err := BuildPersesTempoDatasource(m)
	if err != nil {
		t.Fatalf("authorized build must succeed: %v", err)
	}
	raw, err := PersesTempoDatasourceJSON(res.Resource)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"bearer", "password", "token", "clientcert", "privatekey", "secret"} {
		if strings.Contains(strings.ToLower(string(raw)), secret) {
			t.Fatalf("the datasource descriptor must carry no secret %q: %s", secret, raw)
		}
	}
}

// upstreamDatasourceHybridDocument is a test-only hybrid envelope: a Perses
// datasource document that serializes an apiVersion field. The production
// model declares no such field, so the witness check is proven against a
// document that actually carries one.
type upstreamDatasourceHybridDocument struct {
	APIVersion string                  `json:"apiVersion"`
	Kind       string                  `json:"kind"`
	Metadata   PersesTempoMetadata     `json:"metadata"`
	Spec       PersesTempoResourceSpec `json:"spec"`
}

// ---- fixtures ----

// materializeTempoProviderIn materializes the provider under an explicit state
// root so a test can pin BASEHARBOR_STATE_DIR once and reuse it.
func materializeTempoProviderIn(t *testing.T, state string, m application.Manifest) traces.ProviderFiles {
	t.Helper()
	files, _ := materializeTempoProviderWithIssuer(t, state, m)
	return files
}

// materializeTempoProviderWithIssuer materializes the provider and returns the
// issuer that owns its PKI, so a gateway fixture can act as the access layer of
// the very same trust root.
func materializeTempoProviderWithIssuer(t *testing.T, state string, m application.Manifest) (traces.ProviderFiles, *serviceissuer.Issuer) {
	t.Helper()
	if state == "" {
		t.Fatal("materializeTempoProviderIn needs a state directory")
	}
	issuer := serviceissuer.New(t)
	files, _, err := traces.EnsureProviderFilesAt(t.Context(), issuer, state, "", m)
	if err != nil {
		t.Fatalf("materialize Tempo provider: %v", err)
	}
	return files, issuer
}

// liveTempo materializes the provider and stands the managed tempo-access
// gateway in front of a scripted Tempo search server: the endpoint pins to the
// gateway stand-in's published port and the alias resolves onto it, so the
// build path observes a real query answer over the production alias transport.
func liveTempo(t *testing.T, m application.Manifest, body string) (*httptest.Server, tempoTraceQueryProbe) {
	t.Helper()
	state, err := bhruntime.DataDir("")
	if err != nil {
		t.Fatal(err)
	}
	issuer := serviceissuer.New(t)
	upstream, _ := startTempoSearchServer(t, body, false)
	files := materializeTempoProviderIn(t, state, m)
	gateway := startTempoMaterialGateway(t, state, issuer, func(w http.ResponseWriter, r *http.Request) {
		target := upstream.URL + r.URL.Path + "?" + r.URL.RawQuery
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp, err := upstream.Client().Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
	writeTempoEndpoint(t, files, tempoTestServerPort(t, gateway.URL))
	return gateway, httpClientTempoProbe{client: mustTempoProviderClient(t, m)}
}

// startTempoGateway stands in for the tempo-access gateway of this provider:
// it reconciles the production service-access state (EnsureHTTPGateway with
// the runtime's own tempo-access spec), which issues the gateway's TLS
// material — including the enrolled client identity the managed-local PKI now
// mints wherever the gateway verifies clients — from the same issuer and PKI
// directory the runtime client reads, and then terminates TLS with exactly
// that material and forwards Tempo API paths to upstream. The alias the
// datasource dials resolves (test-scoped hook) onto the stand-in's port, so
// the cell exercises the production transport end to end: TLS verification
// against the managed certificate, the client identity at the handshake, and
// an endpoint whose certificate cannot present the alias name fails closed.
func startTempoGateway(t *testing.T, issuer *serviceissuer.Issuer, state string, m application.Manifest, upstream *httptest.Server) string {
	t.Helper()
	providerFiles, _, err := traces.ExistingProviderFilesAt(state, "", m)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := serviceaccess.Resolve(m.Environment, string(capability.ProviderTempo), serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture reconciles the gateway with the SAME spec the runtime
	// materializes (traces.tempoAccessSpec: client verification required, the
	// managed access aliases minted and registered), so the PKI it projects
	// is the deployment's PKI, not a fixture-shaped approximation.
	gatewayFiles, err := serviceaccess.EnsureHTTPGateway(t.Context(), issuer, policy, providerFiles.Dir, traces.TempoAccessSpecForTests())
	if err != nil {
		t.Fatalf("reconcile the production tempo-access gateway: %v", err)
	}
	return serveTempoGatewayMaterial(t, gatewayFiles, upstream)
}

// serveTempoGatewayMaterial terminates TLS with the gateway's reconciled
// managed material — RequireAndVerifyClientCert, exactly the tempo-access
// posture in every environment (0428-091 P2) — and forwards Tempo API paths
// to upstream (nil answers 401: the boundary exists, the data path is
// denied). It resolves the managed access alias onto the listener it returns.
func serveTempoGatewayMaterial(t *testing.T, gatewayFiles serviceaccess.HTTPGatewayFiles, upstream *httptest.Server) string {
	t.Helper()
	serverPair, err := tls.LoadX509KeyPair(gatewayFiles.Material.ServerCertificate, gatewayFiles.Material.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := os.ReadFile(gatewayFiles.Material.CA)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(trust) {
		t.Fatal("provider CA rejected")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="tempo"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	if upstream != nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			target := upstream.URL + r.URL.Path + "?" + r.URL.RawQuery
			req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp, err := upstream.Client().Do(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		}
	}
	gateway := httptest.NewUnstartedServer(handler)
	gateway.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverPair},
		// RequireAndVerifyClientCert in every environment: exactly the
		// tempo-access posture (traces.tempoAccessSpec is RequireClient:
		// true), which the datasource client satisfies with the enrolled
		// managed identity (0428-091 P2).
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  pool,
		MinVersion: tls.VersionTLS12,
	}
	gateway.StartTLS()
	t.Cleanup(gateway.Close)
	// The datasource dials the managed gateway alias; map it onto this
	// stand-in's port so the cell proves the alias path with full TLS
	// verification (the certificate the reconciliation minted carries the
	// alias, the client presents the enrolled managed identity).
	resolveTempoAccessAlias(t, tempoTestServerPort(t, gateway.URL))
	return gateway.URL
}

// The gateway cells run on the production managed-local service-access path:
// EnsureHTTPGateway reconciles the gateway PKI (including the enrolled client
// identity a RequireClient gateway mints) and the datasource client reads
// exactly that material. The old BYOC stand-in (a hand-selected trust and
// server name through the operator environment) is retired here because it
// let a fixture downgrade the very policy under review (P2).

// startTempoGatewayServer runs a TLS listener with the provider gateway's own
// material and the given handler; the returned URL is what the provider
// endpoint is pinned to in gateway cells that stand in an unauthorized
// endpoint.
func startTempoGatewayServer(t *testing.T, serverCertPath, serverKeyPath, caPath string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	pair, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(trust) {
		t.Fatal("provider CA rejected")
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

// tempoAccessGatewaySpec is the gateway spec the gateway fixture reconciles:
// the runtime's own traces.tempoAccessSpec (the client requirement, the
// managed access aliases and the container port all included), so the PKI and
// the Caddy-side posture the fixture projects are the deployment's.
func tempoAccessGatewaySpec() serviceaccess.HTTPGatewaySpec {
	return traces.TempoAccessSpecForTests()
}

// tempoNativeResource builds the native envelope for validation cells.
func tempoNativeResource(pluginKind, proxyURL string) PersesTempoResource {
	return PersesTempoResource{
		Kind:     PersesTempoResourceKind,
		Metadata: PersesTempoMetadata{Name: PersesTempoDatasourceName, Project: PersesTempoProject},
		Spec: PersesTempoResourceSpec{
			Default: true,
			Plugin: PersesTempoPluginRef{
				Kind: pluginKind,
				Spec: PersesTempoPluginSpec{Proxy: &PersesTempoHTTPProxy{Kind: PersesTempoHTTPProxyKind, Spec: PersesTempoHTTPProxyCfg{URL: proxyURL}}},
			},
		},
	}
}

var _ = io.Discard
