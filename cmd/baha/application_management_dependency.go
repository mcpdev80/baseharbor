package main

import (
	"errors"
	"os"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func managementSecretsDependency(target string) capability.ProviderInstance {
	return capability.ProviderInstance{ID: "runtime/" + bhruntime.SharedProjectName(target) + "/control-plane/openbao", Provider: capability.Provider{Kind: capability.ProviderOpenBao}, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
}

// An independently requested installation protects its physical modules, even
// without any application registry offer. This is an ephemeral projection of
// its existing owner, not a new registration or an implicit application Core.
func appendManagementProviderDependencies(resolved resolvedApplication, observations map[string]application.ProviderFact, snapshot *application.ResolutionSnapshot) ([]string, error) {
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return nil, err
	}
	state, err := coreinstallation.Load(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	if state.Spec.Target != resolved.Target.Name || state.Spec.Runtime != resolved.Target.RuntimeProvider {
		return nil, invalidProviderConsumerInventory(errors.New("installation binding differs from selected Target"))
	}
	identity, identityOwner, err := identityprovider.CoreIdentityDependency(resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	for _, item := range []struct {
		instance capability.ProviderInstance
		owner    string
	}{{managementSecretsDependency(resolved.Target.Name), root}, {identity, identityOwner}} {
		fact := application.ProviderFact{Target: resolved.Target.Name, State: application.ProviderUnverifiable, Core: application.CoreRequired, Ancillary: true, OwnerReference: item.owner, Dependencies: []string{application.RuntimeStorageDependencyID(resolved.Target.Name)}}
		if observed, ok := observations[item.instance.ID]; ok && len(observed.Dependencies) == 1 && observed.Dependencies[0] == application.RuntimeStorageDependencyID(resolved.Target.Name) {
			fact = observed
			fact.Ancillary, fact.OwnerReference = true, item.owner
			fact.Dependencies = []string{application.RuntimeStorageDependencyID(resolved.Target.Name)}
		}
		snapshot.Ancillary = append(snapshot.Ancillary, item.instance)
		snapshot.Facts[item.instance.ID] = fact
	}
	return managementProviderDependencyRoots(resolved, *snapshot)
}

func managementProviderDependencyRoots(resolved resolvedApplication, snapshot application.ResolutionSnapshot) ([]string, error) {
	identity, _, err := identityprovider.CoreIdentityDependency(resolved.TargetStateRoot, resolved.Target.Name)
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	roots := []string{identity.ID, managementSecretsDependency(resolved.Target.Name).ID}
	// Shared native Keycloak aliases all use the canonical core module. Keep
	// those aliases too so retirement cannot remove that physical owner through
	// an application binding. Application-scoped and external modules are separate.
	for _, instance := range snapshot.Registry.Instances {
		if instance.Ownership != capability.OwnershipBaseHarbor || instance.Scope != capability.ScopeShared {
			continue
		}
		if instance.Provider.Kind == capability.ProviderKeycloak || instance.Provider.Kind == capability.ProviderOpenBao && instance.SharingBoundary == "" && instance.Reference == "" {
			roots = append(roots, instance.ID)
		}
	}
	sort.Strings(roots)
	return roots, nil
}

// Resolve the same native management module for facts and impact inventory.
func existingManagementIdentity(resolved resolvedApplication) (capability.ProviderInstance, bool) {
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		return capability.ProviderInstance{}, false
	}
	state, err := coreinstallation.Load(root)
	if err != nil || state.Spec.Target != resolved.Target.Name || state.Spec.Runtime != resolved.Target.RuntimeProvider {
		return capability.ProviderInstance{}, false
	}
	instance, _, err := identityprovider.CoreIdentityDependency(resolved.TargetStateRoot, resolved.Target.Name)
	return instance, err == nil
}
