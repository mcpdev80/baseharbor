package runtimeexplorer

import (
	"context"
	"testing"
)

type inventoryFakeBackend struct {
	*fakeBackend
	items map[ResourceKind][]InventoryResource
}

func (f *inventoryFakeBackend) InventoryResourceKinds() []ResourceKind {
	return []ResourceKind{KindImage, KindVolume, KindNetwork}
}

func (f *inventoryFakeBackend) ListInventoryResources(_ context.Context, kind ResourceKind) ([]InventoryResource, error) {
	return append([]InventoryResource(nil), f.items[kind]...), nil
}

func TestServiceListsAndInspectsReadOnlyInventoryKinds(t *testing.T) {
	backend := &inventoryFakeBackend{
		fakeBackend: &fakeBackend{},
		items: map[ResourceKind][]InventoryResource{
			KindImage: {
				{Kind: KindImage, ResourceID: "sha256:image", Name: "example/api:v1"},
			},
			KindVolume: {
				{Kind: KindVolume, ResourceID: "volume-data", Name: "volume-data"},
			},
			KindNetwork: {
				{Kind: KindNetwork, ResourceID: "network-id", Name: "backend"},
			},
		},
	}
	service, err := NewService(backend, "local", nil)
	if err != nil {
		t.Fatal(err)
	}

	caps, err := service.Capabilities(context.Background(), "local")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []ResourceKind{KindContainer, KindImage, KindVolume, KindNetwork} {
		if !containsResourceKind(caps.ResourceKinds, kind) {
			t.Fatalf("capabilities missing resource kind %q: %#v", kind, caps.ResourceKinds)
		}
	}

	resources, err := service.List(context.Background(), ListRequest{Target: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 3 {
		t.Fatalf("inventory resources = %d, want 3: %#v", len(resources), resources)
	}
	for _, resource := range resources {
		if resource.Ownership != OwnershipUnmanaged {
			t.Fatalf("inventory ownership guessed unexpectedly: %#v", resource)
		}
	}

	network, err := service.Inspect(context.Background(), ResourceRef{
		Provider: "docker", Target: "local", Kind: KindNetwork, ResourceID: "network-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if network.RuntimeName != "backend" || network.Ref.ResourceID != "network-id" {
		t.Fatalf("network inspect = %#v", network)
	}

	_, err = service.Operate(context.Background(), OperationRequest{
		Resource:  network.Ref,
		Operation: OperationStop,
	})
	if err == nil {
		t.Fatal("non-container inventory resource mutation unexpectedly succeeded")
	}
}
