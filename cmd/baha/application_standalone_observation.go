package main

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func observeStandaloneProviderFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return
	}
	for _, instance := range registry.Instances {
		if !standaloneProviderKind(instance.Provider.Kind) {
			continue
		}
		inventory, err := existingStandaloneProviderInventory(resolved, provider, instance, registry)
		if err != nil {
			continue
		}
		if _, err := provider.InspectProjectResources(ctx, inventory.project, inventory.resources); err != nil {
			continue
		}
		count, state := int64(0), application.ProviderStoppedOwned
		for _, container := range containers {
			for _, resource := range inventory.resources {
				if resource.Kind == "container" && bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, inventory.project, resource) {
					count++
					if container.Running {
						state = application.ProviderRunning
					}
					break
				}
			}
		}
		facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreNotRequired, Dependencies: inventory.dependencies, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "retained canonical provider renderer and ownership-checked selected module"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
	}
}

func observeStandaloneProviderImpact(ctx context.Context, resolved resolvedApplication, instance capability.ProviderInstance, fact *application.ProviderImpactFact) {
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return
	}
	owners, err := applicationRuntimeOwners(resolved)
	if err != nil {
		return
	}
	for _, owner := range owners {
		inventory, err := existingStandaloneProviderInventory(owner, provider, instance, registry)
		if err != nil {
			continue
		}
		resources, err := provider.InspectProjectResources(ctx, inventory.project, inventory.resources)
		if err != nil {
			continue
		}
		fact.Realization.Supported, fact.Realization.Verified = true, true
		fact.Realization.Evidence = "retained canonical provider definition and native selected-module ownership inspection"
		for _, resource := range resources {
			if resource.Kind != "volume" {
				continue
			}
			location := application.ResourceLocation{Owner: fact.Realization.Owner, Target: resolved.Target.Name, Kind: "volume", Reference: resource.Name}
			fact.Realization.Locations = append(fact.Realization.Locations, location)
			fact.Materials = append(fact.Materials, application.ProviderMaterial{ID: instance.ID + "/volume/" + resource.Name, Kind: "persistent-provider-data", Ownership: capability.OwnershipBaseHarbor, Location: &location, Recovery: []application.ResourceLocation{}, Verified: true, Evidence: "owned persistent volume; contents, retention and consumer access are not established by volume existence"})
		}
		materials, complete, fileErr := observeOwnedProviderFiles(fact.Realization.Owner, resolved.Target.Name, instance.ID, inventory.dir)
		fact.Materials = append(fact.Materials, materials...)
		if !complete || fileErr != nil {
			fact.Realization.Evidence += "; protected material inventory incomplete"
		}
		// Queued exports, retained telemetry/object contents and operational
		// consumers need separate proofs. No fictitious continuity receipt.
		fact.InventoryComplete = false
		sort.Slice(fact.Materials, func(i, j int) bool { return fact.Materials[i].ID < fact.Materials[j].ID })
		return
	}
}
