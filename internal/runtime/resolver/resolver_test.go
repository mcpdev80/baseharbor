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

func TestUnavailableProviderFailsClosed(t *testing.T) {
	if _, err := Descriptor(runtimecontract.ProviderKubernetes); err == nil {
		t.Fatal("unregistered Kubernetes descriptor unexpectedly resolved")
	}
	if _, err := RuntimeProvider(context.Background(), runtimecontract.ProviderKubernetes); err == nil {
		t.Fatal("unregistered Kubernetes provider unexpectedly resolved")
	}
}
