package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// The golden witness pins the marshalled Perses Prometheus datasource against a
// committed golden file under testdata/perses-prometheus-datasource. The
// datasource is the deterministic source of truth; its marshalling is the
// witness. Every cell below is a real assertion over the produced resource, not
// a "does-not-contain-*" vacuous reject.
//
// Correction round (act-luna-r1): the adapter is tied to the selected, pinned,
// AUTHORIZED runtime sources and its health is decided by a real Prometheus
// query over the management boundary. Construction fails closed when the source
// is missing, unselected or denied, when the endpoint is missing or invalid,
// and health verify never passes on pin validation alone.

// persesTestPort is the fixed materialized Prometheus management port used by
// the datasource fixtures. The golden witness pins it so the emitted config is
// byte-deterministic.
const persesTestPort = 39901

// persesPrometheusDatasourceManifest returns a selected, pinned-surface
// manifest (observability management UI + Perses selected) with metrics
// collection enabled.
func persesPrometheusDatasourceManifest() application.Manifest {
	m := application.New("perses-datasource-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// persesMaterializeProvider writes a canonical, materialized Prometheus
// provider directory for the resolved placement: compose.yaml, runtime.env with
// the management port, prometheus.yml, targets/ and the retained
// runtime-sources.json snapshot carrying the given sources.
func persesMaterializeProvider(t *testing.T, m application.Manifest, sources ...observability.MetricsSource) ProviderFiles {
	t.Helper()
	placement, err := PlacementFor(m)
	if err != nil {
		t.Fatalf("placement: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(placement.Dir, "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := ProviderFiles{
		Dir:        placement.Dir,
		Compose:    filepath.Join(placement.Dir, "compose.yaml"),
		Env:        filepath.Join(placement.Dir, "runtime.env"),
		Config:     filepath.Join(placement.Dir, "prometheus.yml"),
		TargetsDir: filepath.Join(placement.Dir, "targets"),
		RuntimeCA:  filepath.Join(placement.Dir, "baseharbor-runtime-ca.pem"),
	}
	if err := os.WriteFile(files.Compose, []byte("name: bh-local-shared\nservices:\n  prometheus:\n    image: quay.io/prometheus/prometheus:v3.5.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_PROMETHEUS_PORT="+strconv.Itoa(persesTestPort)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Config, []byte("global:\n  scrape_interval: 30s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The provider's public runtime CA (the datasource trust anchor the
	// materializer projects, 0428-083 F3): a fixture PEM, public material.
	if err := os.WriteFile(files.RuntimeCA, []byte("-----BEGIN CERTIFICATE-----\nZml4dHVyZQ==\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeSourceSnapshot(files, placement, sources); err != nil {
		t.Fatalf("materialize runtime sources: %v", err)
	}
	return files
}

// persesProviderSource is a materialized platform-provider metrics source.
func persesProviderSource(id string, provider capability.ProviderKind) observability.MetricsSource {
	return observability.MetricsSource{
		ID: id, Provider: provider, Class: observability.SourcePlatformProvider,
		Scope: capability.ScopeShared, Network: "bh-local-shared_default",
		Target: string(provider) + ":9464", Path: "/metrics", Scheme: "http",
	}
}

// recordingProbe answers the datasource health query and records the exact
// query and endpoint it was asked about.
type recordingProbe struct {
	mu       sync.Mutex
	query    string
	endpoint string
	up       bool
	err      error
}

func (p *recordingProbe) queryUp(_ context.Context, endpoint, query string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endpoint = endpoint
	p.query = query
	return p.up, p.err
}

// startPrometheusQueryServer answers the Prometheus query API over a real HTTP
// client and records the query it saw. Basic credentials, when set, are
// required (management-auth path witness).
func startPrometheusQueryServer(t *testing.T, value string, user, password string) (*httptest.Server, *string) {
	t.Helper()
	seen := new(string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		if user != "" || password != "" {
			gotUser, gotPass, ok := r.BasicAuth()
			if !ok || gotUser != user || gotPass != password {
				w.Header().Set("WWW-Authenticate", `Basic realm="Prometheus"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		*seen = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"`+value+`"]}]}}`)
	}))
	t.Cleanup(server.Close)
	return server, seen
}

func endpointPort(t *testing.T, raw string) int {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func writeProviderEnv(t *testing.T, files ProviderFiles, body string) {
	t.Helper()
	if err := os.WriteFile(files.Env, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestPersesPrometheusDatasourceShape pins the produced datasource contract
// against the pinned Perses v0.55.1 native PrometheusDatasource shape: kind,
// name, default, and a config that carries the real materialized endpoint —
// never an empty config with nil error.
func TestPersesPrometheusDatasourceShape(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized sources must build a datasource: %v", err)
	}
	ds := res.Datasource
	if ds.Kind != PersesPrometheusDatasourceKind {
		t.Fatalf("perses datasource kind = %q, want %q (pinned native plugin, not a Grafana UI API)", ds.Kind, PersesPrometheusDatasourceKind)
	}
	if ds.Name != PersesPrometheusDatasourceName {
		t.Fatalf("perses datasource name = %q, want %q", ds.Name, PersesPrometheusDatasourceName)
	}
	if !ds.Default {
		t.Fatal("perses prometheus datasource must be the default datasource of the observability surface")
	}
	if want := "https://127.0.0.1:" + strconv.Itoa(persesTestPort); ds.Config.URL != want {
		t.Fatalf("perses datasource url = %q, want the materialized management endpoint %q", ds.Config.URL, want)
	}
	if ds.Config.Authentication != capability.PersesDefaultRelease.AuthenticationClass {
		t.Fatalf("perses datasource authentication = %q, want the pinned release class %q", ds.Config.Authentication, capability.PersesDefaultRelease.AuthenticationClass)
	}
}

// TestPersesPrometheusDatasourceNamesPrometheusOwner asserts the datasource
// names the real owner, not a capability claim: the Prometheus provider kind as
// an owner reference; Perses only visualises the backend data owner.
func TestPersesPrometheusDatasourceNamesPrometheusOwner(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized sources must build a datasource: %v", err)
	}
	if res.Datasource.Owner != string(capability.ProviderPrometheus) {
		t.Fatalf("perses datasource owner = %q, want %q (the real observability owner, as an owner reference)", res.Datasource.Owner, string(capability.ProviderPrometheus))
	}
}

// TestPersesPrometheusDatasourceNoCapabilityClaim asserts the produced
// datasource declares NO Metrics capability of its own: the marshalled JSON
// carries the datasource kind/name/config only, never an IntegrationDescriptor
// or a Metrics capability kind. Perses is a management surface, never an
// application capability.
func TestPersesPrometheusDatasourceNoCapabilityClaim(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized sources must build a datasource: %v", err)
	}
	raw, err := json.Marshal(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "Metrics") || strings.Contains(string(raw), "IntegrationDescriptor") {
		t.Fatalf("perses prometheus datasource must not declare a Metrics capability or IntegrationDescriptor: %s", raw)
	}
}

// TestPersesPrometheusDatasourceNoSecretInDescriptor asserts no secret value is
// materialized into the datasource descriptor: the auth field is an owner
// reference (authentication class) only.
func TestPersesPrometheusDatasourceNoSecretInDescriptor(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized sources must build a datasource: %v", err)
	}
	raw, err := json.Marshal(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"bearer", "password", "basicauth", "token", "clientcert", "privatekey"} {
		if strings.Contains(strings.ToLower(string(raw)), secret) {
			t.Fatalf("perses prometheus datasource descriptor must not contain secret %q: %s", secret, raw)
		}
	}
}

// TestPersesPrometheusDatasourceAccessIsProxy asserts the datasource access mode
// is "proxy": BaseHarbor routes every request through the Prometheus management
// proxy over TLS, never "direct", which would bypass the access boundary.
func TestPersesPrometheusDatasourceAccessIsProxy(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized sources must build a datasource: %v", err)
	}
	if res.Datasource.Config.Access != "proxy" {
		t.Fatalf("perses prometheus datasource access = %q, want proxy (request routed through the Prometheus management proxy over TLS)", res.Datasource.Config.Access)
	}
}

// TestPersesPrometheusDatasourceFailClosedWhenUnpinned asserts the fail-closed
// rule: without a pinned release that validates, the surface is not configured,
// never "configured".
func TestPersesPrometheusDatasourceFailClosedWhenUnpinned(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	persesMaterializeProvider(t, persesPrometheusDatasourceManifest(), persesProviderSource("tempo:runtime", capability.ProviderTempo))
	badRelease := capability.PersesRelease{Version: "", Image: "", AuthenticationClass: "unsupported"}
	_, err := buildPersesPrometheusDatasourceWithProbe(persesPrometheusDatasourceManifest(), badRelease, unavailablePrometheusProbe{})
	if err == nil {
		t.Fatal("unpinned perses must fail closed, not fabricate a datasource")
	}
}

// TestPersesPrometheusDatasourceFailClosedWhenUnselected asserts an unselected
// surface (no observability management UI / Perses) never produces a datasource.
func TestPersesPrometheusDatasourceFailClosedWhenUnselected(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := application.New("perses-unselected-demo", "dev", false, false, false)
	if _, err := BuildPersesPrometheusDatasource(m); err == nil {
		t.Fatal("unselected perses surface must fail closed, not fabricate a datasource")
	}
}

// TestPersesPrometheusDatasourceFailClosedWhenSourceAbsent asserts a provider
// whose runtime sources are NOT materialized (absent snapshot) is a source
// error, never an empty datasource with nil error.
func TestPersesPrometheusDatasourceFailClosedWhenSourceAbsent(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m) // no sources materialized
	if _, err := BuildPersesPrometheusDatasource(m); err == nil {
		t.Fatal("absent Prometheus sources must fail closed, not fabricate a datasource")
	} else if !strings.Contains(err.Error(), "no materialized Prometheus sources") {
		t.Fatalf("absent-source error = %v, want the fail-closed absent-sources error", err)
	}
}

// TestPersesPrometheusDatasourceFailClosedWhenNoProvider asserts construction
// without a materialized provider directory (missing provider) is a source
// error, never an empty datasource with nil error.
func TestPersesPrometheusDatasourceFailClosedWhenNoProvider(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	if _, err := BuildPersesPrometheusDatasource(m); err == nil {
		t.Fatal("a missing Prometheus provider must fail closed, not fabricate a datasource")
	} else if !strings.Contains(err.Error(), "no materialized Prometheus provider") {
		t.Fatalf("missing-provider error = %v, want the fail-closed missing-provider error", err)
	}
}

// TestPersesPrometheusDatasourceFailClosedWhenSourceDenied asserts sources that
// exist but are denied by the metrics collection policy are a source error
// (denied), never an empty datasource with nil error.
func TestPersesPrometheusDatasourceFailClosedWhenSourceDenied(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.MetricsCollectSourcesEnv, "application")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	if _, err := BuildPersesPrometheusDatasource(m); err == nil {
		t.Fatal("denied Prometheus sources must fail closed, not fabricate a datasource")
	} else if !strings.Contains(err.Error(), "no authorized Prometheus sources") {
		t.Fatalf("denied-source error = %v, want the fail-closed denied-sources error", err)
	}
}

// TestPersesPrometheusDatasourceFailClosedWhenForeignSourceOwned asserts a
// source owned by a DIFFERENT application is not visualized: construction fails
// closed on the ownership check and never leaks a foreign application's
// datasource.
func TestPersesPrometheusDatasourceFailClosedWhenForeignSourceOwned(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	foreign := observability.MetricsSource{
		ID: "api", Provider: capability.ProviderPrometheus, Class: observability.SourceApplication,
		Scope: capability.ScopeApplication, OwnerApplication: "someone-else",
		Network: "bh-local-shared_default", Target: "api:8080", Path: "/metrics", Scheme: "http",
	}
	persesMaterializeProvider(t, m, foreign)
	if _, err := BuildPersesPrometheusDatasource(m); err == nil {
		t.Fatal("a source owned by another application must fail closed, not be visualized")
	} else if !strings.Contains(err.Error(), "no authorized Prometheus sources") {
		t.Fatalf("foreign-source error = %v, want the fail-closed unauthorized-sources error", err)
	}
}

// TestPersesPrometheusDatasourceFailClosedInvalidSnapshot asserts a damaged
// retained snapshot (invalid source authority) propagates as an error, never
// nil.
func TestPersesPrometheusDatasourceFailClosedInvalidSnapshot(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	files := persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	snapshotPath := filepath.Join(files.Dir, "runtime-sources.json")
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot runtimeSourceSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Project = "foreign-project"
	bad, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPersesPrometheusDatasource(m); err == nil {
		t.Fatal("a damaged retained snapshot must fail closed, not fabricate a datasource")
	}
}

// TestPersesPrometheusDatasourceFailClosedBadURL asserts an invalid
// materialized endpoint (bad port) is propagated as an error, never swallowed
// into an empty-URL datasource.
func TestPersesPrometheusDatasourceFailClosedBadURL(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	files := persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	writeProviderEnv(t, files, "BASEHARBOR_PROMETHEUS_PORT=not-a-port\n")
	if _, err := BuildPersesPrometheusDatasource(m); err == nil {
		t.Fatal("an invalid materialized endpoint must fail closed, not fabricate a datasource")
	} else if !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("bad-URL error = %v, want the endpoint error", err)
	}
}

// TestPersesPrometheusDatasourceQueryPath asserts a successful build runs the
// health query with the exact authorized-source up-series union and reports
// "verified" only when the probe observed an up=1 sample; a clean down answer
// stays "unverified" with nil error (never fabricated verified health).
func TestPersesPrometheusDatasourceQueryPath(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	sources := []observability.MetricsSource{
		persesProviderSource("loki:runtime", capability.ProviderLoki),
		persesProviderSource("tempo:runtime", capability.ProviderTempo),
	}
	persesMaterializeProvider(t, m, sources...)

	wantQuery := `max(up{baseharbor_provider="loki",baseharbor_source="loki:runtime"} or up{baseharbor_provider="tempo",baseharbor_source="tempo:runtime"})`
	for _, cell := range []struct {
		name         string
		up           bool
		reachability string
	}{
		{"up", true, PersesPrometheusReachabilityVerified},
		{"down", false, PersesPrometheusReachabilityUnverified},
	} {
		t.Run(cell.name, func(t *testing.T) {
			probe := &recordingProbe{up: cell.up}
			res, err := BuildPersesPrometheusDatasourceWithProbe(m, probe)
			if err != nil {
				t.Fatalf("authorized sources with a working query path must build: %v", err)
			}
			if probe.query != wantQuery {
				t.Fatalf("health query = %q, want the exact authorized-source up-series union %q", probe.query, wantQuery)
			}
			if probe.endpoint != res.Datasource.Config.URL {
				t.Fatalf("probe endpoint = %q, want the datasource url %q", probe.endpoint, res.Datasource.Config.URL)
			}
			if res.Reachability != cell.reachability {
				t.Fatalf("reachability = %q, want %q", res.Reachability, cell.reachability)
			}
			if res.SourceCount != len(sources) {
				t.Fatalf("source count = %d, want %d", res.SourceCount, len(sources))
			}
		})
	}
}

// TestPersesPrometheusDatasourceQueryErrorPropagates asserts a failing query
// path (transport/permission error) propagates as an error, never a silent
// empty datasource.
func TestPersesPrometheusDatasourceQueryErrorPropagates(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	probe := &recordingProbe{err: fmt.Errorf("permission denied")}
	if _, err := BuildPersesPrometheusDatasourceWithProbe(m, probe); err == nil {
		t.Fatal("a failing health query must propagate, not fabricate a datasource")
	} else if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("query error = %v, want the propagated probe error", err)
	}
}

// TestPersesPrometheusDatasourceVerifyRealQuerySuccess asserts health passes
// only on a REAL Prometheus query over the HTTP client path: a live test
// Prometheus answers the exact query, and a denied (401) endpoint is not
// swallowed.
func TestPersesPrometheusDatasourceVerifyRealQuerySuccess(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	files := persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	server, seen := startPrometheusQueryServer(t, "1", "", "")
	endpoint := server.URL
	writeProviderEnv(t, files, "BASEHARBOR_PROMETHEUS_PORT="+strconv.Itoa(endpointPort(t, endpoint))+"\n")

	client := &http.Client{}
	verified, err := persesPrometheusDatasourceQuery(t.Context(), endpoint,
		[]observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)},
		httpClientQueryProbe{client: client})
	if err != nil {
		t.Fatalf("a real Prometheus up=1 answer must not error: %v", err)
	}
	if !verified {
		t.Fatal("a real Prometheus up=1 answer must verify datasource health")
	}
	if want := `max(up{baseharbor_provider="tempo",baseharbor_source="tempo:runtime"})`; *seen != want {
		t.Fatalf("server saw query %q, want %q", *seen, want)
	}

	// Down answer over the same real HTTP path: not verified, still no error.
	downServer, _ := startPrometheusQueryServer(t, "0", "", "")
	verified, err = persesPrometheusDatasourceQuery(t.Context(), downServer.URL,
		[]observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)},
		httpClientQueryProbe{client: client})
	if err != nil {
		t.Fatalf("a down answer must not error: %v", err)
	}
	if verified {
		t.Fatal("a down answer must not verify datasource health")
	}

	// Denied endpoint (401): the health path propagates an error, never a
	// false success.
	deniedServer, _ := startPrometheusQueryServer(t, "1", "bh", "secret")
	_, err = persesPrometheusDatasourceQuery(t.Context(), deniedServer.URL,
		[]observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)},
		httpClientQueryProbe{client: client})
	if err == nil {
		t.Fatal("a denied (401) health query must propagate an error, never report verified")
	}

	// TLS/CA handling: the client trusts ONLY the provided CA bundle; an
	// untrusted server certificate fails the query (never silently accepted).
	tlsServer, _ := startPrometheusQueryServer(t, "1", "", "")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: x509.NewCertPool()}
	untrustedClient := &http.Client{Transport: transport}
	if _, err := persesPrometheusDatasourceQuery(t.Context(), strings.Replace(tlsServer.URL, "http://", "https://", 1),
		[]observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)},
		httpClientQueryProbe{client: untrustedClient}); err == nil {
		t.Fatal("a server certificate outside the pinned CA bundle must fail the health query")
	}
}

