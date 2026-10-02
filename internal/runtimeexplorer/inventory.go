package runtimeexplorer

import (
	"context"
	"strings"
)

type InventoryResource struct {
	Kind       ResourceKind
	ResourceID string
	Name       string
	State      string
	Health     string
	References map[string]string
}

type InventoryBackend interface {
	InventoryResourceKinds() []ResourceKind
	ListInventoryResources(context.Context, ResourceKind) ([]InventoryResource, error)
}

func inventoryResource(target, provider string, item InventoryResource) (Resource, error) {
	ready := false
	var readyPtr *bool
	if strings.TrimSpace(item.State) != "" || strings.TrimSpace(item.Health) != "" {
		ready = inventoryReady(item.State, item.Health)
		readyPtr = &ready
	}
	resource := Resource{
		ContractVersion: ContractVersion,
		Ref: ResourceRef{
			Provider:   provider,
			Target:     target,
			Kind:       item.Kind,
			ResourceID: strings.TrimSpace(item.ResourceID),
		},
		DisplayName: strings.TrimSpace(item.Name),
		RuntimeName: strings.TrimSpace(item.Name),
		Ownership:   OwnershipUnmanaged,
		State: ResourceState{
			Observed: strings.TrimSpace(item.State),
			Health:   strings.TrimSpace(item.Health),
			Ready:    readyPtr,
		},
		References: copyReferences(item.References),
	}
	if err := resource.Validate(); err != nil {
		return Resource{}, err
	}
	return resource, nil
}

func inventoryReady(state, health string) bool {
	state = strings.ToLower(strings.TrimSpace(state))
	health = strings.ToLower(strings.TrimSpace(health))
	if health != "" && health != "healthy" && health != "running" && health != "up" {
		return false
	}
	switch state {
	case "", "running", "up", "active", "created":
		return true
	default:
		return false
	}
}

func copyReferences(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if strings.TrimSpace(value) != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
