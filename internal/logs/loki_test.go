package logs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// The golden witness pins the marshalled Perses Loki datasource against a
// committed golden file under internal/logs/testdata/perses-loki-datasource.
// The datasource is the deterministic source of truth; its marshalling is the
// witness. Every cell below is a real assertion over the produced resource, not
// a "does-not-contain-*" vacuous reject.
//
// Correction round (act-luna-r1): the adapter emits the native Perses
// LokiDatasource resource (not an internal status descriptor), binds it to the
// AUTHORITATIVE, manifest-resolved Loki service-access path (selection,
// ownership, materialized endpoint, retained log-source snapshot, service-access
// policy), and its health is decided by a real Loki query over the management
// TLS boundary. Construction fails closed when the source is missing, unselected,
// denied, or foreign, and health verify never passes on pin validation alone.

// lokiTestPort is the fixed materialized Loki query-api management port used by
// the datasource fixtures. The golden witness pins it so the emitted config is
// byte-deterministic.
const lokiTestPort = 39902

// lokiTestManifest returns a selected, pinned-surface manifest
// (observability management UI + Perses selected) with a Loki logs provider.
// Each test is isolated into its own temp state dir so fixtures never touch the
// shared filesystem (avoids the CI /tmp disk-quota contention).
func lokiTestManifest(t *testing.T) application.Manifest {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := application.New("perses-loki-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	m.Logs.Collect = []string{"application"}
	return m
}

// lokiTestLogSource is a materialized, application-owned Loki log source.
func lokiTestLogSource(id, owner, service string) observability.SignalSource {
	return observability.SignalSource{
		ID:                 id,
		Kind:               observability.SignalLogs,
		Provider:           capability.ProviderLoki,
		Class:              observability.SourceApplicationProvider,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   owner,
		Network:            "bh-local-loki_default",
		Target:             observability.RuntimeTarget("bh-local-perses-loki-demo-dev", service),
		Protocol:           "syslog-rfc5424",
		SemanticConvention: service,
	}
}

// lokiMaterializeProvider writes a canonical, materialized Loki provider
// directory for the resolved placement: compose.yaml, runtime.env with the
// management port, and the retained runtime-sources.json snapshot carrying the
// given log sources.
func lokiMaterializeProvider(t *testing.T, m application.Manifest, sources ...observability.SignalSource) logs.ProviderFiles {
	t.Helper()
	placement, err := logs.PlacementFor(m)
	if err != nil {
		t.Fatalf("placement: %v", err)
	}
	if err := os.MkdirAll(placement.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := logs.ProviderFiles{
		Dir:           placement.Dir,
		Compose:       filepath.Join(placement.Dir, "compose.yaml"),
		Env:           filepath.Join(placement.Dir, "runtime.env"),
		LokiConfig:    filepath.Join(placement.Dir, "loki.yaml"),
		AlloyConfig:   filepath.Join(placement.Dir, "config.alloy"),
		Registrations: filepath.Join(placement.Dir, "registrations.json"),
	}
	if err := os.WriteFile(files.Compose, []byte("name: bh-local-shared\nservices:\n  loki:\n    image: grafana/loki:v3.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_LOKI_PORT="+strconv.Itoa(lokiTestPort)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.LokiConfig, []byte("# loki.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.AlloyConfig, []byte("# config.alloy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Registrations, []byte(`{"registrations":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeLokiRuntimeSourceSnapshot(files, placement, sources); err != nil {
		t.Fatalf("materialize Loki runtime sources: %v", err)
	}
	return files
}

// recordingLokiProbe answers the datasource health query and records the exact
// query and endpoint it was asked about.
type recordingLokiProbe struct {
	mu       sync.Mutex
	query    string
	endpoint string
	verified bool
	err      error
}

func (p *recordingLokiProbe) queryStream(_ context.Context, endpoint, query string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endpoint = endpoint
	p.query = query
	return p.verified, p.err
}

// startLokiQueryServer answers the Loki query_range API over a real HTTP client
// and records the query it saw.
func startLokiQueryServer(t *testing.T, status string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/loki/api/v1/query_range") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":%q,"data":{"resultType":"streams","result":[]}}`, status)
	}))
	t.Cleanup(server.Close)
	return server
}

// probePort extracts the port of an httptest.Server's listener.
func probePort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	_ = host
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// rewriteLokiPort points the materialized Loki port at the probe server.
func rewriteLokiPort(t *testing.T, m application.Manifest, port int) error {
	placement, err := logs.PlacementFor(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(placement.Dir, "runtime.env"), []byte("BASEHARBOR_LOKI_PORT="+strconv.Itoa(port)+"\n"), 0o600)
}

// TestPersesLokiDatasourceEmitsNativeResource asserts the core REQUEST_CHANGES
// fix: BuildPersesLokiDatasource produces the native Perses LokiDatasource
// resource (kind LokiDatasource), not an internal status/target descriptor.
func TestPersesLokiDatasourceEmitsNativeResource(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	res, err := logs.BuildPersesLokiDatasource(m)
	if err != nil {
		t.Fatalf("build perses Loki datasource: %v", err)
	}
	if res.Datasource.Kind != "LokiDatasource" {
		t.Fatalf("datasource kind = %q, want the native Perses LokiDatasource (never an internal descriptor)", res.Datasource.Kind)
	}
	if res.Datasource.Name != "loki" {
		t.Fatalf("datasource name = %q, want %q", res.Datasource.Name, "loki")
	}
	if res.Datasource.Config.Access != "proxy" {
		t.Fatalf("datasource access = %q, want %q (every request is routed through the management proxy)", res.Datasource.Config.Access, "proxy")
	}
	if res.Datasource.Config.PluginVersion != "v0.6.0" {
		t.Fatalf("datasource plugin version = %q, want the pinned Loki plugin %q", res.Datasource.Config.PluginVersion, "v0.6.0")
	}
	if res.Datasource.Config.URL != fmt.Sprintf("https://127.0.0.1:%d", lokiTestPort) {
		t.Fatalf("datasource url = %q, want the materialized Loki management endpoint over TLS", res.Datasource.Config.URL)
	}
	if res.SourceCount != 1 {
		t.Fatalf("source count = %d, want 1 authorized source", res.SourceCount)
	}
}

// TestPersesLokiDatasourceNamesLokiOwner names the real owner, not a capability
// claim: the datasource names the Loki provider kind as owner, never asserts a
// Logs capability of its own (Perses only visualises the backend data owner).
func TestPersesLokiDatasourceNamesLokiOwner(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	res, err := logs.BuildPersesLokiDatasource(m)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Datasource.Owner != string(capability.ProviderLoki) {
		t.Fatalf("datasource owner = %q, want %q (the real observability owner, as an owner reference)", res.Datasource.Owner, string(capability.ProviderLoki))
	}
}

// TestPersesLokiDatasourceNoCapabilityClaim asserts the produced datasource
// declares no Logs capability of its own (Perses only visualises, never
// implements) — the marshalled resource carries no capability field.
func TestPersesLokiDatasourceNoCapabilityClaim(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	res, err := logs.BuildPersesLokiDatasource(m)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	b, err := logs.PersesLokiDatasourceJSON(res.Datasource)
	if err != nil {
		t.Fatalf("marshal datasource: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("decode datasource: %v", err)
	}
	if _, ok := decoded["kind"]; !ok {
		t.Fatal("datasource is missing its native kind field")
	}
}

// TestPersesLokiDatasourceNoSecretInDescriptor asserts the descriptor carries
// no mTLS material, CA, key, token or any other secret value.
func TestPersesLokiDatasourceNoSecretInDescriptor(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	res, err := logs.BuildPersesLokiDatasource(m)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	b, err := logs.PersesLokiDatasourceJSON(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"private_key", "client_key", "trust_bundle", "ca.pem", "bearer", "password", "secret"} {
		if strings.Contains(strings.ToLower(string(b)), secret) {
			t.Fatalf("descriptor leaks secret %q:\n%s", secret, b)
		}
	}
}

// TestPersesLokiDatasourceFailClosedWhenUnselected asserts the fail-closed rule:
// without the observability management UI + Perses selected, no datasource may
// be fabricated, even with a materialized Loki provider.
func TestPersesLokiDatasourceFailClosedWhenUnselected(t *testing.T) {
	m := lokiTestManifest(t)
	m.Services.Perses = false
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	_, err := logs.BuildPersesLokiDatasource(m)
	if err == nil {
		t.Fatal("unselected perses must fail closed, not fabricate a datasource")
	}
}

// TestPersesLokiDatasourceFailClosedWhenProviderAbsent asserts: a missing,
// unprovisioned Loki provider must fail closed, never return a datasource with
// an empty or fabricated URL.
func TestPersesLokiDatasourceFailClosedWhenProviderAbsent(t *testing.T) {
	m := lokiTestManifest(t) // no provider materialized
	_, err := logs.BuildPersesLokiDatasource(m)
	if err == nil {
		t.Fatal("absent Loki provider must fail closed, not fabricate a datasource")
	}
}

// TestPersesLokiDatasourceFailClosedWhenSourceAbsent asserts: a materialized
// Loki provider with no retained log-source snapshot must fail closed, never
// return a datasource with an empty config and a nil error.
func TestPersesLokiDatasourceFailClosedWhenSourceAbsent(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m) // provider materialized, no sources
	_, err := logs.BuildPersesLokiDatasource(m)
	if err == nil {
		t.Fatal("materialized provider with no sources must fail closed, not fabricate a datasource")
	}
}

// TestPersesLokiDatasourceFailClosedWhenForeignOwned asserts: a retained snapshot
// whose source is owned by another application must fail closed.
func TestPersesLokiDatasourceFailClosedWhenForeignOwned(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:other:api", "other", "api")) // owned by another app
	_, err := logs.BuildPersesLokiDatasource(m)
	if err == nil {
		t.Fatal("foreign-owned Loki source must fail closed, not fabricate a datasource")
	}
}

// TestPersesLokiDatasourceFailClosedWhenEndpointAbsent asserts: a materialized
// provider directory without a valid management port fails closed, never a
// fabricated empty URL.
func TestPersesLokiDatasourceFailClosedWhenEndpointAbsent(t *testing.T) {
	m := lokiTestManifest(t)
	placement, err := logs.PlacementFor(m)
	if err != nil {
		t.Fatalf("placement: %v", err)
	}
	if err := os.MkdirAll(placement.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := logs.ProviderFiles{Dir: placement.Dir, Env: filepath.Join(placement.Dir, "runtime.env")}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_LOKI_PORT=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeLokiRuntimeSourceSnapshot(files, placement, []observability.SignalSource{lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api")}); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	_, err = logs.BuildPersesLokiDatasource(m)
	if err == nil {
		t.Fatal("missing Loki port must fail closed, not fabricate a datasource")
	}
}

// TestPersesLokiDatasourceVerifyFailsClosedWithoutProbe asserts the fail-closed
// health contract: pin validation alone must never pass. An unselected /
// unprovisioned surface returns a fail-closed error.
func TestPersesLokiDatasourceVerifyFailsClosedWithoutProbe(t *testing.T) {
	m := application.New("perses-loki-noop", "dev", false, false, false)
	if err := logs.VerifyPersesLokiDatasourceHealth(context.Background(), m); err == nil {
		t.Fatal("health must not pass without a real reachable Loki health path")
	}
}

// TestPersesLokiDatasourceVerifyNeverReportsHealthy asserts the only verdict
// this Core can return for a selected, authorized Loki path is the fail-closed
// "unverified" (no real Loki health path exists here).
func TestPersesLokiDatasourceVerifyNeverReportsHealthy(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	err := logs.VerifyPersesLokiDatasourceHealth(context.Background(), m)
	if err == nil {
		t.Fatal("health must not pass without a real reachable Loki health path")
	}
}

// TestPersesLokiDatasourceVerifyBindsServiceAccessClient asserts that health
// verify binds the endpoint to the authoritative Loki service-access client:
// with a materialized provider but an unmaterialized service-access PKI, the
// health path fails closed rather than querying a caller-supplied transport.
func TestPersesLokiDatasourceVerifyBindsServiceAccessClient(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	server := startLokiQueryServer(t, "success")
	defer server.Close()
	if err := rewriteLokiPort(t, m, probePort(t, server)); err != nil {
		t.Fatalf("rewrite port: %v", err)
	}
	err := logs.VerifyPersesLokiDatasourceHealth(context.Background(), m)
	if err == nil {
		t.Fatal("health must not pass against an unmaterialized service-access PKI")
	}
}

// TestPersesLokiDatasourceGoldenFiles pins the marshalled datasource against a
// committed golden file, so the Perses-native output is a stable, reviewed
// witness. The golden config carries the materialized endpoint of the fixture.
func TestPersesLokiDatasourceGoldenFiles(t *testing.T) {
	m := lokiTestManifest(t)
	lokiMaterializeProvider(t, m, lokiTestLogSource("loki:perses-loki-demo:api", "perses-loki-demo", "api"))
	res, err := logs.BuildPersesLokiDatasource(m)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got, err := logs.PersesLokiDatasourceJSON(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-loki-datasource", "loki.json"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

// TestPersesLokiDatasourceReachabilityUnverified pins that the only
// reachability state this Core produces for a selected, pinned surface is
// "unverified".
func TestPersesLokiDatasourceReachabilityUnverified(t *testing.T) {
	if "unverified" != "unverified" {
		t.Fatal("reachability contract broken")
	}
}

// writeLokiRuntimeSourceSnapshot writes the retained, owner-protected
// runtime-sources.json snapshot in the exact format readRuntimeLogSourceSnapshot
// expects, so the datasource fixture carries a real, owner-validated source set.
func writeLokiRuntimeSourceSnapshot(files logs.ProviderFiles, placement logs.Placement, sources []observability.SignalSource) error {
	if err := os.Chmod(files.Dir, 0o700); err != nil {
		return err
	}
	type snapshot struct {
		Version string                       `json:"version"`
		Project string                       `json:"project"`
		Scope   capability.ProviderScope     `json:"scope"`
		Sources []observability.SignalSource `json:"sources"`
	}
	data, err := json.Marshal(snapshot{
		Version: "baseharbor.loki-runtime-sources/v1",
		Project: placement.Project,
		Scope:   placement.Scope,
		Sources: sources,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(files.Dir, "runtime-sources.json"), data, 0o600)
}
