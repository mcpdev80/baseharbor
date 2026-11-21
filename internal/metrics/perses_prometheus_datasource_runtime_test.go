package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// The tests in this file prove the RUNTIME provisioning side of the datasource
// contract (0428-083 correction round): the validated datasource is not just
// built and golden-marshalled, it is materialized where the running Perses
// consumes it (config.yaml + provisioning folder), it fails closed there, and
// the provisioned proxy endpoint answers a real query over the TLS boundary.

// persesDatasourceReadJSON reads a materialized provisioning artifact.
func persesDatasourceReadJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provisioned artifact %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("provisioned artifact %s is not valid JSON: %v\n%s", path, err, data)
	}
	return doc
}

// datasourceNested digs a JSON object path out of a decoded document.
func datasourceNested(t *testing.T, doc map[string]any, path ...string) any {
	t.Helper()
	var cur any = doc
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %v: not an object at %q", path, key)
		}
		cur, ok = m[key]
		if !ok {
			t.Fatalf("path %v: missing key %q in %v", path, key, m)
		}
	}
	return cur
}

// TestPersesPrometheusDatasourceRuntimeProvisioning is the runtime-level test:
// the authorized datasource appears in the rendered/joined Perses configuration
// the runtime service consumes. It proves the whole chain fail-closed: build ->
// config.yaml -> provisioning folder -> the exact GlobalDatasource resource,
// joined against the SAME application manifest the runtime would start, and it
// pins that the artifacts carry the management endpoint, the proxy access mode,
// the pinned native plugin kind and NO secret value.
func TestPersesPrometheusDatasourceRuntimeProvisioning(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m,
		persesProviderSource("loki:runtime", capability.ProviderLoki),
		persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, configPath, resourcePath, err := MaterializePersesPrometheusDatasourceAt(m, "", "")
	if err != nil {
		t.Fatalf("the selected, pinned, authorized datasource must provision: %v", err)
	}
	if res.SourceCount != 2 {
		t.Fatalf("provisioned source count = %d, want the two authorized runtime sources", res.SourceCount)
	}

	// (1) The config file the Perses command line consumes (--config=
	// /etc/perses/config.yaml) declares the provisioning folder the server
	// loads at startup, with a bounded reconcile interval and a file database
	// (provisioning injects into the database).
	configDoc := persesDatasourceReadJSON(t, configPath)
	if got := filepath.Base(configPath); got != "config.yaml" {
		t.Fatalf("config artifact = %q, want config.yaml (the --config file name)", got)
	}
	folders, ok := datasourceNested(t, configDoc, "provisioning", "folders").([]any)
	if !ok || len(folders) != 1 || folders[0] != "/etc/perses/provisioning" {
		t.Fatalf("config provisioning.folders = %v, want the mounted provisioning folder /etc/perses/provisioning", folders)
	}
	if interval := datasourceNested(t, configDoc, "provisioning", "interval"); interval != "300s" {
		t.Fatalf("config provisioning.interval = %v, want the bounded reconcile interval 300s", interval)
	}
	if ext := datasourceNested(t, configDoc, "database", "file", "extension"); ext != "json" {
		t.Fatalf("config database.file.extension = %v, want json (the provisioning resource format)", ext)
	}

	// (2) The resource file in the provisioning folder is the joined, native
	// GlobalDatasource: the pinned native plugin kind, the default flag, the
	// materialized management endpoint behind the HTTPProxy (never direct), and
	// the pinned Prometheus API allowlist.
	resourceDoc := persesDatasourceReadJSON(t, resourcePath)
	if kind := datasourceNested(t, resourceDoc, "kind"); kind != PersesDatasourceProvisioningKind {
		t.Fatalf("resource kind = %v, want %q (the provisioning resource kind the loader resolves)", kind, PersesDatasourceProvisioningKind)
	}
	if name := datasourceNested(t, resourceDoc, "metadata", "name"); name != PersesDatasourceProvisioningResourceName {
		t.Fatalf("resource metadata.name = %v, want %q", name, PersesDatasourceProvisioningResourceName)
	}
	if pluginKind := datasourceNested(t, resourceDoc, "spec", "plugin", "kind"); pluginKind != PersesPrometheusDatasourceKind {
		t.Fatalf("resource plugin.kind = %v, want the pinned native plugin %q", pluginKind, PersesPrometheusDatasourceKind)
	}
	if isDefault := datasourceNested(t, resourceDoc, "spec", "default"); isDefault != true {
		t.Fatalf("resource spec.default = %v, want true (the datasource of the selected observability surface)", isDefault)
	}
	if proxyKind := datasourceNested(t, resourceDoc, "spec", "plugin", "spec", "proxy", "kind"); proxyKind != PersesDatasourceProxyKind {
		t.Fatalf("resource proxy.kind = %v, want %q (server-side proxy access, never browser-direct)", proxyKind, PersesDatasourceProxyKind)
	}
	// The proxy URL is the Prometheus GATEWAY SERVICE name over the runtime
	// TLS boundary (0428-083 F2) — the one stable identity of the endpoint,
	// not a host-loopback address that is not reachable from the Perses
	// container's network namespace.
	if url := datasourceNested(t, resourceDoc, "spec", "plugin", "spec", "proxy", "spec", "url"); url != "https://"+ProviderNetworkEndpointHost {
		t.Fatalf("resource proxy url = %v, want the gateway service endpoint %q", url, "https://"+ProviderNetworkEndpointHost)
	}
	if secretRef := datasourceNested(t, resourceDoc, "spec", "plugin", "spec", "proxy", "spec", "secret"); secretRef != PersesDatasourceTLSSecretName {
		t.Fatalf("resource proxy secret reference = %v, want the provisioned GlobalSecret %q (F3)", secretRef, PersesDatasourceTLSSecretName)
	}
	if _, hasDirect := datasourceNested(t, resourceDoc, "spec", "plugin", "spec").(map[string]any)["directUrl"]; hasDirect {
		t.Fatal("the provisioned datasource must not carry a direct URL (the access boundary is the proxy)")
	}
	endpoints, ok := datasourceNested(t, resourceDoc, "spec", "plugin", "spec", "proxy", "spec", "allowedEndpoints").([]any)
	if !ok || len(endpoints) != len(persesPrometheusAllowedEndpoints) {
		t.Fatalf("allowedEndpoints = %v, want the pinned %d-entry Prometheus API allowlist", endpoints, len(persesPrometheusAllowedEndpoints))
	}
	if first := endpoints[0].(map[string]any); first["endpointPattern"] != "/api/v1/labels" || first["method"] != "POST" {
		t.Fatalf("allowedEndpoints[0] = %v, want the pinned Prometheus API entry", first)
	}

	// (3) No secret material: the provisioning artifacts carry endpoint and
	// owner references only.
	for _, path := range []string{configPath, resourcePath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"bearer", "password", "basicauth", "token", "clientcert", "privatekey", "authorization"} {
			if strings.Contains(strings.ToLower(string(data)), secret) {
				t.Fatalf("provisioned artifact %s must not contain secret %q: %s", path, secret, data)
			}
		}
	}

	// (4) The mount sources the Perses runtime service consumes: provider-root
	// relative paths, joined against the same manifest the runtime starts.
	configSource, provisioningSource, trustBundleSource, err := PersesDatasourceProviderPaths("", "", m)
	if err != nil {
		t.Fatalf("provider paths: %v", err)
	}
	if trustBundleSource == "" {
		t.Fatal("the datasource projection must name its public trust bundle path")
	}
	// F1: with no namespace the deployment's runtime dir is
	// <dataRoot>/<app>/runtime, and the datasource root is its
	// providers/management-ui/perses sibling — the single layout formula
	// (application.PersesDatasourceRuntimeRoot), asserted as the formula, not
	// a duplicated path.
	if wantSource := application.PersesDatasourceRuntimeRoot(
		application.RuntimeFilesFor(application.Store{Root: state}, m).Dir) + "/config.yaml"; configSource != wantSource {
		t.Fatalf("config mount source = %q, want the runtime-sibling datasource root %q", configSource, wantSource)
	}
	if !strings.HasSuffix(provisioningSource, "/providers/management-ui/perses/provisioning") ||
		!strings.HasPrefix(configSource, strings.TrimSuffix(provisioningSource, "/provisioning")) {
		t.Fatalf("provisioning mount source = %q, want the sibling provisioning folder of %q", provisioningSource, configSource)
	}
}

