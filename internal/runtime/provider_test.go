package runtime

import (
	"context"
	"strings"
	"testing"
)

var _ Provider = Compose{}

type testProvider struct {
	kind ProviderKind
	caps ProviderCapabilities
}

func (p testProvider) Kind() ProviderKind                 { return p.kind }
func (p testProvider) Capabilities() ProviderCapabilities { return p.caps }

func TestComposeProviderMetadata(t *testing.T) {
	var provider Provider = Compose{}
	if got, want := provider.Kind(), ProviderCompose; got != want {
		t.Fatalf("Kind() = %q, want %q", got, want)
	}

	caps := provider.Capabilities()
	if !caps.WorkloadLifecycle {
		t.Fatal("Compose provider must support workload lifecycle")
	}
	if !caps.ServiceExec {
		t.Fatal("Compose provider must support service exec")
	}
	if !caps.PublishedPorts {
		t.Fatal("Compose provider must support published-port inspection")
	}
	if !caps.ResourceOwnership {
		t.Fatal("Compose provider must support project resource ownership checks")
	}
}

func TestProviderKindIsDeploymentMetadata(t *testing.T) {
	if ProviderCompose == "" {
		t.Fatal("Compose provider kind must be stable and non-empty")
	}
	if got, want := string(ProviderCompose), "compose"; got != want {
		t.Fatalf("ProviderCompose = %q, want %q", got, want)
	}
}

func TestParseProviderKindDefaultsToCompose(t *testing.T) {
	for _, input := range []string{"", " ", "compose", " COMPOSE "} {
		got, err := ParseProviderKind(input)
		if err != nil {
			t.Fatalf("ParseProviderKind(%q) error = %v", input, err)
		}
		if got != ProviderCompose {
			t.Fatalf("ParseProviderKind(%q) = %q, want %q", input, got, ProviderCompose)
		}
	}
}

func TestParseProviderKindAcceptsKnownFutureProviders(t *testing.T) {
	for input, want := range map[string]ProviderKind{
		"docker":     ProviderDocker,
		"podman":     ProviderPodman,
		"kubernetes": ProviderKubernetes,
		"openshift":  ProviderOpenShift,
	} {
		got, err := ParseProviderKind(input)
		if err != nil {
			t.Fatalf("ParseProviderKind(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseProviderKind(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseProviderKindRejectsUnknownProvider(t *testing.T) {
	if _, err := ParseProviderKind("future-runtime"); err == nil {
		t.Fatal("unknown runtime provider unexpectedly accepted")
	}
}

func TestDetectProviderForKindRejectsUnavailableProvider(t *testing.T) {
	if _, err := DetectProviderForKind(context.Background(), ProviderKind("kubernetes")); err == nil {
		t.Fatal("unavailable runtime provider unexpectedly resolved")
	}
}

func TestProviderCapabilitiesSupportsKnownCapabilities(t *testing.T) {
	caps := ProviderCapabilities{WorkloadLifecycle: true, ServiceExec: true}
	if !caps.Supports(CapabilityWorkloadLifecycle) {
		t.Fatal("workload lifecycle should be supported")
	}
	if !caps.Supports(CapabilityServiceExec) {
		t.Fatal("service exec should be supported")
	}
	if caps.Supports(CapabilityPublishedPorts) {
		t.Fatal("published ports should not be supported")
	}
	if caps.Supports(RuntimeCapability("future-capability")) {
		t.Fatal("unknown capability must fail closed")
	}
}

func TestRequireCapabilitiesAcceptsSatisfiedRequirements(t *testing.T) {
	provider := testProvider{
		kind: "test",
		caps: ProviderCapabilities{WorkloadLifecycle: true, ServiceExec: true},
	}
	if err := RequireCapabilities(provider, CapabilityWorkloadLifecycle, CapabilityServiceExec); err != nil {
		t.Fatalf("RequireCapabilities() error = %v", err)
	}
}

func TestRequireCapabilitiesRejectsMissingRequirement(t *testing.T) {
	provider := testProvider{
		kind: "test",
		caps: ProviderCapabilities{WorkloadLifecycle: true},
	}
	err := RequireCapabilities(provider, CapabilityWorkloadLifecycle, CapabilityServiceExec)
	if err == nil {
		t.Fatal("expected missing capability to fail")
	}
	if !strings.Contains(err.Error(), "service-exec") || !strings.Contains(err.Error(), "test") {
		t.Fatalf("error does not identify provider and capability: %v", err)
	}
}

func TestRequireCapabilitiesRejectsNilProviderAndEmptyRequirement(t *testing.T) {
	if err := RequireCapabilities(nil, CapabilityWorkloadLifecycle); err == nil {
		t.Fatal("nil provider unexpectedly accepted")
	}
	provider := testProvider{kind: "test", caps: ProviderCapabilities{}}
	if err := RequireCapabilities(provider, ""); err == nil {
		t.Fatal("empty capability unexpectedly accepted")
	}
}
