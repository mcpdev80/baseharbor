package application

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Project the shared resolver's required instances onto the existing physical
// module. Multiple environment bindings can use this same module: releasing a
// logical binding must not stop another consumer's PostgreSQL or Valkey.
func startSharedBackendRuntime(ctx context.Context, provider bhruntime.RuntimeProvider, dataDir, namespace string, m Manifest, files SharedBackendFiles, plans []ProviderPlan) error {
	if len(plans) == 0 {
		return provider.UpProject(ctx, files.Project, files.Compose, files.Env)
	}
	registry, err := PreviewReferenceProviderRegistryAt(dataDir, m)
	if err != nil {
		return err
	}
	_, err = ConvergeExistingSharedBackendRuntime(ctx, provider, dataDir, namespace, plans[0], registry)
	return err
}

func sharedBackendSelectedServices(inventory SharedBackendInventory, plan ProviderPlan, registry capability.Registry) (wanted, unwanted []string, err error) {
	if plan.Version != ProviderPlanVersion || plan.Target == "" {
		return nil, nil, errors.New("shared backend convergence requires the resolved provider plan")
	}
	if err := registry.Validate(); err != nil {
		return nil, nil, err
	}
	instances := map[string]capability.ProviderInstance{}
	for _, instance := range registry.Instances {
		instances[instance.ID] = instance
	}
	kinds := map[capability.ProviderKind]bool{}
	retirable := map[capability.ProviderKind]bool{}
	preserved := map[capability.ProviderKind]bool{}
	actions := map[string]string{}
	for _, change := range plan.Changes {
		actions[change.InstanceID] = change.Action
	}
	for _, instance := range registry.Instances {
		if instance.Scope != capability.ScopeShared || instance.Ownership != capability.OwnershipBaseHarbor {
			continue
		}
		if actions[instance.ID] == "stop" || actions[instance.ID] == "remove-runtime" {
			retirable[instance.Provider.Kind] = true
		} else {
			preserved[instance.Provider.Kind] = true
		}
	}
	ancillary := map[string]bool{}
	for _, footprint := range plan.Consumers {
		for _, group := range [][]ProviderFootprint{footprint.Existing, footprint.Additional} {
			for _, provider := range group {
				ancillary[provider.InstanceID] = provider.Ancillary
			}
		}
	}
	for _, id := range plan.Required {
		instance, found := instances[id]
		if !found {
			// Ancillary storage belongs to its distinct native owner; it is not
			// another app SQL offer or a standalone shared-backend service.
			if !ancillary[id] {
				return nil, nil, fmt.Errorf("required shared provider %s is absent from the resolved inventory", id)
			}
			continue
		}
		if instance.Scope == capability.ScopeShared && instance.Ownership == capability.OwnershipBaseHarbor {
			kinds[instance.Provider.Kind] = true
		}
	}
	for kind, services := range inventory.Services {
		if kinds[kind] {
			wanted = append(wanted, services...)
		} else if plan.Complete && retirable[kind] && !preserved[kind] {
			unwanted = append(unwanted, services...)
		}
	}
	sort.Strings(wanted)
	sort.Strings(unwanted)
	return wanted, unwanted, nil
}

// ConvergeExistingSharedBackendRuntime preserves the existing data owner and
// credentials. It uses selected-service lifecycle operations on the canonical
// module rather than removing databases, roles, ACLs or recovery material.
// The existing mutation authority must acknowledge and revalidate the shared
// impact plan before calling this adapter.
func ConvergeExistingSharedBackendRuntime(ctx context.Context, provider bhruntime.RuntimeProvider, dataDir, namespace string, plan ProviderPlan, registry capability.Registry) (bhruntime.RuntimeRetirement, error) {
	result := bhruntime.RuntimeRetirement{Removed: []bhruntime.ProjectResource{}, Retained: []bhruntime.RetainedRuntimeResource{}}
	if provider == nil || plan.Target != namespace {
		return result, errors.New("shared backend convergence binding differs from its Target")
	}
	inventory, err := ExistingSharedBackendInventoryAt(dataDir, namespace, "core")
	if err != nil {
		return result, err
	}
	wanted, unwanted, err := sharedBackendSelectedServices(inventory, plan, registry)
	if err != nil {
		return result, err
	}
	before, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
	if err != nil {
		return result, fmt.Errorf("verify shared backend ownership before convergence: %w", err)
	}
	environment, err := readRuntimeEnv(inventory.Files.Env)
	if err != nil {
		return result, err
	}
	// An empty selected-services slice means ALL services in native Up, so never
	// call it when the last consumer was removed.
	if len(wanted) > 0 {
		if err := provider.UpProjectFilesSelected(ctx, inventory.Files.Project, inventory.Files.Dir, environment, wanted, inventory.Files.Compose); err != nil {
			return result, fmt.Errorf("start selected shared backend services; retain owner for retry: %w", err)
		}
	}
	// Keep required siblings available before retiring released services; a
	// failed start must preserve the existing runtime for recovery and retry.
	if len(unwanted) > 0 {
		native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
		if !ok {
			return result, errors.New("shared backend retirement requires a verified selected-container adapter")
		}
		selected := []bhruntime.ProjectResource{}
		for _, service := range unwanted {
			selected = append(selected, bhruntime.ProjectResource{Kind: "container", Name: inventory.Files.Project + "-" + service + "-1"})
		}
		result, err = bhruntime.RetireOwnedServices(ctx, native, inventory.Files.Project, inventory.Resources, selected)
		if err != nil {
			return result, fmt.Errorf("retire unused shared backend services; retain owner for retry: %w", err)
		}
	}
	after, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
	if err != nil {
		return result, fmt.Errorf("verify shared backend resources after convergence: %w", err)
	}
	for _, resource := range before {
		if resource.Kind == "volume" {
			if !ResourceNamedExists(after, resource) {
				return result, fmt.Errorf("shared backend convergence lost volume %s; retain owner for recovery", resource.Name)
			}
			alreadyRetained := false
			for _, retained := range result.Retained {
				alreadyRetained = alreadyRetained || retained.Resource == resource
			}
			if !alreadyRetained {
				result.Retained = append(result.Retained, bhruntime.RetainedRuntimeResource{Resource: resource, Reason: "persistent shared data remains at its existing owned volume; no database, credential or recovery deletion"})
			}
		}
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return result, err
	}
	running := map[string]bool{}
	for _, container := range containers {
		if container.Project == inventory.Files.Project {
			running[container.Service] = running[container.Service] || container.Running
		}
	}
	for _, service := range wanted {
		if !running[service] {
			return result, fmt.Errorf("required shared backend service %s is not running", service)
		}
	}
	for _, service := range unwanted {
		if running[service] {
			return result, fmt.Errorf("unused shared backend service %s remains running", service)
		}
	}
	return result, nil
}
