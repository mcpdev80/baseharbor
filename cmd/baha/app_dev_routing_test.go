package main

import (
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestDevelopmentWorkloadRouteUsesExplicitHTTPS(t *testing.T) {
	files := application.RuntimeFiles{Bindings: filepath.Join("/state", "bindings")}
	route := developmentWorkloadRoute(
		"app/demo/dev",
		"demo.baha.localhost",
		"bh-dev-demo-api",
		"bh-demo-dev_default",
		files,
		"demo-app",
		8080,
		"https",
	)
	if route.Upstream != "https://bh-dev-demo-api:8080" {
		t.Fatalf("Upstream = %q", route.Upstream)
	}
	if route.TrustFile != filepath.Join(files.Bindings, "runtime-identity", "ca.pem") {
		t.Fatalf("TrustFile = %q", route.TrustFile)
	}
	if route.ServerName != "demo-app" {
		t.Fatalf("ServerName = %q", route.ServerName)
	}
}

func TestDevelopmentWorkloadRouteDefaultsToHTTP(t *testing.T) {
	route := developmentWorkloadRoute(
		"app/example/dev",
		"example.baha.localhost",
		"bh-dev-example-api",
		"bh-example-dev_default",
		application.RuntimeFiles{},
		"api",
		8080,
		"",
	)
	if route.Upstream != "http://bh-dev-example-api:8080" {
		t.Fatalf("Upstream = %q", route.Upstream)
	}
	if route.TrustFile != "" || route.ServerName != "" {
		t.Fatalf("HTTP route unexpectedly configured TLS trust: %#v", route)
	}
}

func TestDevelopmentGatewayAllowsNonRoutableWorkload(t *testing.T) {
	m := application.New("worker", "dev", false, false, false)
	m.Services.SQL = false
	m = application.WithWorkloadComponents(m, "worker")
	if !requiresDevelopmentGateway(m) {
		t.Fatal("development workload should still participate in route reconciliation")
	}
	if requiresDeclaredDevelopmentGatewaySurface(m) {
		t.Fatal("non-routable workload must not require a canonical development URL")
	}
}

func TestDevelopmentGatewayRequiresExplicitExposure(t *testing.T) {
	m := application.New("web", "dev", false, false, false)
	m.Services.SQL = false
	m = application.WithWorkloadComponents(m, "web")
	m.Exposures = []application.HTTPExposureRequirement{{
		Name: "public", Service: "web", Port: 8080, Protocol: "http",
	}}
	if !requiresDeclaredDevelopmentGatewaySurface(m) {
		t.Fatal("explicit exposure must require a canonical development URL")
	}
}

func TestDevelopmentExposureUpstreamUsesProviderAlias(t *testing.T) {
	got := developmentExposureUpstream("bh-demo-dev-exposure", "demo-app", "http", 8080)
	want := "http://bh-dev-bh-demo-dev-exposure-demo-app:8080"
	if got != want {
		t.Fatalf("upstream = %q, want %q", got, want)
	}
}
