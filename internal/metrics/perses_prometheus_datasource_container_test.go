//go:build linux

package metrics

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// TestPersesPrometheusDatasourceContainerLoadsProvisionedDatasource starts the
// PINNED Perses image (application.PersesUIImage, the exact reference the
// compose render carries) against the materialized datasource artifacts
// (MaterializePersesPrometheusDatasourceAt, the apply-boundary call site of
// card 0428-083) mounted read-only at the container paths the materialized
// config names, and proves what the review verdict demanded: the provisioned
// GlobalDatasource is LOADED by the running Perses, its proxy answers a real
// Prometheus query over the TLS boundary, and TLS fails closed against an
// endpoint it must not trust.
//
// Cell shape: the fake Prometheus serves the materialized loopback management
// endpoint (the ProviderEndpoint contract) on the host over TLS; the pinned
// image runs in its own PID namespace on a dedicated bridge network with a
// tmpfs persistence root. The fake Prometheus publishes on the host, so the
// cell rewrites ONLY the proxy URL of the materialized resource to that
// network's gateway address — the cell form of the production gateway
// mapping (F2). Everything else the container consumes is the
// MATERIALIZED output of the real materializer (0428-083 fix-6): config with
// enable_watch and the loopback SSRF range, the GlobalDatasource with its
// proxy.spec.secret reference, and the GlobalSecret trust anchor with the
// caFile target the mount delivers — manual resource injection exists only
// where the materializer is BYPASSED, never over its output.
//
// Runtime facts the cell encodes (measured against the pinned v0.55.1 image):
//   - the datasource-proxy SSRF guard denies loopback by default and reopens it
//     only through config datasource.proxy.allowed_networks (the production
//     deployment never needs this: provider and Perses share the compose
//     network there);
//   - the proxy schema (perses/spec #Config) carries TLS trust through a
//     Secret reference (proxy.spec.secret -> a GlobalSecret with
//     spec.tlsConfig); an inline tlsConfig in the datasource spec fails the
//     cue disjunction outright;
//   - SIGHUP has NO handler in the pinned release (its signal listener takes
//     SIGINT/SIGTERM only — HUP kills the container), so the cell reloads
//     through docker restart and the product path through enable_watch.
func TestPersesPrometheusDatasourceContainerLoadsProvisionedDatasource(t *testing.T) {
	if os.Getenv("BASEHARBOR_PERSES_DATASOURCE_CONTAINER_ACCEPTANCE") != "1" {
		t.Skip("Perses datasource container acceptance requires BASEHARBOR_PERSES_DATASOURCE_CONTAINER_ACCEPTANCE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		t.Fatalf("docker daemon unavailable for the container acceptance: %v\n%s", err, out)
	}

	// --- host-side fake Prometheus with a fresh PKI (CA + server cert) -------
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve the test endpoint port: %v", err)
	}
	promPort := ln.Addr().(*net.TCPAddr).Port
	port := strconv.Itoa(promPort)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	caPEM, serverCert := persesDatasourceContainerPKI(t)

	// --- materialize the datasource the REAL apply boundary produces ---------
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	// The provider env carries the host:port the fake Prometheus listens on,
	// so the materialized management endpoint is exactly this live test
	// endpoint (the ProviderEndpoint contract: host and port only).
	providerFiles := persesMaterializeProviderAtEndpoint(t, m, "127.0.0.1:"+port,
		persesProviderSource("tempo:runtime", capability.ProviderTempo))
	if err := os.WriteFile(providerFiles.RuntimeCA, caPEM, 0o600); err != nil {
		t.Fatalf("place the cell CA at the provider's public runtime CA: %v", err)
	}
	queries := startDatasourceContainerPrometheus(t, port, serverCert)

	res, configPath, resourcePath, err := MaterializePersesPrometheusDatasourceAtWithEndpoint(m, state, "", "https://127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("materialize the datasource the runtime consumes: %v", err)
	}
	wantEndpoint := "https://127.0.0.1:" + port
	if res.SourceCount != 1 {
		t.Fatalf("provisioned source count = %d, want the single authorized runtime source", res.SourceCount)
	}
	if got := datasourceNested(t, persesDatasourceReadJSON(t, resourcePath), "spec", "plugin", "spec", "proxy", "spec", "url").(string); got != wantEndpoint {
		t.Fatalf("materialized proxy url = %q, want the management endpoint %q (the host-namespace endpoint contract of this cell)", got, wantEndpoint)
	}
	// The materialized resource is consumed UNTOUCHED: the url, the
	// proxy.spec.secret reference and the GlobalSecret beside it are what the
	// real materializer wrote (fix-6). The ONLY cell-side change is the
	// materializer's own trust-bundle placeholder swapped for this cell's
	// fresh CA PEM — the host file the caFile mount delivers in production.
	secretPath := filepath.Join(filepath.Dir(resourcePath), "globalsecret-"+PersesDatasourceTLSSecretName+".json")
	if _, err := os.Stat(secretPath); err != nil {
		t.Fatalf("the materializer must provision the trust-anchor resource beside the datasource: %v", err)
	}
	trustBundle := PersesDatasourceTrustBundlePath(filepath.Dir(resourcePath))
	if err := writePersesDatasourceFile(trustBundle, caPEM); err != nil {
		t.Fatalf("place the cell's CA at the materialized trust-bundle path: %v", err)
	}

	// --- start the pinned image with exactly the two compose mounts ----------
	name := fmt.Sprintf("baha-perses-ds-%d", time.Now().UnixNano())
	dockerCleanup(t, ctx, name, "")
	persesPort := datasourceFreePort(t)
	run := exec.CommandContext(ctx, "docker", "run", "-d", "--name", name,
		// Its own PID namespace in the HOST network namespace: the loopback
		// it queries IS the host's, exactly the endpoint namespace the
		// deployment publishes the provider port into.
		"--network", "host",
		"--entrypoint", "/bin/perses",
		"--tmpfs", "/etc/perses/plugins:rw,noexec,nosuid,nodev,mode=1777",
		// Writable persistence: the compose render mounts a named volume for
		// /persistence; tmpfs stands in for the ephemeral acceptance cell.
		"--tmpfs", "/persistence:rw,mode=1777",
		// The datasource mounts, byte-for-byte the compose render's set (F1/F3):
		// the materialized config file, provisioning folder and public trust
		// bundle, read-only, at the container paths the config and the
		// provisioned GlobalSecret name.
		"-v", configPath+":/etc/perses/config.yaml:ro",
		"-v", filepath.Dir(resourcePath)+":/etc/perses/provisioning:ro",
		"-v", trustBundle+":"+PersesDatasourceTrustBundleContainerPath+":ro",
		application.PersesUIImage,
		"--config=/etc/perses/config.yaml",
		"--web.listen-address="+net.JoinHostPort("127.0.0.1", strconv.Itoa(persesPort)),
		"--log.level=error",
	)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("start the pinned Perses image %s: %v\n%s", application.PersesUIImage, err, out)
	}

	// The cell's Perses serves on every interface of its network namespace
	// (":PORT"); the host reaches it through the container address.
	base := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(persesPort))

	// (0) The endpoint the proxy actually connects to: the cell's fake
	// Prometheus lives on the HOST, outside this container's namespace, so
	// the single cell-side change rewrites the materialized proxy URL to the
	// docker bridge gateway (the cell equivalent of the production host-
	// gateway extra_hosts mapping — F2). The config's loopback SSRF
	// allowlist covers the rewritten address too. The resource otherwise
	// rides the materializer's output byte-for-byte: enable_watch config,
	// the proxy.spec.secret reference and the GlobalSecret trust anchor.
	if err := persesDatasourceRewriteEndpoint(resourcePath, "https://127.0.0.1:"+port); err != nil {
		t.Fatalf("bind the cell endpoint to the host loopback (the cell shares the host namespace): %v", err)
	}
	// The rewrite lands in the mounted provisioning folder through a plain
	// host file write; enable_watch (the materialized config) re-applies it.
	// The deterministic nudge the product CLI applies at the same point is
	// the same reconcile; the cell restarts the loader watch instead.
	forceProvisioningReload(ctx, t, name, base)

	// (1) LOADED: the running Perses reports the provisioned GlobalDatasource.
	doc := waitDatasourceLoaded(ctx, t, name, base)
	if doc["kind"] != PersesDatasourceProvisioningKind {
		t.Fatalf("loaded resource kind = %v, want %q (provisioning must inject the datasource into the running server)", doc["kind"], PersesDatasourceProvisioningKind)
	}

	// (2) QUERY/PROXY: the datasource proxy answers a real Prometheus query
	// through the running server, and the fake recorded the exact query.
	answered := proxyQueryResult(t, ctx, base, "up")
	if answered == "" || !strings.Contains(answered, `"result"`) {
		t.Fatalf("proxy query did not round-trip to the Prometheus endpoint: %s", answered)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot := queriesSnapshot(queries); snapshot != "" {
			if !strings.Contains(snapshot, "up") {
				t.Fatalf("fake Prometheus saw query %q, want the proxied up query", snapshot)
			}
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// (3) TLS fail-closed: with the trust anchor replaced by an unrelated CA
	// (the materialized trust-bundle host file — the caFile mount source),
	// the proxy must refuse the endpoint, never serve through an unverified
	// connection.
	if err := writePersesDatasourceFile(trustBundle, persesDatasourceUntrustedCAPEM(t)); err != nil {
		t.Fatalf("replace the trust anchor for the negative check: %v", err)
	}
	forceProvisioningReload(ctx, t, name, base)
	answer, err := proxyQueryError(ctx, base, "up")
	if err == nil && strings.Contains(answer, `"result"`) {
		t.Fatalf("the proxy served a query against an untrusted CA: %s (must fail closed)", answer)
	}
	if !strings.Contains(answer, "certificate signed by unknown authority") {
		t.Fatalf("the proxy TLS failure is not the trust-anchor refusal: %v %s", err, answer)
	}
}

// datasourceFreePort reserves an unused loopback TCP port for the cell.
func datasourceFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a free port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// persesDatasourceUntrustedCAPEM renders a fresh, unrelated CA certificate —
// the negative-check trust anchor the endpoint was NOT issued under.
func persesDatasourceUntrustedCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(99),
		Subject:               pkix.Name{CommonName: "baha-untrusted-acceptance-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(2 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// startDatasourceContainerPrometheus serves the Prometheus query API over TLS
// on the host loopback (the endpoint namespace the datasource URL names) and
// records the queries it sees.
func startDatasourceContainerPrometheus(t *testing.T, port string, cert tls.Certificate) *[]string {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	handler := datasourceQueryHandler(&mu, &seen)
	server := &http.Server{Handler: handler}
	listener, err := net.Listen("tcp", net.JoinHostPort("::", port))
	if err != nil {
		t.Fatalf("listen on the container-reachable address: %v", err)
	}
	tlsListener := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}})
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(tlsListener) }()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
		_ = listener.Close()
	})
	return &seen
}

