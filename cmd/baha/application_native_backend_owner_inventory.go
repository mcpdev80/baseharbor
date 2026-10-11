package main

import (
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
