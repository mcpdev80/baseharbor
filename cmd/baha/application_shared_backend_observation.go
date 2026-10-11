package main

import (
	"context"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func observeSharedBackendProviderFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	inventory, err := application.ExistingSharedBackendInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, "core")
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
	for _, kind := range []capability.ProviderKind{capability.ProviderPostgreSQL, capability.ProviderValkey} {
		wanted := map[string]bool{}
		for _, service := range inventory.Services[kind] {
			wanted[service] = true
		}
		count, zero := int64(0), int64(0)
		state := application.ProviderStoppedOwned
		for _, container := range containers {
			if container.Project == inventory.Files.Project && wanted[container.Service] {
				count++
				if container.Running {
					state = application.ProviderRunning
				}
			}
		}
		module := application.SharedBackendRuntimeDependency(resolved.Target.Name, kind)
		dependencies := []string{}
		core := application.CoreNotRequired
		if kind == capability.ProviderPostgreSQL && inventory.CoreSQL != nil {
			dependencies = append(dependencies, application.RuntimeStorageDependencyID(resolved.Target.Name))
			core = application.CoreRequired
		}
		facts[module.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: core, Dependencies: dependencies, Ancillary: true, OwnerReference: "runtime/" + inventory.Files.Project, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "canonical shared module with inspected runtime ownership"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
		for _, instance := range registry.Instances {
			if instance.Scope == capability.ScopeShared && instance.Ownership == capability.OwnershipBaseHarbor && instance.Provider.Kind == kind {
				facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: core, Dependencies: []string{module.ID}, Containers: application.Quantity{Value: &zero, Classification: "measured", Source: "logical binding; physical runtime counted once in its ancillary module"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
			}
		}
	}
}