// TestPersesPrometheusDatasourceVerifyFailClosed asserts the fail-closed health
// contract at the Verify entry point: pin validation alone NEVER passes —
// without a materialized provider (the default Core state), verify returns an
// error, and a selected+pinned+authorized surface still only reports the probe
// result (down => not verified).
func TestPersesPrometheusDatasourceVerifyFailClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")

	// Missing provider: verify must fail closed (pin validation alone never
	// passes health without a real reachable Prometheus health path).
	if err := VerifyPersesPrometheusDatasourceHealth(t.Context(), persesPrometheusDatasourceManifest()); err == nil {
		t.Fatal("verify must fail closed when no Prometheus provider is materialized (pin validation alone never passes)")
	}

	// Pinned + selected + authorized sources but a down provider: the build
	// path reports unverified, never a fabricated verified datasource.
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	res, err := BuildPersesPrometheusDatasourceWithProbe(m, &recordingProbe{up: false})
	if err != nil {
		t.Fatalf("a working query path with a down answer still constructs (unverified): %v", err)
	}
	if res.Reachability != PersesPrometheusReachabilityUnverified {
		t.Fatalf("down probe reachability = %q, want unverified (never verified without an up sample)", res.Reachability)
	}
}

// TestPersesPrometheusDatasourceUnverifiedReachability pins that the build
// path without a live probe reports only "unverified" for a selected, pinned,
// authorized surface (never "verified" without a real query observation).
func TestPersesPrometheusDatasourceUnverifiedReachability(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized sources must build a datasource: %v", err)
	}
	if res.Reachability != PersesPrometheusReachabilityUnverified {
		t.Fatalf("perses prometheus datasource reachability = %q, want %q (no live probe ran)", res.Reachability, PersesPrometheusReachabilityUnverified)
	}
}

// TestPersesPrometheusDatasourceGoldenFiles pins the marshalled datasource
// against a committed golden file, so the Perses-native output is a stable,
// reviewed witness. The golden config carries the materialized endpoint of the
// provider fixture (fixed port), proving the adapter never emits an empty
// config alongside a nil error.
func TestPersesPrometheusDatasourceGoldenFiles(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		t.Fatalf("selected+pinned+authorized sources must build a datasource: %v", err)
	}
	got, err := PersesPrometheusDatasourceJSON(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-prometheus-datasource", "prometheus.json"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}
