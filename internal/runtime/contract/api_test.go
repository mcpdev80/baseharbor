package contract

import (
	"context"
	"strings"
	"testing"
)

type testProvider struct {
	descriptor ProviderDescriptor
}

func (p testProvider) Kind() ProviderKind                 { return p.descriptor.Kind }
func (p testProvider) Descriptor() ProviderDescriptor     { return p.descriptor }
func (p testProvider) Capabilities() ProviderCapabilities { return p.descriptor.Capabilities }

func TestParseProviderKind(t *testing.T) {
	for input, want := range map[string]ProviderKind{
		"":                ProviderDocker,
		" ":               ProviderDocker,
		"docker":          ProviderDocker,
		"podman":          ProviderPodman,
		"kubernetes":      ProviderKubernetes,
		"openshift":       ProviderOpenShift,
		"future-runtime":  "future-runtime",
		"example/runtime": "example/runtime",
	} {
		got, err := ParseProviderKind(input)
		if err != nil {
			t.Fatalf("ParseProviderKind(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseProviderKind(%q) = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"bad provider", "/runtime", "runtime/", "runtime//nested", "runtime@"} {
		if _, err := ParseProviderKind(input); err == nil {
			t.Fatalf("invalid provider id %q accepted", input)
		}
	}
}

func TestRequireCapabilitiesFailClosed(t *testing.T) {
	descriptor := ProviderDescriptor{
		Kind:            "example/runtime",
		ContractVersion: RuntimeProviderContractVersion,
		ProviderVersion: "1.0.0",
		WorkloadSources: []string{"compose-spec"},
		Realization:     "example",
		Capabilities:    ProviderCapabilities{WorkloadLifecycle: true},
	}
	provider := testProvider{descriptor: descriptor}
	if err := RequireCapabilities(provider, CapabilityWorkloadLifecycle); err != nil {
		t.Fatal(err)
	}
	if err := RequireCapabilities(provider, CapabilityServiceExec); err == nil {
		t.Fatal("missing capability unexpectedly accepted")
	}
	bad := provider
	bad.descriptor.ContractVersion = "baseharbor.runtime/v999"
	if err := RequireCapabilities(bad, CapabilityWorkloadLifecycle); err == nil || !strings.Contains(err.Error(), RuntimeProviderContractVersion) {
		t.Fatalf("expected contract mismatch, got %v", err)
	}
}

func TestProviderRegistryAcceptsThirdPartyDescriptorWithoutCoreEnumeration(t *testing.T) {
	descriptor := ProviderDescriptor{
		Kind:            "example/runtime",
		ContractVersion: RuntimeProviderContractVersion,
		ProviderVersion: "1.0.0",
		Standards:       []string{"OCI Runtime Specification"},
		WorkloadSources: []string{"compose-spec"},
		Realization:     "example-runtime",
		Capabilities:    ProviderCapabilities{WorkloadLifecycle: true},
	}
	registry, err := NewProviderRegistry(ProviderRegistration{
		Descriptor: descriptor,
		Factory: func(context.Context) (Provider, error) {
			return testProvider{descriptor: descriptor}, nil
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
	if provider.Kind() != descriptor.Kind {
		t.Fatalf("provider kind = %q, want %q", provider.Kind(), descriptor.Kind)
	}
}

func TestServiceStateTerminalFailure(t *testing.T) {
	cases := []struct {
		name  string
		state ServiceState
		want  bool
	}{
		{name: "running", state: ServiceState{State: "running"}, want: false},
		{name: "created without error", state: ServiceState{State: "created"}, want: false},
		{name: "created with runtime error", state: ServiceState{State: "created", Error: "failed to start container"}, want: true},
		{name: "created with nonzero exit", state: ServiceState{State: "created", ExitCode: 128}, want: true},
		{name: "exited", state: ServiceState{State: "exited"}, want: true},
		{name: "dead", state: ServiceState{State: "dead"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.state.TerminalFailure(); got != tc.want {
				t.Fatalf("TerminalFailure() = %v, want %v", got, tc.want)
			}
		})
	}
}
