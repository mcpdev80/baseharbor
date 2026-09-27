package devgateway

import (
	"strings"
	"testing"
)

func TestRenderCaddyUsesCanonicalHostAndVerifiedUpstreamTLS(t *testing.T) {
	routes := []Route{{
		Owner: "app/demo/dev",
		Key: "app:demo:pgadmin",
		Host: "demo-pgadmin.baseharbor.localhost",
		Upstream: "https://bh-dev-demo-pgadmin:8443",
		Network: "baseharbor-local-demo-dev_default",
		TrustFile: "/tmp/ca.pem",
		ServerName: "localhost",
	}}
	got := renderCaddy(routes)
	for _, want := range []string{
		"https://demo-pgadmin.baseharbor.localhost:8443",
		"reverse_proxy https://bh-dev-demo-pgadmin:8443",
		"tls_trust_pool file /trust/route-00.pem",
		"tls_server_name localhost",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Caddyfile missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "tls_insecure_skip_verify") {
		t.Fatalf("Caddyfile disabled upstream TLS verification:\n%s", got)
	}
}

func TestNormalizedRoutesRejectHostCollisions(t *testing.T) {
	_, err := normalizedRoutes([]Route{
		{Owner:"app/a/dev", Key:"a", Host:"same.baseharbor.localhost", Network:"a", Upstream:"https://a", TrustFile:"/a"},
		{Owner:"app/b/dev", Key:"b", Host:"same.baseharbor.localhost", Network:"b", Upstream:"https://b", TrustFile:"/b"},
	})
	if err == nil {
		t.Fatal("expected canonical host collision to fail")
	}
}
