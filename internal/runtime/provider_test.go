package runtime

import (
	"context"
	"strings"
	"testing"
)

var _ Provider = DockerProvider{}

type testProvider struct {
	kind            ProviderKind
	caps            ProviderCapabilities
	contractVersion string
}

func (p testProvider) Kind() ProviderKind { return p.kind }
func (p testProvider) Capabilities() ProviderCapabilities { return p.caps }
func (p testProvider) Descriptor() ProviderDescriptor {
	version := p.contractVersion
	if version == "" {
		version = RuntimeProviderContractVersion
	}
	return ProviderDescriptor{
		Kind:            p.kind,
		ContractVersion: version,
		ProviderVersion: "test",
		Capabilities:    p.caps,
	}
}

func TestDockerProviderMetadata(t *testing.T) {
	var provider Provider = DockerProvider{}
	if got, want := provider.Kind(), ProviderDocker; got != want {
		t.Fatalf("Kind() = %q, want %q", got, want)
	}

	caps := provider.Capabilities()
	if !caps.WorkloadLifecycle {
		t.Fatal("Docker provider must support workload lifecycle")
	}
	if !caps.ServiceExec {
		t.Fatal("Docker provider must support service exec")
	}
	if !caps.PublishedPorts {
		t.Fatal("Docker provider must support published-port inspection")
	}
	if !caps.ResourceOwnership {
		t.Fatal("Docker provider must support project resource ownership checks")
	}
}

func TestProviderKindIsDeploymentMetadata(t *testing.T) {
	if ProviderDocker == "" {
		t.Fatal("Docker provider kind must be stable and non-empty")
	}
	if got, want := string(ProviderDocker), "docker"; got != want {
		t.Fatalf("ProviderDocker = %q, want %q", got, want)
	}
}

func TestParseProviderKindDefaultsToDocker(t *testing.T) {
	for _, input := range []string{"", " "} {
		got, err := ParseProviderKind(input)
		if err != nil {
			t.Fatalf("ParseProviderKind(%q) error = %v", input, err)
		}
		if got != ProviderDocker {
			t.Fatalf("ParseProviderKind(%q) = %q, want %q", input, got, ProviderDocker)
		}
	}
}

func TestParseProviderKindAcceptsKnownProviders(t *testing.T) {
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

func TestParseProviderKindAcceptsExtensibleProviderIDs(t *testing.T) {
	for input, want := range map[string]ProviderKind{
		"future-runtime":  "future-runtime",
		"example/runtime": "example/runtime",
		"compose":         "compose",
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

func TestParseProviderKindRejectsInvalidProviderID(t *testing.T) {
	for _, input := range []string{"bad provider", "/runtime", "runtime/", "runtime//nested", "runtime@"} {
		if _, err := ParseProviderKind(input); err == nil {
			t.Fatalf("invalid runtime provider %q unexpectedly accepted", input)
		}
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

func TestProviderDescriptorRegistryDeclaresReferenceProviders(t *testing.T) {
	for _, kind := range []ProviderKind{ProviderDocker, ProviderPodman} {
		descriptor, err := ProviderDescriptorForKind(kind)
		if err != nil {
			t.Fatalf("ProviderDescriptorForKind(%q) error = %v", kind, err)
		}
		if descriptor.Kind != kind {
			t.Fatalf("descriptor kind = %q, want %q", descriptor.Kind, kind)
		}
		if descriptor.ContractVersion != RuntimeProviderContractVersion {
			t.Fatalf("descriptor contract = %q, want %q", descriptor.ContractVersion, RuntimeProviderContractVersion)
		}
		if descriptor.ProviderVersion == "" {
			t.Fatalf("descriptor %q has empty provider version", kind)
		}
		if !descriptor.Capabilities.WorkloadLifecycle || !descriptor.Capabilities.ResourceOwnership {
			t.Fatalf("descriptor %q is missing required reference capabilities: %#v", kind, descriptor.Capabilities)
		}
	}
}

func TestProviderDescriptorRegistryRejectsUnavailableProvider(t *testing.T) {
	if _, err := ProviderDescriptorForKind(ProviderKubernetes); err == nil {
		t.Fatal("unregistered Kubernetes provider unexpectedly resolved")
	}
}

func TestRequireCapabilitiesRejectsContractMismatch(t *testing.T) {
	provider := testProvider{
		kind:            "test",
		caps:            ProviderCapabilities{WorkloadLifecycle: true},
		contractVersion: "baseharbor.runtime/v999",
	}
	err := RequireCapabilities(provider, CapabilityWorkloadLifecycle)
	if err == nil || !strings.Contains(err.Error(), RuntimeProviderContractVersion) {
		t.Fatalf("expected contract-version mismatch, got %v", err)
	}
}

func TestProviderRegistryAcceptsThirdPartyDescriptorWithoutCoreEnumeration(t *testing.T) {
	descriptor := ProviderDescriptor{
		Kind:            "example/runtime",
		ContractVersion: RuntimeProviderContractVersion,
		ProviderVersion: "1.0.0",
		Standards:       []string{"OCI Runtime Specification"},
		Capabilities:    ProviderCapabilities{WorkloadLifecycle: true},
	}
	registry, err := NewProviderRegistry(ProviderRegistration{
		Descriptor: descriptor,
		Factory: func(context.Context) (Provider, error) {
			return thirdPartyTestProvider{descriptor: descriptor}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := registry.Descriptor("example/runtime")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != descriptor.Kind || got.ContractVersion != RuntimeProviderContractVersion {
		t.Fatalf("unexpected descriptor: %#v", got)
	}
	provider, err := registry.Resolve(context.Background(), "example/runtime")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Kind() != "example/runtime" {
		t.Fatalf("provider kind = %q", provider.Kind())
	}
	if err := RequireCapabilities(provider, CapabilityWorkloadLifecycle); err != nil {
		t.Fatal(err)
	}
}

type thirdPartyTestProvider struct {
	descriptor ProviderDescriptor
}

func (p thirdPartyTestProvider) Kind() ProviderKind { return p.descriptor.Kind }
func (p thirdPartyTestProvider) Descriptor() ProviderDescriptor { return p.descriptor }
func (p thirdPartyTestProvider) Capabilities() ProviderCapabilities { return p.descriptor.Capabilities }
