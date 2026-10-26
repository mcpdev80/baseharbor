package application

import (
	"context"
	"errors"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RetireUnneededControlPlaneServices projects the common consumer plan onto
// the existing native module. It neither resolves dependencies nor deletes
// storage, credentials, installation records or recovery material.
func RetireUnneededControlPlaneServices(ctx context.Context, provider bhruntime.RuntimeProvider, root, target string, plan ProviderPlan, registry capability.Registry) (bhruntime.RuntimeRetirement, error) {
	result := bhruntime.RuntimeRetirement{Removed: []bhruntime.ProjectResource{}, Retained: []bhruntime.RetainedRuntimeResource{}}
	if plan.Version != ProviderPlanVersion || plan.Target != target {
		return result, errors.New("control-plane retirement requires its resolved Target plan")
	}
	if err := registry.Validate(); err != nil {
		return result, err
	}
	instances := map[string]capability.ProviderInstance{}
	for _, instance := range registry.Instances {
		instances[instance.ID] = instance
	}
	module := func(id string) string {
		if id == RuntimeStorageDependencyID(target) {
			return "sql"
		}
		instance := instances[id]
		if instance.Scope == capability.ScopeShared && instance.Ownership == capability.OwnershipBaseHarbor && instance.Provider.Kind == capability.ProviderOpenBao {
			return "secrets"
		}
		return ""
	}
	required := map[string]bool{}
	for _, id := range plan.Required {
		required[module(id)] = true
	}
	remove := map[string]bool{}
	for _, change := range plan.Changes {
		kind := module(change.InstanceID)
		if kind == "" || required[kind] || (change.Action != "stop" && change.Action != "remove-runtime") {
			continue
		}
		if !plan.Complete || change.Ownership != capability.OwnershipBaseHarbor || len(change.Consumers) > 0 {
			return result, errors.New("control-plane retirement has incomplete or conflicting consumer ownership")
		}
		remove[kind] = true
	}
	if len(remove) == 0 {
		return result, nil
	}
	inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(target))
	if err != nil {
		return result, err
	}
	selected := []bhruntime.ProjectResource{}
	for kind, services := range map[string][]string{"sql": inventory.SQLServices, "secrets": inventory.SecretsServices} {
		if !remove[kind] {
			continue
		}
		for _, service := range services {
			selected = append(selected, bhruntime.ProjectResource{Kind: "container", Name: inventory.Files.Project + "-" + service + "-1"})
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
	if !ok {
		return result, errors.New("runtime has no data-preserving control-plane retirement adapter")
	}
	return bhruntime.RetireOwnedServices(ctx, native, inventory.Files.Project, inventory.Resources, selected)
}
