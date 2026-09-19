package exposure

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	runtime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestCaddyfileUsesLogicalServiceEndpoint(t *testing.T) {
	got := caddyfile(Route{Name: "public", Service: "web", TargetPort: 8080, Protocol: "http"})
	if !strings.Contains(got, "reverse_proxy web:8080") {
		t.Fatalf("Caddyfile does not use logical service endpoint:\n%s", got)
	}
	if strings.Contains(got, "container_name") {
		t.Fatalf("Caddyfile leaked runtime container identity:\n%s", got)
	}
}

func TestComposeOwnsStableExposureNetworkAndNoProviderVolume(t *testing.T) {
	m := application.Manifest{Version: 1, Name: "demo", Environment: "dev"}
	state := State{Version: 1, Project: ProjectName(m), Network: application.ApplicationExposureNetworkName(m), Routes: []Route{{Name: "public", Service: "web", TargetPort: 8080, Protocol: "http", PublishedPort: 18080}}}
	got := composeYAML(state, Files{Dir: "/tmp/provider"})
	if strings.Contains(got, "external: true") {
		t.Fatalf("Caddy provider must own its Compose default network:\n%s", got)
	}
	if strings.Contains(got, "volumes:\n  ") {
		t.Fatalf("provider must not create persistent named volumes:\n%s", got)
	}
	if state.Network != "baseharbor-exposure-demo-dev_default" {
		t.Fatalf("unexpected stable exposure network %q", state.Network)
	}
}

func TestHTTPSRequiresExistingTLS(t *testing.T) {
	m := application.Manifest{
		Version: 1, Name: "demo", Environment: "dev",
		Workload: application.WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}},
		Exposures: []application.HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "https"}},
	}
	driver := NewDriver(structCompose{}, m, application.RuntimeFiles{}, Deployment{Hostname: "demo.example", TLSMode: "acme"})
	resource := capabilityResource(m, "public")
	if err := driver.Preflight(context.Background(), resource, bindingFor(resource, "web")); err == nil || !strings.Contains(err.Error(), "existing/BYOC") {
		t.Fatalf("expected managed HTTPS TLS boundary, got %v", err)
	}
}

// structCompose is the zero value runtime Compose; Preflight does not mutate or invoke it.
type structCompose = runtime.Compose

func capabilityResource(m application.Manifest, name string) capability.Resource {
	return capability.Resource{Application: m.Name, Kind: capability.ExposureHTTP, Name: name, Provider: capability.ProviderCaddy}
}

func bindingFor(resource capability.Resource, service string) capability.Binding {
	return capability.Binding{Resource: resource, Workload: "service/" + service}
}

func TestComposePublicAndInternalBindings(t *testing.T) {
	files := Files{Dir: "/tmp/provider"}
	publicState := State{Routes: []Route{{Name: "public", Service: "web", TargetPort: 8080, Protocol: "http", Visibility: "public", PublishedPort: 18080}}}
	publicCompose := composeYAML(publicState, files)
	if !strings.Contains(publicCompose, "\"18080:80\"") || strings.Contains(publicCompose, "127.0.0.1:18080:80") {
		t.Fatalf("public exposure must bind host interfaces:\n%s", publicCompose)
	}

	internalState := State{Routes: []Route{{Name: "internal", Service: "admin", TargetPort: 9090, Protocol: "http", Visibility: "internal", PublishedPort: 19090}}}
	internalCompose := composeYAML(internalState, files)
	if !strings.Contains(internalCompose, "\"127.0.0.1:19090:80\"") {
		t.Fatalf("internal exposure must bind loopback only:\n%s", internalCompose)
	}
}
