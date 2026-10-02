package exposure

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	dockerprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/docker"
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

func TestCaddyfileSeparatesPublicAndWorkloadTransport(t *testing.T) {
	got := caddyfile(Route{
		Name: "public", Service: "web", TargetPort: 8080,
		Protocol: "http", WorkloadProtocol: "https",
	})
	for _, want := range []string{
		"reverse_proxy https://web:8080",
		"tls_trust_pool file /trust/workload-ca.pem",
		"tls_server_name web",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("secure workload Caddyfile missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "tls_insecure_skip_verify") {
		t.Fatalf("secure workload Caddyfile disabled TLS verification:\n%s", got)
	}
}

func TestComposeMountsWorkloadTrustOnlyForSecureUpstream(t *testing.T) {
	files := Files{Dir: "/tmp/provider"}
	secure := State{Routes: []Route{{
		Name: "public", Service: "web", TargetPort: 8080,
		Protocol: "http", WorkloadProtocol: "https", Visibility: "public", PublishedPort: 18080,
	}}}
	got := composeYAML(secure, files)
	if !strings.Contains(got, "/tmp/provider/routes/public/workload-ca.pem:/trust/workload-ca.pem:ro") {
		t.Fatalf("secure workload compose does not mount runtime trust:\n%s", got)
	}

	plain := State{Routes: []Route{{
		Name: "public", Service: "web", TargetPort: 8080,
		Protocol: "http", WorkloadProtocol: "http", Visibility: "public", PublishedPort: 18080,
	}}}
	got = composeYAML(plain, files)
	if strings.Contains(got, "workload-ca.pem") {
		t.Fatalf("plain workload compose unexpectedly mounts runtime trust:\n%s", got)
	}
}

func TestComposeConsumesStableWorkloadOwnedExposureNetworkAndNoProviderVolume(t *testing.T) {
	m := application.Manifest{Version: 1, Name: "demo", Environment: "dev"}
	state := State{Version: 1, Project: ProjectName(m), Network: application.ApplicationExposureNetworkName(m), Routes: []Route{{Name: "public", Service: "web", TargetPort: 8080, Protocol: "http", PublishedPort: 18080}}}
	got := composeYAML(state, Files{Dir: "/tmp/provider"})
	if !strings.Contains(got, "external: true") || !strings.Contains(got, "name: "+state.Network) {
		t.Fatalf("Caddy provider must consume the stable exposure network externally:\n%s", got)
	}
	if strings.Contains(got, "\nvolumes:\n") {
		t.Fatalf("provider must not create persistent named volumes:\n%s", got)
	}
	if state.Network != "baseharbor-exposure-demo-dev_default" {
		t.Fatalf("unexpected stable exposure network %q", state.Network)
	}
}

func TestHTTPSAllowsLocalGatewayTerminationAndRejectsUnsupportedTLS(t *testing.T) {
	m := application.Manifest{
		Version: 1, Name: "demo", Environment: "dev",
		Workload:  application.WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}},
		Exposures: []application.HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "https"}},
	}
	resource := capabilityResource(m, "public")

	local := NewDriver(structCompose{}, m, application.RuntimeFiles{}, Deployment{Hostname: "demo.baha.localhost", TLSMode: "local"})
	if err := local.Preflight(context.Background(), resource, bindingFor(resource, "web")); err != nil {
		t.Fatalf("local development HTTPS must terminate at the development gateway: %v", err)
	}

	acme := NewDriver(structCompose{}, m, application.RuntimeFiles{}, Deployment{Hostname: "demo.example", TLSMode: "acme"})
	if err := acme.Preflight(context.Background(), resource, bindingFor(resource, "web")); err == nil || !strings.Contains(err.Error(), "local development TLS termination or existing/BYOC") {
		t.Fatalf("expected managed HTTPS TLS boundary, got %v", err)
	}
}

func TestLifecycleAllowsLocalGatewayTermination(t *testing.T) {
	m := application.Manifest{
		Version: 1, Name: "demo", Environment: "dev",
		Workload:  application.WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}},
		Exposures: []application.HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "https"}},
	}
	resource := capabilityResource(m, "public")
	driver := NewDriver(structCompose{}, m, application.RuntimeFiles{}, Deployment{Hostname: "demo.baha.localhost", TLSMode: "local"})
	lifecycle := NewLifecycle("demo.baha.localhost", "local", NewCaddyRealization(driver))
	if err := lifecycle.Preflight(context.Background(), resource, bindingFor(resource, "web")); err != nil {
		t.Fatalf("runtime-neutral exposure lifecycle must allow local development TLS termination: %v", err)
	}
}

