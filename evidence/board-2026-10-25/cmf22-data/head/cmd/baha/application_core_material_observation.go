package main

import (
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Core storage is a physical ancillary module, not every PostgreSQL instance.
// Observe its shared protected owner without inferring consumer functions or
// live secret delivery from the product name or retained recovery files.
func observeExistingCoreProviderMaterial(resolved resolvedApplication, instance capability.ProviderInstance, fact *application.ProviderImpactFact) {
	if isRemoteApplication(resolved) || instance.Ownership != capability.OwnershipBaseHarbor {
		return
	}
	coreSecrets := instance.Provider.Kind == capability.ProviderOpenBao && instance.Scope == capability.ScopeShared && instance.SharingBoundary == "" && instance.Reference == ""
	coreStorage := instance.ID == application.RuntimeStorageDependencyID(resolved.Target.Name) && instance.Provider.Kind == capability.ProviderPostgreSQL && fact.Realization.Ancillary
	if !coreSecrets && !coreStorage {
		return
	}
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return
	}
	// Only an exact canonical Core module can establish this relationship.
	// In particular, a registry entry alone never adopts a foreign directory.
	if _, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, targetRuntimeProjectName(resolved.Target)); err != nil {
		return
	}
	materials, _, _ := observeOwnedProviderFiles(fact.Realization.Owner, resolved.Target.Name, instance.ID, root)
	fact.Materials = append(fact.Materials, materials...)
	fact.Realization.Locations = append(fact.Realization.Locations, application.ResourceLocation{
		Owner: fact.Realization.Owner, Target: resolved.Target.Name, Kind: "path", Reference: filepath.Clean(root),
	})
	// Shared configuration and recovery are observed, but neither lease/token
	// continuity nor native volume contents have been verified by this adapter.
	fact.InventoryComplete = false
	sort.Slice(fact.Materials, func(i, j int) bool { return fact.Materials[i].ID < fact.Materials[j].ID })
}