// TestPersesPrometheusDatasourceRuntimeProvisioningFailClosed proves the
// provisioning path consumes the SAME builder contract as the descriptor: when
// the selection, authorization or materialized sources deny the datasource, the
// RUNTIME artifacts are removed (never left stale) and the call fails closed.
func TestPersesPrometheusDatasourceRuntimeProvisioningFailClosed(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")

	// Authorized: provision once so real artifacts exist to be removed.
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	_, configPath, resourcePath, err := MaterializePersesPrometheusDatasourceAt(m, "", "")
	if err != nil {
		t.Fatalf("authorized materialize: %v", err)
	}
	// Source authorization revoked: the policy collects only the
	// application-provider class, the single platform-provider source is denied,
	// the builder fails closed and the artifacts must disappear.
	t.Setenv(application.MetricsCollectSourcesEnv, "application-provider")
	if _, _, _, err := MaterializePersesPrometheusDatasourceAt(m, "", ""); err == nil {
		t.Fatal("revoked source authorization must fail closed on the provisioning path")
	}
	for _, path := range []string{configPath, resourcePath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale provisioning artifact after fail-closed: %s", path)
		}
	}
	// The provisioning folder itself is gone (RemoveAll of the projection root).
	if _, err := os.Stat(filepath.Dir(resourcePath)); !os.IsNotExist(err) {
		t.Fatalf("revoked authorization left the provisioning folder behind: %v", err)
	}

	// Surface deselected: materializing must fail closed and remove everything.
	t.Setenv(application.MetricsCollectSourcesEnv, "")
	unselected := application.New("perses-unselected-demo", "dev", false, false, false)
	if _, _, _, err := MaterializePersesPrometheusDatasourceAt(unselected, state, ""); err == nil {
		t.Fatal("unselected surface must fail closed")
	}
	root, err := persesDatasourceProjectionRoot(state, "", unselected)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("deselected surface left the provisioning root behind: %v", err)
	}
}

