package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func applicationExposureInstance(instance capability.ProviderInstance, m application.Manifest) bool {
	return instance.Provider.Kind == capability.ProviderCaddy && instance.Scope == capability.ScopeApplication && instance.Ownership == capability.OwnershipBaseHarbor && instance.OwnerApplicationID == m.ApplicationID && instance.OwnerEnvironment == m.Environment
}

func observeApplicationExposureFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	inventory, err := exposure.ExistingRuntimeInventory(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	if err != nil {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	if _, err := provider.InspectProjectResources(ctx, inventory.State.Project, inventory.Resources); err != nil {
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
	count, state := int64(0), application.ProviderStoppedOwned
	for _, container := range containers {
		for _, resource := range inventory.Resources {
			if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, inventory.State.Project, resource) {
				count++
				if container.Running {
					state = application.ProviderRunning
				}
				break
			}
		}
	}
	for _, instance := range registry.Instances {
		if applicationExposureInstance(instance, resolved.Manifest) {
			facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreNotRequired, Dependencies: []string{}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "protected canonical exposure renderer and native ownership inventory"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
		}
	}
}

func retireApplicationExposureRuntime(ctx context.Context, resolved resolvedApplication, provider bhruntime.RuntimeProvider, instance capability.ProviderInstance, plan application.ProviderPlan, out io.Writer) error {
	if !applicationExposureInstance(instance, resolved.Manifest) || len(resolved.Manifest.Exposures) != 0 || !plan.Complete || plan.Target != resolved.Target.Name || plan.Version != application.ProviderPlanVersion {
		return invalidProviderConsumerInventory(fmt.Errorf("exposure retirement requires its complete unused application owner plan"))
	}
	result, err := exposure.RetireRuntime(ctx, provider, resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	for _, item := range result.Removed {
		fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", item.Kind, item.Name)
	}
	for _, item := range result.Retained {
		fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
	}
	return err
}