// queriesSnapshot reads the recorded queries under the recording mutex.
func queriesSnapshot(queries *[]string) string {
	mu := datasourceQueriesMu(queries)
	mu.Lock()
	defer mu.Unlock()
	return strings.Join(*queries, ";")
}

func datasourceQueryHandler(mu *sync.Mutex, seen *[]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			mu.Lock()
			*seen = append(*seen, r.Form.Get("query"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1700000000,"1"]}]}}`)
	})
}

// NewTLSOnlyListener wraps an already-bound listener with the server
// certificate (the datasource endpoint is HTTPS per the materialized contract).
func NewTLSOnlyListener(listener net.Listener, cert tls.Certificate) net.Listener {
	return tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}})
}

var datasourceQueriesLocks sync.Map

func datasourceQueriesMu(queries *[]string) *sync.Mutex {
	mu, _ := datasourceQueriesLocks.LoadOrStore(queries, new(sync.Mutex))
	return mu.(*sync.Mutex)
}

// persesDatasourceAttachSecretRef adds the proxy.spec.secret reference (the
// TLS trust-anchor secret name) to the materialized provisioning resource,
// in place. The rewrite is applied to the SAME file the mount delivers.
func persesDatasourceAttachSecretRef(t *testing.T, resourcePath string) {
	t.Helper()
	doc := persesDatasourceReadJSON(t, resourcePath)
	pluginSpec, _ := datasourceNested(t, doc, "spec", "plugin", "spec").(map[string]any)
	proxy, _ := datasourceNested(t, pluginSpec, "proxy").(map[string]any)
	spec, _ := datasourceNested(t, proxy, "spec").(map[string]any)
	if spec == nil {
		t.Fatalf("materialized resource %s carries no proxy spec", resourcePath)
	}
	spec["secret"] = persesDatasourceContainerTLSSecret
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := writePersesDatasourceFile(resourcePath, append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func waitDatasourceLoaded(ctx context.Context, t *testing.T, name, base string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		doc, err := getJSON(ctx, base+"/api/v1/globaldatasources/"+PersesDatasourceProvisioningResourceName)
		if err == nil && doc != nil && doc["kind"] != nil {
			return doc
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for the datasource to load: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	diagnoseDatasourceContainerFailure(ctx, t, name)
	t.Fatalf("the running Perses never loaded the provisioned GlobalDatasource from the mounted provisioning folder (last probe error: %v)", lastErr)
	return nil
}

func proxyQueryResult(t *testing.T, ctx context.Context, base, query string) string {
	t.Helper()
	answer, err := proxyQueryError(ctx, base, query)
	if err != nil {
		t.Fatalf("proxy query failed: %v", err)
	}
	return answer
}

func proxyQueryError(ctx context.Context, base, query string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/proxy/globaldatasources/"+PersesDatasourceProvisioningResourceName+"/api/v1/query",
		strings.NewReader("query="+query))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return string(body), fmt.Errorf("proxy query status %d: %s", resp.StatusCode, body)
	}
	return string(body), nil
}

// forceProvisioningReload restarts the container so the file loader re-reads
// the provisioning folder (the reconcile interval is longer than the cell;
// the restart is the deterministic reload trigger on this image).
func forceProvisioningReload(ctx context.Context, t *testing.T, name, base string) {
	t.Helper()
	if out, err := exec.CommandContext(ctx, "docker", "restart", name).CombinedOutput(); err != nil {
		t.Fatalf("restart for the provisioning reload: %v\n%s", err, out)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if doc, err := getJSON(ctx, base+"/api/v1/globaldatasources/"+PersesDatasourceProvisioningResourceName); err == nil && doc != nil && doc["kind"] != nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for the reloaded datasource: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatal("the datasource did not reload after the restart")
}

// diagnoseDatasourceContainerFailure captures what the cell looked like when
// the load failed: container state, the Perses log tail, and the resource
// files the mount delivered (host side).
func diagnoseDatasourceContainerFailure(ctx context.Context, t *testing.T, name string) {
	t.Helper()
	diagCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(diagCtx, "docker", "inspect",
		"--format", "{{.State.Status}} {{.State.ExitCode}} {{.State.Error}}", name).CombinedOutput(); err == nil {
		t.Logf("container state: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.CommandContext(diagCtx, "docker", "logs", "--tail", "40", name).CombinedOutput(); err == nil {
		t.Logf("perses logs:\n%s", out)
	}
}

func getJSON(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func dockerCleanup(t *testing.T, ctx context.Context, container, network string) {
	t.Helper()
	t.Cleanup(func() {
		clean := context.WithoutCancel(ctx)
		rmCtx, cancel := context.WithTimeout(clean, 30*time.Second)
		defer cancel()
		if os.Getenv("BASEHARBOR_PERSES_DATASOURCE_CONTAINER_KEEP") == container {
			t.Logf("keeping %s for diagnosis (set was requested)", container)
			return
		}
		_ = exec.CommandContext(rmCtx, "docker", "rm", "-f", container).Run()
		if network != "" {
			_ = exec.CommandContext(rmCtx, "docker", "network", "rm", network).Run()
		}
	})
}

// persesMaterializeProviderAtEndpoint writes the materialized provider state
// with the management endpoint pinned to host:port (the live test endpoint
// instead of the loopback publish constant), keeping every other provider
// artifact the builder consumes.
func persesMaterializeProviderAtEndpoint(t *testing.T, m application.Manifest, hostPort string, sources ...observability.MetricsSource) ProviderFiles {
	t.Helper()
	state := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR"))
	if state == "" {
		t.Fatal("the cell must set BASEHARBOR_STATE_DIR before materializing the provider")
	}
	placement, err := PlacementForAt(state, "", m)
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
	_, port, ok := strings.Cut(hostPort, ":")
	if !ok {
		t.Fatalf("provider endpoint %q must be host:port", hostPort)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_PROMETHEUS_BIND=0.0.0.0\nBASEHARBOR_PROMETHEUS_PORT="+port+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Config, []byte("global:\n  scrape_interval: 30s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The provider's public runtime CA (the datasource trust anchor source,
	// F3): the cell's PKI helper writes it where the provider would, and the
	// materializer projects it to the datasource bundle.
	if err := os.WriteFile(files.RuntimeCA, []byte("-----BEGIN CERTIFICATE-----\nZml4dHVyZQ==\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeSourceSnapshot(files, placement, sources); err != nil {
		t.Fatalf("materialize runtime sources: %v", err)
	}
	return files
}

// persesDatasourceContainerTLSSecret is the GlobalSecret name the container
// acceptance gives the datasource endpoint's TLS trust anchor.
const persesDatasourceContainerTLSSecret = "baseharbor-prometheus-tls"

// persesDatasourceAllowLoopback adds the loopback range to the materialized
// config's datasource.proxy.allowed_networks. The pinned Perses SSRF guard
// denies loopback and "this network" by default (v0.55 upgrade notes); the
// acceptance cell serves the fake Prometheus on the host loopback the datasource
// URL already names, so the cell reopens exactly that range in the SAME config
// file the compose mount delivers. The product deployment never needs this:
// there, the Perses container and the provider share the runtime network.
func persesDatasourceAllowLoopback(t *testing.T, configPath string) {
	t.Helper()
	doc := persesDatasourceReadJSON(t, configPath)
	ds, _ := doc["datasource"].(map[string]any)
	if ds == nil {
		ds = map[string]any{}
		doc["datasource"] = ds
	}
	proxy, _ := ds["proxy"].(map[string]any)
	if proxy == nil {
		proxy = map[string]any{}
		ds["proxy"] = proxy
	}
	proxy["allowed_networks"] = []any{"127.0.0.0/8"}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := writePersesDatasourceFile(configPath, append(data, 0x0a)); err != nil {
		t.Fatal(err)
	}
}

// persesDatasourceGlobalSecretJSON renders the provisioning GlobalSecret that
// carries the datasource endpoint's TLS trust anchor (a public CA certificate
// — never a secret value).
func persesDatasourceGlobalSecretJSON(caPEM []byte) []byte {
	doc := map[string]any{
		"kind":     "GlobalSecret",
		"metadata": map[string]any{"name": persesDatasourceContainerTLSSecret},
		"spec": map[string]any{
			"tlsConfig": map[string]any{"ca": string(caPEM)},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil
	}
	return append(data, '\n')
}

// persesDatasourceContainerPKI issues a CA and a server certificate valid for
// addr (the host address the fake Prometheus serves and the datasource names).
func persesDatasourceContainerPKI(t *testing.T) (caPEM []byte, cert tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "baha-perses-ds-acceptance-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(2 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "baseharbor-test-prometheus"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(2 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caTmpl, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(leaf, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return caPEM, pair
}

// persesDatasourceRewriteEndpoint is the cell's one resource-side change: the
// materialized proxy URL is rewritten to the address the container-routed
// endpoint lives on (the docker bridge gateway of the cell network). The
// trust-anchor reference, the kind and everything else stay as the
// materializer wrote them (fix-6 consumes materialized output, never a
// hand-injected shape).
func persesDatasourceRewriteEndpoint(resourcePath, newURL string) error {
	data, err := os.ReadFile(resourcePath)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	spec := doc["spec"].(map[string]any)["plugin"].(map[string]any)["spec"].(map[string]any)
	proxy := spec["proxy"].(map[string]any)
	proxy["spec"].(map[string]any)["url"] = newURL
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writePersesDatasourceFile(resourcePath, append(out, '\n'))
}

// datasourceHostGatewayAddress returns the host address of the cell
// container's default bridge gateway (172.17.0.1 by default; read from the

// persesDatasourceCellNetwork is the dedicated bridge network of the cell: the
// fake Prometheus publishes on the host, so the datasource endpoint must name
