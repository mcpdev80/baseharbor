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
		resources := canonicalProviderServiceResources(inventory.Files.Project, inventory.Services[kind])
		count, state := observeCanonicalProviderContainers(provider.Kind(), inventory.Files.Project, resources, containers)
		if kind == capability.ProviderPostgreSQL && inventory.CoreSQL != nil && len(resources) == 0 {
			// This logical module reuses the already observed physical SQL owner.
			// It contributes no additional containers to the Target footprint.
			state = observeReusedCoreSQLState(resolved, inventory.CoreSQL, facts)
		}
		zero := int64(0)
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
