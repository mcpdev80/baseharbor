package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func applicationNativeBackendInstance(instance capability.ProviderInstance, m application.Manifest) bool {
	if instance.Scope != capability.ScopeApplication || instance.Ownership != capability.OwnershipBaseHarbor || instance.OwnerApplicationID != m.ApplicationID || instance.OwnerEnvironment != m.Environment {
		return false
	}
	switch instance.Provider.Kind {
	case capability.ProviderPostgreSQL, capability.ProviderValkey, capability.ProviderRabbitMQ, capability.ProviderMongoDB:
		return true
	default:
		return false
	}
}

func retireApplicationNativeBackendServices(ctx context.Context, resolved resolvedApplication, provider bhruntime.RuntimeProvider, plan application.ProviderPlan, out io.Writer) error {
	files, _, previous, err := nativeBackendInventory(resolved)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	desired, err := application.NativeBackendResources(resolved.Manifest, files)
	if err != nil {
		return err
	}
	selected := releasedNativeBackendContainers(previous, desired)
	if len(selected) == 0 {
		return nil
	}
	if plan.Target != resolved.Target.Name || plan.Version != application.ProviderPlanVersion || !plan.Complete {
		return invalidProviderConsumerInventory(fmt.Errorf("native backend retirement requires complete resolved consumer inventory"))
	}
	native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
	if !ok {
		return fmt.Errorf("runtime %s has no data-preserving native container retirement adapter", provider.Kind())
	}
	result, err := bhruntime.RetireOwnedServices(ctx, native, files.Project, previous, selected)
	for _, resource := range result.Removed {
		fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", resource.Kind, resource.Name)
	}
	for _, retained := range result.Retained {
		fmt.Fprintf(out, "Retained %s %s: %s\n", retained.Resource.Kind, retained.Resource.Name, retained.Reason)
	}
	return err
}

func releasedNativeBackendContainers(previous, desired []bhruntime.ProjectResource) []bhruntime.ProjectResource {
	keep := map[bhruntime.ProjectResource]bool{}
	for _, resource := range desired {
		keep[resource] = true
	}
	selected := []bhruntime.ProjectResource{}
	for _, resource := range previous {
		if resource.Kind == "container" && !keep[resource] {
			selected = append(selected, resource)
		}
	}
	return selected
}
