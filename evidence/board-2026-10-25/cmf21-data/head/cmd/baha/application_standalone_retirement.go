package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func retireStandaloneApplicationProvider(ctx context.Context, resolved resolvedApplication, provider bhruntime.RuntimeProvider, instance capability.ProviderInstance, registry capability.Registry, out io.Writer) error {
	native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
	if !ok {
		return fmt.Errorf("runtime has no selected-container retirement adapter")
	}
	owners, err := applicationRuntimeOwners(resolved)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		inventory, err := existingStandaloneProviderInventory(owner, provider, instance, registry)
		if err != nil {
			continue
		}
		selected := []bhruntime.ProjectResource{}
		for _, resource := range inventory.resources {
			if resource.Kind == "container" {
				selected = append(selected, resource)
			}
		}
		result, operationErr := bhruntime.RetireOwnedServices(ctx, native, inventory.project, inventory.resources, selected)
		for _, resource := range result.Removed {
			fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", resource.Kind, resource.Name)
		}
		for _, item := range result.Retained {
			fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
		}
		fmt.Fprintf(out, "Retained %s recovery files at %s; runtime retirement does not prove continued consumer access or transient-data retention.\n", instance.Provider.Kind, inventory.dir)
		return operationErr
	}
	return invalidProviderConsumerInventory(fmt.Errorf("provider %s has no verifiable retained canonical runtime owner", instance.ID))
}
