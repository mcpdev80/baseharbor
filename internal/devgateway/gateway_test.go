package devgateway

import (
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