// TestPersesPrometheusDatasourceRuntimeProxyQueryTLS is the isolated runtime
// query/proxy/TLS check: it starts a real HTTPS Prometheus-shaped query
// endpoint (management TLS boundary with a private CA), materializes the
// datasource against it through the provisioning path, and proves (a) the
// provisioned proxy URL is exactly the materialized https management endpoint
// of that provider, (b) the datasource health query over a client pinned to the
// management CA verifies the authorized up=1 series, and (c) a client outside
// that CA bundle fails the query closed — the datasource never silently
// bypasses the TLS boundary.
func TestPersesPrometheusDatasourceRuntimeProxyQueryTLS(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	files := persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	var seenMu sync.Mutex
	var seen string
	server := startDatasourceQueryTLSserver(t, &seenMu, &seen)
	cert := server.Certificate()
	_, portString, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	if err != nil {
		t.Fatalf("test endpoint: %v", err)
	}
	writeProviderEnv(t, files, "BASEHARBOR_PROMETHEUS_PORT="+portString+"\n")

	res, _, resourcePath, err := MaterializePersesPrometheusDatasourceAt(m, "", "")
	if err != nil {
		t.Fatalf("materialize against a live HTTPS Prometheus: %v", err)
	}
	if res.SourceCount != 1 || res.Datasource.Config.Access != "proxy" {
		t.Fatalf("provisioned datasource contract = %+v, want the one authorized proxy-access source", res)
	}
	// (a) The provisioned proxy URL is the gateway service endpoint (F2),
	// independent of the published port of this materialization.
	got := datasourceNested(t, persesDatasourceReadJSON(t, resourcePath), "spec", "plugin", "spec", "proxy", "spec", "url").(string)
	want := "https://" + ProviderNetworkEndpointHost
	if got != want {
		t.Fatalf("provisioned proxy url = %q, want the gateway service endpoint %q", got, want)
	}
	want = "https://127.0.0.1:" + portString
	if _, hasDirect := datasourceNested(t, persesDatasourceReadJSON(t, resourcePath), "spec", "plugin", "spec").(map[string]any)["directUrl"]; hasDirect {
		t.Fatal("the provisioned datasource must never carry a direct URL")
	}

	// (b) The datasource health query over the management TLS boundary: the
	// client is pinned to the CA that signed the server certificate and the
	// Host stays the certificate name (the Prometheus service-access contract).
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	trusted := withPrometheusHostHeader(&http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool},
	}}, "prometheus")
	verified, err := persesPrometheusDatasourceQuery(context.Background(), want,
		[]observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)},
		httpClientQueryProbe{client: trusted})
	if err != nil {
		t.Fatalf("a client pinned to the management CA must succeed: %v", err)
	}
	if !verified {
		t.Fatal("an up=1 answer over the management TLS boundary must verify the datasource")
	}
	seenMu.Lock()
	wantQuery := `max(up{baseharbor_provider="tempo",baseharbor_source="tempo:runtime"})`
	if seen != wantQuery {
		t.Fatalf("runtime endpoint saw query %q, want the authorized source up-series %q", seen, wantQuery)
	}
	seenMu.Unlock()

	// (c) An untrusted client (empty root pool) fails closed at the TLS layer.
	untrusted := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool()},
	}}
	if _, err := persesPrometheusDatasourceQuery(context.Background(), want,
		[]observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)},
		httpClientQueryProbe{client: untrusted}); err == nil {
		t.Fatal("a client outside the management CA bundle must fail the TLS query, never verify through it")
	}
}

