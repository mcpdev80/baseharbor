package main

import (
	"context"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Inventory the existing executable owner, including the prior applied intent.
// A named volume proves location/authority, not SQL contents, queue delivery,
// cache/session durability or a successful replacement.
func observeNativeBackendImpact(ctx context.Context, resolved resolvedApplication, instance capability.ProviderInstance, fact *application.ProviderImpactFact) {
	files, prior, owned, err := nativeBackendInventory(resolved)
	if err != nil {
		return
	}
	selected, err := application.NativeBackendInstanceResources(prior, files, instance)
	if err != nil {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	actual, err := provider.InspectProjectResources(ctx, files.Project, owned)
	if err != nil {
		return
	}
	selectedNames := map[string]bool{}
	for _, resource := range selected {
		selectedNames[resource.Kind+"/"+resource.Name] = true
	}
	fact.Realization.Supported, fact.Realization.Verified = true, true
	fact.Realization.Evidence = "existing canonical native backend renderer and exact owned runtime inspection"
	for _, resource := range actual {
		if resource.Kind != "volume" || !selectedNames[resource.Kind+"/"+resource.Name] {
			continue
		}
		location := application.ResourceLocation{Owner: fact.Realization.Owner, Target: resolved.Target.Name, Kind: "volume", Reference: resource.Name}
		fact.Realization.Locations = append(fact.Realization.Locations, location)
		fact.Materials = append(fact.Materials, application.ProviderMaterial{ID: instance.ID + "/volume/" + resource.Name, Kind: "persistent-provider-data", Ownership: capability.OwnershipBaseHarbor, Location: &location, Recovery: []application.ResourceLocation{}, Verified: true, Evidence: "observed exact owned persistent volume; contents, backups and application access not verified"})
	}
	materials, complete, err := observeOwnedProviderFiles(fact.Realization.Owner, resolved.Target.Name, instance.ID, files.Dir)
	fact.Materials = append(fact.Materials, materials...)
	if !complete || err != nil {
		fact.Realization.Evidence += "; protected file inventory incomplete"
	}
	fact.InventoryComplete = false // Transient work and actual consumer behavior remain unknown.
	sort.Slice(fact.Materials, func(i, j int) bool { return fact.Materials[i].ID < fact.Materials[j].ID })
}
