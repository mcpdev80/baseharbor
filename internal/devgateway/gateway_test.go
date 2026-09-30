package devgateway

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderCaddyfileUsesCanonicalHostVerifiedTLSAndPathRouting(t *testing.T) {
	routes := normalizedRoutes([]Route{
		{
			Owner:      "app/demo/dev",
			Key:        "app:demo:api:swagger",
			Host:       "demo-api.baseharbor.localhost",
			PathPrefix: "/swagger",
			Upstream:   "https://baseharbor-runtime:8081",
			Network:    "baseharbor-local-demo-dev_default",
			TrustFile:  "/tmp/runtime-ca.pem",
			ServerName: "baseharbor-runtime",
		},
		{
			Owner:      "app/demo/dev",
			Key:        "app:demo:api",
			Host:       "demo-api.baseharbor.localhost",
			Upstream:   "https://bh-dev-demo-api:8443",
			Network:    "baseharbor-local-demo-dev_default",
			TrustFile:  "/tmp/app-ca.pem",
			ServerName: "localhost",
		},
	})
	got := renderCaddyfile(routes, 18443)
	for _, want := range []string{
		":18443 {",
		"host demo-api.baseharbor.localhost",
		"path /swagger /swagger/*",
		"uri strip_prefix /swagger",
		"reverse_proxy https://baseharbor-runtime:8081",
		"tls_trust_pool file /trust/route-000.pem",
		"tls_server_name baseharbor-runtime",
		"reverse_proxy https://bh-dev-demo-api:8443",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Caddyfile missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n:443 {\n") || strings.Contains(got, "\n:8443 {\n") {
		t.Fatalf("Caddyfile contains hard-coded gateway listeners:\n%s", got)
	}
	if strings.Contains(got, "tls_insecure_skip_verify") {
		t.Fatalf("Caddyfile disabled upstream TLS verification:\n%s", got)
	}
}

func TestNormalizedRoutesOrdersSpecificPathBeforeHostFallback(t *testing.T) {
	routes := normalizedRoutes([]Route{
		{Key: "fallback", Host: "demo-api.baseharbor.localhost", Upstream: "http://app:8080", Network: "app"},
		{Key: "swagger", Host: "demo-api.baseharbor.localhost", PathPrefix: "/swagger", Upstream: "http://docs:8081", Network: "app"},
	})
	if len(routes) != 2 {
		t.Fatalf("normalized routes = %d, want 2", len(routes))
	}
	if routes[0].Key != "swagger" || routes[0].PathPrefix != "/swagger" {
		t.Fatalf("specific path route was not ordered first: %#v", routes)
	}
}

type testRuntime struct {
	engine string
}

func (r testRuntime) Engine() string { return r.engine }
func (r testRuntime) PreferredLocalHTTPSPort() int {
	if r.engine == "podman" {
		return 8443
	}
	return 443
}
func (testRuntime) ConfigProject(context.Context, string, string, string) error  { return nil }
func (testRuntime) UpProject(context.Context, string, string, string) error      { return nil }
func (testRuntime) DestroyProject(context.Context, string, string, string) error { return nil }

func TestGatewayHostPortUsesUnprivilegedPortForPodman(t *testing.T) {
	if got := gatewayHostPort(testRuntime{engine: "docker"}); got != 443 {
		t.Fatalf("docker gateway port = %d, want 443", got)
	}
	if got := gatewayHostPort(testRuntime{engine: "podman"}); got != 8443 {
		t.Fatalf("podman gateway port = %d, want 8443", got)
	}
	if got := canonicalURL("demo.baha.localhost", 443); got != "https://demo.baha.localhost" {
		t.Fatalf("docker canonical URL = %q", got)
	}
	if got := canonicalURL("demo.baha.localhost", 8443); got != "https://demo.baha.localhost:8443" {
		t.Fatalf("podman canonical URL = %q", got)
	}
}

func TestPruneUnavailableTrustRoutesDropsOnlyStaleHTTPSRoutes(t *testing.T) {
	dir := t.TempDir()
	trust := dir + "/ca.pem"
	if err := os.WriteFile(trust, []byte("ca"), 0o600); err != nil {
		t.Fatal(err)
	}
	routes := []Route{
		{Key: "http", Upstream: "http://app:8080"},
		{Key: "https-live", Upstream: "https://live:8443", TrustFile: trust},
		{Key: "https-stale", Upstream: "https://stale:8443", TrustFile: dir + "/missing.pem"},
	}
	got, changed, err := pruneUnavailableTrustRoutes(routes)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected stale HTTPS route to be pruned")
	}
	if len(got) != 2 || got[0].Key != "http" || got[1].Key != "https-live" {
		t.Fatalf("unexpected remaining routes: %#v", got)
	}
}

func TestURLForRuntimeUsesRuntimePortBeforeGatewayStateExists(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := "missing-target-" + strings.ReplaceAll(t.Name(), "/", "-")
	available := func(int) bool { return true }

	if got := urlForRuntime(target, "auth.baha.localhost", testRuntime{engine: "docker"}, available); got != "https://auth.baha.localhost" {
		t.Fatalf("docker canonical URL = %q", got)
	}
	if got := urlForRuntime(target, "auth.baha.localhost", testRuntime{engine: "podman"}, available); got != "https://auth.baha.localhost:8443" {
		t.Fatalf("podman canonical URL = %q", got)
	}
}

func TestRenderComposeUsesOnlyBindServiceCapabilityForCanonicalHTTPS(t *testing.T) {
	files := Files{
		Caddyfile: "/tmp/Caddyfile",
		Cert:      "/tmp/server.pem",
		Key:       "/tmp/server-key.pem",
	}
	routes := []Route{{
		Owner:    "shared/keycloak",
		Key:      "shared/keycloak/login",
		Host:     "auth.baha.localhost",
		Upstream: "https://identity:9443",
		Network:  "identity-consumer",
	}}
	got := renderCompose(files, routes, nil, 18443)
	for _, want := range []string{
		"cap_drop: [\"ALL\"]",
		"cap_add: [\"NET_BIND_SERVICE\"]",
		"security_opt: [\"no-new-privileges:true\"]",
		"127.0.0.1:18443:18443",
		"auth.baha.localhost",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("gateway Compose missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "privileged: true") {
		t.Fatalf("gateway Compose became privileged:\n%s", got)
	}
}

func TestSelectGatewayHostPortFallsBackAndPersists(t *testing.T) {
	availability := map[int]bool{443: false, 18443: false, 18444: true}
	available := func(port int) bool { return availability[port] }

	selected, err := selectGatewayHostPort(443, 0, false, available)
	if err != nil {
		t.Fatal(err)
	}
	if selected != 18444 {
		t.Fatalf("selected fallback = %d, want 18444", selected)
	}

	availability[18444] = false
	selected, err = selectGatewayHostPort(443, 18444, true, available)
	if err != nil {
		t.Fatal(err)
	}
	if selected != 18444 {
		t.Fatalf("materialized persisted port = %d, want 18444", selected)
	}
}

func TestSelectGatewayHostPortMovesFromStalePersistedPort(t *testing.T) {
	availability := map[int]bool{443: false, 18443: true, 18444: false}
	available := func(port int) bool { return availability[port] }

	selected, err := selectGatewayHostPort(443, 18444, false, available)
	if err != nil {
		t.Fatal(err)
	}
	if selected != 18443 {
		t.Fatalf("replacement fallback = %d, want 18443", selected)
	}
}

func TestURLForRuntimeIgnoresUninitializedGatewayStateAndSelectsFallback(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := "gateway-zero-state"
	files, err := FilesFor(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(files.State), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := saveState(files.State, state{Version: stateVersion, HostPort: 0}); err != nil {
		t.Fatal(err)
	}

	available := func(port int) bool {
		return port == 18443
	}
	got := urlForRuntime(target, "auth.baha.localhost", testRuntime{engine: "docker"}, available)
	if got != "https://auth.baha.localhost:18443" {
		t.Fatalf("canonical URL = %q, want persisted-selection fallback port", got)
	}
}

func TestURLForRuntimeUsesPersistedEffectiveGatewayPort(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := "gateway-persisted-state"
	files, err := FilesFor(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(files.State), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := saveState(files.State, state{Version: stateVersion, HostPort: 18443}); err != nil {
		t.Fatal(err)
	}

	got := urlForRuntime(target, "auth.baha.localhost", testRuntime{engine: "docker"}, func(int) bool { return false })
	if got != "https://auth.baha.localhost:18443" {
		t.Fatalf("canonical URL = %q, want persisted effective port", got)
	}
}

type recordingGatewayRuntime struct {
	destroyCalls int
}

func (*recordingGatewayRuntime) ConfigProject(context.Context, string, string, string) error {
	return nil
}
func (*recordingGatewayRuntime) UpProject(context.Context, string, string, string) error { return nil }
func (r *recordingGatewayRuntime) DestroyProject(context.Context, string, string, string) error {
	r.destroyCalls++
	return nil
}

func TestSaveRouteStateRecreatesMaterializedGatewayWhenNetworkSetChanges(t *testing.T) {
	dir := t.TempDir()
	files := Files{
		Dir:     dir,
		State:   filepath.Join(dir, "routes.json"),
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "runtime.env"),
		Project: "bh-dev-gateway",
	}
	if err := os.WriteFile(files.Compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingGatewayRuntime{}
	previous := state{Version: stateVersion, Routes: []Route{
		{Key: "app", Network: "app-network"},
		{Key: "shared", Network: "shared-network"},
	}}
	next := state{Version: stateVersion, Routes: []Route{
		{Key: "shared", Network: "shared-network"},
	}}
	if err := saveRouteStateForReconcile(context.Background(), runtime, files, previous, next); err != nil {
		t.Fatal(err)
	}
	if runtime.destroyCalls != 1 {
		t.Fatalf("gateway destroy calls = %d, want 1 after route network removal", runtime.destroyCalls)
	}
	saved, err := loadState(files.State)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Routes) != 1 || saved.Routes[0].Network != "shared-network" {
		t.Fatalf("saved routes = %#v", saved.Routes)
	}
}

func TestSaveRouteStateKeepsMaterializedGatewayWhenNetworkSetIsStable(t *testing.T) {
	dir := t.TempDir()
	files := Files{
		Dir:     dir,
		State:   filepath.Join(dir, "routes.json"),
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "runtime.env"),
		Project: "bh-dev-gateway",
	}
	if err := os.WriteFile(files.Compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingGatewayRuntime{}
	previous := state{Version: stateVersion, Routes: []Route{
		{Key: "old", Network: "app-network"},
	}}
	next := state{Version: stateVersion, Routes: []Route{
		{Key: "new", Network: "app-network"},
	}}
	if err := saveRouteStateForReconcile(context.Background(), runtime, files, previous, next); err != nil {
		t.Fatal(err)
	}
	if runtime.destroyCalls != 0 {
		t.Fatalf("gateway destroy calls = %d, want 0 for stable network set", runtime.destroyCalls)
	}
}

func TestRelatedRedirectURLsAllowsPairedIdentityLoginOnly(t *testing.T) {
	routes := []Route{
		{Owner: "shared/keycloak", Key: "shared/keycloak/login", Host: "auth.baha.localhost"},
		{Owner: "shared/keycloak", Key: "shared/keycloak/admin", Host: "auth-admin.baha.localhost"},
		{Owner: "shared/postgresql", Key: "shared/postgresql", Host: "pgadmin.baha.localhost"},
	}
	got := relatedRedirectURLs(routes, routes[1], 18443)
	if len(got) != 1 || got[0] != "https://auth.baha.localhost:18443" {
		t.Fatalf("admin redirect authorities = %#v", got)
	}
	if got := relatedRedirectURLs(routes, routes[2], 18443); len(got) != 0 {
		t.Fatalf("non-identity route unexpectedly allows redirects: %#v", got)
	}
}
