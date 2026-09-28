package devgateway

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRenderCaddyfileUsesCanonicalHostVerifiedTLSAndPathRouting(t *testing.T) {
	routes := normalizedRoutes([]Route{
		{
			Owner: "app/demo/dev",
			Key: "app:demo:api:swagger",
			Host: "demo-api.baseharbor.localhost",
			PathPrefix: "/swagger",
			Upstream: "https://baseharbor-runtime:8081",
			Network: "baseharbor-local-demo-dev_default",
			TrustFile: "/tmp/runtime-ca.pem",
			ServerName: "baseharbor-runtime",
		},
		{
			Owner: "app/demo/dev",
			Key: "app:demo:api",
			Host: "demo-api.baseharbor.localhost",
			Upstream: "https://bh-dev-demo-api:8443",
			Network: "baseharbor-local-demo-dev_default",
			TrustFile: "/tmp/app-ca.pem",
			ServerName: "localhost",
		},
	})
	got := renderCaddyfile(routes)
	for _, want := range []string{
		":443 {",
		":8443 {",
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
func (testRuntime) ConfigProject(context.Context, string, string, string) error { return nil }
func (testRuntime) UpProject(context.Context, string, string, string) error { return nil }
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
	target := "missing-target-" + strings.ReplaceAll(t.Name(), "/", "-")

	if got := URLForRuntime(target, "auth.baha.localhost", testRuntime{engine: "docker"}); got != "https://auth.baha.localhost" {
		t.Fatalf("docker canonical URL = %q", got)
	}
	if got := URLForRuntime(target, "auth.baha.localhost", testRuntime{engine: "podman"}); got != "https://auth.baha.localhost:8443" {
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
	got := renderCompose(files, routes, nil, 8443)
	for _, want := range []string{
		"cap_drop: [\"ALL\"]",
		"cap_add: [\"NET_BIND_SERVICE\"]",
		"security_opt: [\"no-new-privileges:true\"]",
		"127.0.0.1:8443:8443",
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
