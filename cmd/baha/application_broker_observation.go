package main

import (
	"context"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func observeBrokerProviderFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return
	}
	inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(resolved.Target.Name))
	if err != nil {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	if _, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources); err != nil {
		return
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return
	}
	for index, services := range [][]string{inventory.SQLServices, inventory.SecretsServices} {
		resources := canonicalProviderServiceResources(inventory.Files.Project, inventory.ContinuousServices(services))
		count, state := observeCanonicalProviderContainers(provider.Kind(), inventory.Files.Project, resources, containers)
		oneShots := []string{}
		for _, service := range inventory.OneShotServices {
			for _, owned := range services {
				if owned == service {
					oneShots = append(oneShots, service)
				}
			}
		}
		completedCount, completed := observeCanonicalProviderCompletion(provider.Kind(), inventory.Files.Project, canonicalProviderServiceResources(inventory.Files.Project, oneShots), containers)
		count += completedCount
		if state == application.ProviderRunning && !completed {
			state = application.ProviderUnverifiable
		}
		fact := application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreRequired, Dependencies: []string{}, MemoryBytes: application.Quantity{Classification: "unknown"}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "canonical protected native module with inspected ownership"}}
		if index == 0 {
			fact.Ancillary = true
			fact.OwnerReference = root
			facts[application.RuntimeStorageDependencyID(resolved.Target.Name)] = fact
			continue
		}
		fact.Dependencies = []string{application.RuntimeStorageDependencyID(resolved.Target.Name)}
		management := fact
		management.Ancillary, management.OwnerReference = true, root
		facts[managementSecretsDependency(resolved.Target.Name).ID] = management
		for _, instance := range registry.Instances {
			if instance.Provider.Kind == capability.ProviderOpenBao && instance.Scope == capability.ScopeShared && instance.Ownership == capability.OwnershipBaseHarbor && instance.SharingBoundary == "" && instance.Reference == "" {
				facts[instance.ID] = fact
			}
		}
	}
}
