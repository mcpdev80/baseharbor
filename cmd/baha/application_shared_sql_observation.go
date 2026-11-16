package main

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// A shared logical SQL module can reuse the physical Core SQL owner without
// declaring another container. Only that exact canonical owner's observation
// supplies availability; retained paths alone are not running-state evidence.
func observeReusedCoreSQLState(resolved resolvedApplication, selected *bhruntime.Files, facts map[string]application.ProviderFact) application.ProviderState {
	if selected == nil {
		return application.ProviderUnverifiable
	}
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return application.ProviderUnverifiable
	}
	inventory, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, bhruntime.SharedProjectName(resolved.Target.Name))
	if err != nil || *selected != inventory.Files {
		return application.ProviderUnverifiable
	}
	fact, observed := facts[application.RuntimeStorageDependencyID(resolved.Target.Name)]
	if !observed || fact.Target != resolved.Target.Name || !fact.Ancillary || fact.OwnerReference != root || fact.Core != application.CoreRequired || fact.Dependencies == nil || len(fact.Dependencies) != 0 || fact.Containers.Value == nil || fact.Containers.Classification != "measured" {
		return application.ProviderUnverifiable
	}
	if *fact.Containers.Value < 0 || fact.State == application.ProviderRunning && *fact.Containers.Value == 0 {
		return application.ProviderUnverifiable
	}
	if fact.State != application.ProviderRunning && fact.State != application.ProviderStoppedOwned {
		return application.ProviderUnverifiable
	}
	return fact.State
}
