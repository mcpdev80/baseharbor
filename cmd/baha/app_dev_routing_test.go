package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"go.yaml.in/yaml/v3"
)

func TestDevelopmentIdentityRoutesResolveFrontendAliasOnAttachedNetwork(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "keycloak")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), string(capability.ScopeApplication))
	root := t.TempDir()
	app := application.WithIdentity(application.New("demo", "dev", false, false, false))
	app.Services.IdentityManagementUI = true
	files, err := identityprovider.EnsureKeycloakFilesAt(context.Background(), app, serviceissuer.New(t), root, "local")
	if err != nil {
		t.Fatal(err)
	}
	execution := applicationApplyExecution{manifest: app, resolved: resolvedApplication{TargetStateRoot: root}}
	plan := developmentRoutePlan{target: "local", appOwner: "app/demo/dev"}
	if err := execution.addDevelopmentIdentityRoutes(context.Background(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.appRoutes) != 2 {
		t.Fatalf("expected login and administration routes, got %#v", plan.appRoutes)
	}
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	var graph struct {
		Services map[string]struct {
			Networks map[string]struct {
				Aliases []string `yaml:"aliases"`
			} `yaml:"networks"`
		} `yaml:"services"`
		Networks map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	for _, route := range plan.appRoutes {
		upstream, err := url.Parse(route.Upstream)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for network, attachment := range graph.Services["keycloak-access"].Networks {
			if graph.Networks[network].Name != route.Network {
				continue
			}
			for _, alias := range attachment.Aliases {
				found = found || alias == upstream.Hostname()
			}
		}
		if !found {
			t.Errorf("%s upstream alias %s is not published on its route network %s", route.Key, upstream.Hostname(), route.Network)
		}
		if upstream.Scheme != "https" || route.TrustFile == "" || route.ServerName == "" {
			t.Errorf("identity route lost verified TLS: %#v", route)
		}
	}
}

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
