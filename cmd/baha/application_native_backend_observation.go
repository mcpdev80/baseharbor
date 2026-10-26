package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// The applied intent remains authoritative for removed services. During retry
// the existing deployment owner retains its original intent explicitly.
func retainedApplicationBackendIntent(resolved resolvedApplication) (application.Manifest, error) {
	if resolved.DeploymentIdentity.DeploymentID == "" {
		return resolved.Manifest, nil
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if errors.Is(err, os.ErrNotExist) {
		return resolved.Manifest, nil
	}
	if err != nil {
		return application.Manifest{}, err
	}
	intent := record.Applied.Intent
	if previous := record.Applied.GeneratedState["previous_intent"]; previous != "" {
		intent = json.RawMessage(previous)
	}
	var m application.Manifest
	if len(intent) == 0 || json.Unmarshal(intent, &m) != nil || m.ApplicationID != resolved.Manifest.ApplicationID || m.Environment != resolved.Manifest.Environment {
		return application.Manifest{}, invalidProviderConsumerInventory(fmt.Errorf("native backend intent differs from deployment owner"))
	}
	if err := m.ValidateIntent(); err != nil {
		return application.Manifest{}, err
	}
	key := "backend_placements"
	if record.Applied.GeneratedState["previous_intent"] != "" {
		key = "previous_backend_placements"
	}
	return retainedBackendPlacementProjection(resolved, m, record.Applied.GeneratedState[key])
}

func nativeBackendInventory(resolved resolvedApplication) (application.RuntimeFiles, application.Manifest, []bhruntime.ProjectResource, error) {
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return application.RuntimeFiles{}, prior, nil, err
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	intents, err := nativeBackendOwnerIntents(resolved)
	if err != nil {
		return files, prior, nil, err
	}
	if err := application.CheckExistingNativeBackendInventory(files, intents...); err != nil {
		return files, prior, nil, err
	}
	resources, err := nativeBackendOwnedResourceUnion(intents, files, nil)
	return files, prior, resources, err
}

func observeNativeBackendProviderFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	files, _, expected, err := nativeBackendInventory(resolved)
	if err != nil {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	if _, err := provider.InspectProjectResources(ctx, files.Project, expected); err != nil {
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
	intents, err := nativeBackendOwnerIntents(resolved)
	if err != nil {
		return
	}
	for _, instance := range registry.Instances {
		resources, err := nativeBackendOwnedResourceUnion(intents, files, &instance)
		if err != nil || len(resources) == 0 {
			// A Target-registered application SQL instance can be intentionally
			// absent from the previous applied workload. Derive only its
			// canonical inspection names from the retained Target binding;
			// the actual runtime inspector must still prove measured absence.
			if instance.Provider.Kind != capability.ProviderPostgreSQL ||
				instance.Scope != capability.ScopeApplication ||
				instance.Ownership != capability.OwnershipBaseHarbor ||
				instance.OwnerApplicationID != resolved.Manifest.ApplicationID ||
				instance.OwnerEnvironment != resolved.Manifest.Environment {
				continue
			}
			name := ""
			for _, binding := range registry.Bindings {
				if binding.ProviderInstanceID == instance.ID &&
					binding.ApplicationID == resolved.Manifest.ApplicationID &&
					binding.Environment == resolved.Manifest.Environment &&
					binding.Resource.Kind == capability.SQL {
					name = binding.Resource.Name
				}
			}
			if name == "" {
				continue
			}
			inspectionManifest := application.WithSQLInstances(resolved.Manifest, name)
			resources, err = application.NativeBackendInstanceResources(inspectionManifest, files, instance)
			if err != nil || len(resources) == 0 {
				continue
			}
			if _, err := provider.InspectProjectResources(ctx, files.Project, resources); err != nil {
				continue
			}
		}
		count, state := observeCanonicalProviderContainers(provider.Kind(), files.Project, resources, containers)
		facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreNotRequired, Dependencies: []string{}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "protected canonical applied backend intent and native ownership inventory"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
	}
}
