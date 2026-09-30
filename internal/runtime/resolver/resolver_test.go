package resolver

import (
	"context"
	"testing"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

func TestReferenceProviderDescriptors(t *testing.T) {
	for _, kind := range []runtimecontract.ProviderKind{
		runtimecontract.ProviderDocker,
		runtimecontract.ProviderPodman,
		runtimecontract.ProviderKubernetes,
	} {
		descriptor, err := Descriptor(kind)
		if err != nil {
			t.Fatalf("Descriptor(%q): %v", kind, err)
		}
		if descriptor.Kind != kind {
			t.Fatalf("descriptor kind = %q, want %q", descriptor.Kind, kind)
		}
		if descriptor.ContractVersion != runtimecontract.RuntimeProviderContractVersion {
			t.Fatalf("contract = %q, want %q", descriptor.ContractVersion, runtimecontract.RuntimeProviderContractVersion)
		}
	}
}

func TestKubernetesUsesWorkloadProviderBoundary(t *testing.T) {
	if _, err := WorkloadProvider(context.Background(), runtimecontract.ProviderKubernetes); err != nil {
		t.Fatalf("Kubernetes workload provider did not resolve: %v", err)
	}
	if _, err := RuntimeProvider(context.Background(), runtimecontract.ProviderKubernetes); err == nil {
		t.Fatal("Kubernetes unexpectedly implemented the legacy Docker/Podman runtime interface")
	}
}
