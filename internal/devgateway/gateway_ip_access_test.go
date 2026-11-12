package devgateway

import (
	"strings"
	"testing"
)

func TestGatewayIPAccessAddressRequiresManagementRouteAndConcreteIPv4(t *testing.T) {
	routes := []Route{{Host: "pgadmin.dev.localhost", ManagementKey: "sql"}}
	if got, err := gatewayIPAccessAddress(routes, "192.0.2.18"); err != nil || got != "192.0.2.18" {
		t.Fatalf("explicit gateway IP = %q, %v", got, err)
	}
	if got, err := gatewayIPAccessAddress([]Route{{Host: "app.dev.localhost"}}, "192.0.2.18"); err != nil || got != "" {
		t.Fatalf("workload-only route exposed direct IP listener %q, %v", got, err)
	}
	for _, value := range []string{"127.0.0.1", "0.0.0.0", "169.254.1.2", "224.0.0.1", "::1", "gateway.example.test"} {
		t.Run(value, func(t *testing.T) {
			if _, err := gatewayIPAccessAddress(routes, value); err == nil {
				t.Fatalf("unsafe or ambiguous IP address %q accepted", value)
			}
		})
	}
}

func TestAssignDirectIPPortsRetainsAndUniquelyAllocatesDynamicPorts(t *testing.T) {
	routes := []Route{{ManagementKey: "sql"}, {ManagementKey: "cache"}, {ManagementKey: "metrics"}}
	assigned, err := assignDirectIPPorts(routes, "192.0.2.18", "", false, 18443)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{18443: true}
	for _, route := range assigned {
		if route.DirectIPPort < 1 || seen[route.DirectIPPort] {
			t.Fatalf("invalid or duplicate dynamic management port %d", route.DirectIPPort)
		}
		seen[route.DirectIPPort] = true
	}
	retained, err := assignDirectIPPorts(assigned, "192.0.2.18", "192.0.2.18", true, 18443)
	if err != nil {
		t.Fatal(err)
	}
	for i := range assigned {
		if retained[i].DirectIPPort != assigned[i].DirectIPPort {
			t.Fatalf("materialized route port changed from %d to %d", assigned[i].DirectIPPort, retained[i].DirectIPPort)
		}
	}
}

func TestDirectIPListenersUseDynamicPortsAndOnlyManagementRoutes(t *testing.T) {
	routes := []Route{
		{Owner: "shared/keycloak", Host: "auth-admin.dev.localhost", Upstream: "https://keycloak:8443", Network: "identity", ManagementKey: "identity_admin", DirectIPPort: 52341, ServerName: "localhost"},
		{Owner: "shared/keycloak", Host: "auth.dev.localhost", Upstream: "https://keycloak:8443", Network: "identity", ManagementKey: "identity", DirectIPPort: 52342, ServerName: "localhost"},
		{Host: "pgadmin.dev.localhost", Upstream: "https://pgadmin:8443", Network: "backend", ManagementKey: "sql", DirectIPPort: 52343, ServerName: "localhost"},
		{Host: "app.dev.localhost", Upstream: "http://app:8080", Network: "backend"},
	}
	files := Files{Caddyfile: "/tmp/runtime/Caddyfile"}
	caddy := renderCaddyfile(routes, 18443, "192.0.2.18")
	for _, want := range []string{
		":52341 {", "reverse_proxy https://keycloak:8443",
		"header_down Location https://auth.dev.localhost:18443 https://192.0.2.18:52342",
		":52343 {", "reverse_proxy https://pgadmin:8443", "tls_server_name localhost",
	} {
		if !strings.Contains(caddy, want) {
			t.Errorf("direct listener config missing %q:\n%s", want, caddy)
		}
	}
	if strings.Contains(strings.SplitN(caddy, ":52343 {", 2)[1], "header_down Location https://auth.dev.localhost") {
		t.Fatal("direct IP redirect rewrite crossed management route ownership")
	}
	compose := renderComposeBound(files, routes, nil, 18443, "127.0.0.1", "192.0.2.18")
	for _, want := range []string{"127.0.0.1:18443:18443", "192.0.2.18:52341:52341", "192.0.2.18:52342:52342", "192.0.2.18:52343:52343"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose missing port mapping %q:\n%s", want, compose)
		}
	}
	if strings.Contains(compose, "192.0.2.18:18443:18443") || strings.Contains(compose, "192.0.2.18:8080") {
		t.Fatalf("direct management mode exposed non-management listener or workload: %s", compose)
	}
}

func TestManagementURLsSelectsOwnedRouteBeforeSharedAndHidesOtherApps(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	files, err := FilesFor("dev-docker-1")
	if err != nil {
		t.Fatal(err)
	}
	current := state{
		Version:         stateVersion,
		HostPort:        48721,
		IPAccessAddress: "192.0.2.18",
		Routes: []Route{
			{Owner: "shared/postgresql", Key: "shared/sql", Host: "shared-sql.dev.localhost", ManagementKey: "sql", DirectIPPort: 52341},
			{Owner: "app/other/dev", Key: "other/sql", Host: "other-sql.dev.localhost", ManagementKey: "sql", DirectIPPort: 52342},
			{Owner: "app/demo/dev", Key: "demo/sql", Host: "demo-sql.dev.localhost", ManagementKey: "sql", DirectIPPort: 52343},
			{Owner: "shared/metrics", Key: "shared/metrics", Host: "metrics.dev.localhost", ManagementKey: "metrics", DirectIPPort: 52344},
			{Owner: "shared/object-storage", Key: "shared/storage", Host: "storage.dev.localhost", ManagementKey: "object_storage", DirectIPPort: 52345},
		},
	}
	if err := saveState(files.State, current); err != nil {
		t.Fatal(err)
	}
	desired := []Route{
		{Key: "demo/sql", ManagementKey: "sql"},
		{Key: "shared/metrics", ManagementKey: "metrics"},
	}
	canonical, direct, err := ManagementURLs("dev-docker-1", "app/demo/dev", desired)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := canonical["sql"], "https://demo-sql.dev.localhost:48721"; got != want {
		t.Fatalf("owned app URL = %q, want %q", got, want)
	}
	if got, want := direct["sql"], "https://192.0.2.18:52343"; got != want {
		t.Fatalf("owned app IP URL = %q, want %q", got, want)
	}
	if got, want := canonical["metrics"], "https://metrics.dev.localhost:48721"; got != want {
		t.Fatalf("shared fallback URL = %q, want %q", got, want)
	}
	if _, exists := canonical["other"]; exists {
		t.Fatal("management URL from unrelated app leaked into the selected workload")
	}
	if _, exists := canonical["unrequested"]; exists {
		t.Fatal("shared management URL not selected by this application was exposed")
	}
}
