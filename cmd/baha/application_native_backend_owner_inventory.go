package main

import (
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Interrupted apply can materialize both applied and previous services. Read
// their existing canonical owner; a single desired renderer cannot prove absence.
func nativeBackendOwnerIntents(resolved resolvedApplication) ([]application.Manifest, error) {
	owners, err := applicationRuntimeOwners(resolved)
	if err != nil {
		return nil, err
	}
	intents := []application.Manifest{}
	for _, owner := range owners {
		if owner.Store.Root == resolved.Store.Root && owner.Manifest.ApplicationID == resolved.Manifest.ApplicationID && owner.Manifest.Environment == resolved.Manifest.Environment {
			intents = append(intents, owner.Manifest)
		}
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return nil, err
	}
	// A placement replacement retains the original application-scoped offer
	// even after its binding moves to shared storage. Its exact canonical
	// resource projection remains available for inspection and owned cleanup.
	for _, instance := range registry.Instances {
		if !applicationNativeBackendInstance(instance, resolved.Manifest) || (instance.Provider.Kind != capability.ProviderPostgreSQL && instance.Provider.Kind != capability.ProviderValkey) {
			continue
		}
		for _, owner := range owners {
			if owner.Store.Root != resolved.Store.Root || owner.Manifest.ApplicationID != resolved.Manifest.ApplicationID || owner.Manifest.Environment != resolved.Manifest.Environment {
				continue
			}
			projected, err := application.WithBackendPlacementProjection(owner.Manifest, map[capability.ProviderKind]capability.ProviderPlacement{instance.Provider.Kind: providerInstancePlacement(instance)})
			if err != nil {
				return nil, invalidProviderConsumerInventory(err)
			}
			intents = append(intents, projected)
		}
	}
	return intents, nil
}

func nativeBackendOwnedResourceUnion(intents []application.Manifest, files application.RuntimeFiles, instance *capability.ProviderInstance) ([]bhruntime.ProjectResource, error) {
	seen := map[bhruntime.ProjectResource]bool{}
	for _, intent := range intents {
		var resources []bhruntime.ProjectResource
		var err error
		if instance == nil {
			resources, err = application.NativeBackendResources(intent, files)
		} else {
			resources, err = application.NativeBackendInstanceResources(intent, files, *instance)
		}
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			seen[resource] = true
		}
	}
	resources := []bhruntime.ProjectResource{}
	for resource := range seen {
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Kind+resources[i].Name < resources[j].Kind+resources[j].Name })
	return resources, nil
}