func TestPersesPrometheusDatasourceGlobalDatasourceGoldenFile(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	res, err := buildPersesPrometheusDatasourceContextual(m, capability.PersesDefaultRelease,
		unavailablePrometheusProbe{}, "", "", buildContextRuntime)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	resource, err := NewPersesGlobalDatasourceResource(res)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	got, err := PersesGlobalDatasourceResourceJSON(resource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-prometheus-datasource", "globaldatasource.json"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

func TestPersesPrometheusDatasourceConfigGoldenFile(t *testing.T) {
	got, err := PersesDatasourceConfigJSON(NewPersesDatasourceConfig())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-prometheus-datasource", "config.yaml"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

func TestPersesPrometheusDatasourceProxyURLFailClosed(t *testing.T) {
	for _, bad := range []string{"", "not-a-url", "prometheus:39901", "https://user:pass@127.0.0.1:39901", "https://127.0.0.1:39901/api", "https://127.0.0.1:39901?x=1", "http://[::1", "https://127.0.0.1:99999/x"} {
		if _, err := persesDatasourceProxyURL(bad); err == nil {
			t.Fatalf("endpoint %q must fail closed, never provision a proxy URL", bad)
		}
	}
	for _, good := range []string{"http://127.0.0.1:39901", "https://127.0.0.1:39901"} {
		got, err := persesDatasourceProxyURL(good)
		if err != nil || got != good {
			t.Fatalf("endpoint %q must materialize exactly: %q %v", good, got, err)
		}
	}
}

// trustedProbe adapts a configured *http.Client to the package query probe.
func trustedProbe(client *http.Client) prometheusQueryProbe {
	return httpClientQueryProbe{client: client}
}

// startDatasourceQueryTLSserver serves the Prometheus query API over HTTPS
// with a freshly issued self-signed server certificate (the management TLS
// boundary witness). httptest already serves a trusted-by-pool certificate and
// exposes it through Server.Certificate().
func startDatasourceQueryTLSserver(t *testing.T, mu *sync.Mutex, seen *string) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		*seen = r.URL.Query().Get("query")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"1"]}]}}`)
	}))
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}
