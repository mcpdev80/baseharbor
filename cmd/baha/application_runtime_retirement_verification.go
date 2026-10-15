package main

import (
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Verify physical absence separately from release of a containerless logical
// SQL module whose canonical physical Core owner remains required.
func verifyApplicationRuntimeRetirement(resolved resolvedApplication, plan application.ProviderPlan, observed map[string]application.ProviderFact) error {
	for _, change := range plan.Changes {
		if change.Action != "stop" && change.Action != "remove-runtime" {
			continue
		}
		fact, found := observed[change.InstanceID]
		zero := found && fact.Target == resolved.Target.Name && fact.Containers.Value != nil && fact.Containers.Classification == "measured" && *fact.Containers.Value == 0
		if zero && (fact.State == application.ProviderStoppedOwned || verifiedReleasedCoreSQLReference(resolved, plan, change, observed)) {
			continue
		}
		return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "runtime_retirement_unverified", Resource: change.InstanceID, Message: "Selected runtime retirement could not be verified.", Next: "Inspect the retained deployment owner and retry; data and previous intent remain available."}
	}
	return nil
}

func verifiedReleasedCoreSQLReference(resolved resolvedApplication, plan application.ProviderPlan, change application.ProviderChange, observed map[string]application.ProviderFact) bool {
	if !plan.Complete || plan.Version != application.ProviderPlanVersion || plan.Target != resolved.Target.Name || change.Ownership != capability.OwnershipBaseHarbor || len(change.Consumers) != 0 {
		return false
	}
	inventory, err := application.ExistingSharedBackendInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, "core")
	if err != nil || inventory.CoreSQL == nil || len(inventory.Services[capability.ProviderPostgreSQL]) != 0 || observeReusedCoreSQLState(resolved, inventory.CoreSQL, observed) != application.ProviderRunning {
		return false
	}
	moduleID := application.SharedBackendRuntimeDependency(resolved.Target.Name, capability.ProviderPostgreSQL).ID
	storageID := application.RuntimeStorageDependencyID(resolved.Target.Name)
	module := observed[moduleID]
	if module.Target != resolved.Target.Name || module.State != application.ProviderRunning || !module.Ancillary || module.OwnerReference != "runtime/"+inventory.Files.Project || module.Core != application.CoreRequired || module.Containers.Value == nil || module.Containers.Classification != "measured" || *module.Containers.Value != 0 || len(module.Dependencies) != 1 || module.Dependencies[0] != storageID {
		return false
	}
	protected := false
	for _, physical := range plan.Changes {
		if physical.InstanceID != storageID || physical.Action != "reuse" || physical.Ownership != capability.OwnershipBaseHarbor {
			continue
		}
		for _, consumer := range physical.Consumers {
			_, exists := plan.Consumers[consumer]
			protected = protected || exists && plan.ConsumerActivity[consumer]
		}
	}
	if !protected {
		return false
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return false
	}
	logical := map[string]bool{}
	for _, instance := range registry.Instances {
		if instance.Scope == capability.ScopeShared && instance.Ownership == capability.OwnershipBaseHarbor && instance.Provider.Kind == capability.ProviderPostgreSQL {
			logical[instance.ID] = true
		}
	}
	for _, binding := range registry.Bindings {
		if logical[binding.ProviderInstanceID] {
			return false // The selected empty module has not released all references.
		}
	}
	released := false
	for _, binding := range plan.ReleasedBindings {
		released = released || logical[binding.ProviderInstanceID] && binding.ApplicationID == resolved.Manifest.ApplicationID && binding.Environment == resolved.Manifest.Environment && binding.Resource.Provider == capability.ProviderPostgreSQL
	}
	if !released {
		return false
	}
	if change.InstanceID == moduleID {
		return true
	}
	fact := observed[change.InstanceID]
	return logical[change.InstanceID] && !fact.Ancillary && fact.State == application.ProviderRunning && fact.Core == application.CoreRequired && len(fact.Dependencies) == 1 && fact.Dependencies[0] == moduleID
}
