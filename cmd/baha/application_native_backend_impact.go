package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	// Protected file ownership is independently observable even when the
	// runtime is unavailable. Keep its revision bound to consent without
	// claiming live provider authority or content recovery.
	// Native database material belongs to this exact provider, not every file
	// in the application's Compose project.
	materialRoot := files.Dir
	sqlName := ""
	if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeApplication &&
		instance.OwnerApplicationID == resolved.Manifest.ApplicationID {
		projected, projectErr := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
		if projectErr == nil {
			for _, binding := range projected.Bindings {
				if binding.ApplicationID == resolved.Manifest.ApplicationID &&
					binding.Environment == resolved.Manifest.Environment &&
					binding.ProviderInstanceID == instance.ID && binding.Resource.Kind == capability.SQL {
					sqlName = binding.Resource.Name
				}
			}
		}
		if sqlName != "" {
			materialRoot = filepath.Join(files.Dir, "providers", "postgresql", sqlName)
		}
	}
	materials, complete, fileErr := observeOwnedProviderFiles(fact.Realization.Owner, resolved.Target.Name, instance.ID, materialRoot)
	fact.Materials = append(fact.Materials, materials...)
	sort.Slice(fact.Materials, func(i, j int) bool { return fact.Materials[i].ID < fact.Materials[j].ID })
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
	if sqlName != "" {
		// The desired canonical renderer identifies which owned resources to
		// inspect. Only resources actually returned by the native inspector
		// become recorded materials; desired definitions are not receipts.
		if desired, err := application.NativeBackendInstanceResources(resolved.Manifest, files, instance); err == nil {
			for _, resource := range desired {
				selectedNames[resource.Kind+"/"+resource.Name] = true
			}
		}
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
	if !complete || fileErr != nil {
		fact.Realization.Evidence += "; protected file inventory incomplete"
	}
	// Existing SQL contents are never assumed transferable. Only a source
	// with no canonical provider resources and no protected provider material,
	// or a freshly empty database queried with the retained application SQL
	// credential, can be classified complete in this bounded creation case.
	fact.InventoryComplete = false
	if instance.Provider.Kind == capability.ProviderPostgreSQL && instance.Scope == capability.ScopeApplication &&
		instance.OwnerApplicationID == resolved.Manifest.ApplicationID && sqlName != "" {
		expected, expectedErr := application.NativeBackendInstanceResources(resolved.Manifest, files, instance)
		seen := false
		if expectedErr == nil {
			for _, candidate := range expected {
				for _, resource := range actual {
					if candidate.Kind == resource.Kind && candidate.Name == resource.Name {
						seen = true
					}
				}
			}
		}
		priorHasSQL := false
		for _, name := range application.SQLInstanceNames(prior) {
			if name == sqlName {
				priorHasSQL = true
			}
		}
		if expectedErr == nil && !seen && !priorHasSQL && errors.Is(fileErr, os.ErrNotExist) {
			fact.Materials = []application.ProviderMaterial{}
			fact.InventoryComplete = true
			fact.Realization.Evidence += "; canonical Target project inspected: no owned SQL resources or prior protected SQL material"
		} else if seen && complete && fileErr == nil {
			inventory, probeErr := application.ObserveSQLConsumerInventory(ctx,
				application.NewRuntimeBackendProbeExecutor(provider, files),
				files, resolved.Manifest, sqlName)
			if probeErr == nil && inventory.Empty &&
				inventory.Consumer == "application/"+resolved.Manifest.ApplicationID+"/"+resolved.Manifest.Environment {
				fact.InventoryComplete = true
				fact.Realization.Evidence += "; " + inventory.Evidence + "; no consumer SQL objects or large objects found"
			}
		}
	}
	sort.Slice(fact.Materials, func(i, j int) bool { return fact.Materials[i].ID < fact.Materials[j].ID })
}

const sqlApplyReadLimit = 16

// Collect only actual SQL consumers in the applied Target registry. The probe
// reads retained credentials within the native owner and returns no secrets.
// Cancellation never authorizes writes; this bounded post-run read uses the
// same lock and retains errors as missing evidence.
func observeNativeSQLApplyResults(ctx context.Context, resolved resolvedApplication, registry capability.Registry) map[string]application.SQLConsumerInventory {
	observed := map[string]application.SQLConsumerInventory{}
	if isRemoteApplication(resolved) || len(registry.Bindings) > sqlApplyReadLimit {
		return observed
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appApplyEvidenceObservationTimeout)
	defer cancel()
	provider, err := detectRuntimeForTarget(readCtx, resolved.Target)
	if err != nil {
		return observed
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	for _, binding := range registry.Bindings {
		if readCtx.Err() != nil {
			break
		}
		if binding.ApplicationID != resolved.Manifest.ApplicationID ||
			binding.Environment != resolved.Manifest.Environment ||
			binding.Resource.Kind != capability.SQL {
			continue
		}
		for _, instance := range registry.Instances {
			if instance.ID != binding.ProviderInstanceID ||
				instance.Provider.Kind != capability.ProviderPostgreSQL ||
				instance.Ownership != capability.OwnershipBaseHarbor ||
				instance.Scope != capability.ScopeApplication ||
				instance.OwnerApplicationID != resolved.Manifest.ApplicationID {
				continue
			}
			inventory, err := application.ObserveSQLConsumerInventory(readCtx,
				application.NewRuntimeBackendProbeExecutor(provider, files),
				files, resolved.Manifest, binding.Resource.Name)
			if err == nil && inventory.Consumer == "application/"+resolved.Manifest.ApplicationID+"/"+resolved.Manifest.Environment {
				observed[instance.ID] = inventory
			}
		}
	}
	return observed
}