func TestLocalHTTPSUsesPlainInternalProviderTransport(t *testing.T) {
	route := Route{
		Name: "public", Service: "web", TargetPort: 8080,
		Protocol: "https", ProviderProtocol: "http", Visibility: "public", PublishedPort: 18080,
	}
	got := caddyfile(route)
	if strings.Contains(got, "tls /certs/") {
		t.Fatalf("local development exposure must not duplicate TLS inside Caddy:\n%s", got)
	}
	if !strings.Contains(got, ":8080") {
		t.Fatalf("local development exposure must use the internal HTTP listener:\n%s", got)
	}
	compose := composeYAML(State{Routes: []Route{route}}, Files{Dir: "/tmp/provider"})
	if !strings.Contains(compose, "\"18080:8080\"") {
		t.Fatalf("local development exposure must publish the internal HTTP provider port:\n%s", compose)
	}
	if strings.Contains(compose, "cert.pem:/certs/cert.pem") {
		t.Fatalf("local development exposure must not mount a leaf certificate into Caddy:\n%s", compose)
	}
}

// structCompose is an explicit zero-value Docker provider; Preflight does not mutate or invoke it.
type structCompose = dockerprovider.Provider

func capabilityResource(m application.Manifest, name string) capability.Resource {
	return capability.Resource{Application: m.Name, Kind: capability.ExposureHTTP, Name: name, Provider: capability.ProviderCaddy}
}

func bindingFor(resource capability.Resource, service string) capability.Binding {
	return capability.Binding{
		Resource: resource,
		Workload: "service/" + service,
		HTTPExposure: &capability.HTTPExposureBinding{
			Service: service, TargetPort: 8080, Protocol: "https", Visibility: "public",
		},
	}
}

func TestComposePublicAndInternalBindings(t *testing.T) {
	files := Files{Dir: "/tmp/provider"}
	publicState := State{Routes: []Route{{Name: "public", Service: "web", TargetPort: 8080, Protocol: "http", Visibility: "public", PublishedPort: 18080}}}
	publicCompose := composeYAML(publicState, files)
	if !strings.Contains(publicCompose, "\"18080:8080\"") || strings.Contains(publicCompose, "127.0.0.1:18080:8080") {
		t.Fatalf("public exposure must bind host interfaces:\n%s", publicCompose)
	}

	internalState := State{Routes: []Route{{Name: "internal", Service: "admin", TargetPort: 9090, Protocol: "http", Visibility: "internal", PublishedPort: 19090}}}
	internalCompose := composeYAML(internalState, files)
	if !strings.Contains(internalCompose, "\"127.0.0.1:19090:8080\"") {
		t.Fatalf("internal exposure must bind loopback only:\n%s", internalCompose)
	}
}

func TestCaddyReferenceImageIsPinned(t *testing.T) {
	if caddyImage != "docker.io/library/caddy:2.11.4-alpine" {
		t.Fatalf("unexpected Caddy reference image %q", caddyImage)
	}
}

func TestComposeRunsCaddyUnprivileged(t *testing.T) {
	state := State{Routes: []Route{{Name: "public", Service: "web", TargetPort: 8080, Protocol: "http", Visibility: "internal", PublishedPort: 18080}}}
	got := composeYAML(state, Files{Dir: "/tmp/provider"})
	for _, want := range []string{
		"user: \"65532:65532\"",
		"read_only: true",
		"cap_drop: [\"ALL\"]",
		"cap_add: [\"NET_BIND_SERVICE\"]",
		"no-new-privileges:true",
		"entrypoint: [\"/bin/sh\", \"-ec\"]",
		"/tmp:rw,noexec,nosuid,nodev",
		"/config:rw,noexec,nosuid,nodev,mode=1777",
		"/data:rw,noexec,nosuid,nodev,mode=1777",
		"exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile",
		"127.0.0.1:18080:8080",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Caddy compose missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"/run/baseharbor", "cat /usr/bin/caddy"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Caddy compose still uses runtime-specific binary staging %q:\n%s", forbidden, got)
		}
	}
	if strings.Contains(got, ":80\"") || strings.Contains(got, ":443\"") {
		t.Fatalf("Caddy must not require privileged container ports:\n%s", got)
	}
}
