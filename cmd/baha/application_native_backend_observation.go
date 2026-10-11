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
	if len(intent) == 0 || json.Unmarshal(intent, &m) != nil || m.ApplicationID != resolved.Manifest.ApplicationID || m.Name != resolved.Manifest.Name || m.Environment != resolved.Manifest.Environment {
		return application.Manifest{}, invalidProviderConsumerInventory(fmt.Errorf("native backend intent differs from deployment owner"))
	}
	return m, m.Validate()
}

func nativeBackendInventory(resolved resolvedApplication) (application.RuntimeFiles, application.Manifest, []bhruntime.ProjectResource, error) {
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return application.RuntimeFiles{}, prior, nil, err
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	if err := application.CheckExistingNativeBackendInventory(files, prior, resolved.Manifest); err != nil {
		return files, prior, nil, err
	}
	resources, err := application.NativeBackendResources(prior, files)
	return files, prior, resources, err
}

func observeNativeBackendProviderFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	files, prior, expected, err := nativeBackendInventory(resolved)
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
	for _, instance := range registry.Instances {
		resources, err := application.NativeBackendInstanceResources(prior, files, instance)
		if err != nil {
			continue
		}
		count := int64(0)
		state := application.ProviderStoppedOwned
		for _, container := range containers {
			for _, resource := range resources {
				if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, files.Project, resource) {
					count++
					if container.Running {
						state = application.ProviderRunning
					}
					break
				}
			}
		}
		facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreNotRequired, Dependencies: []string{}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "protected canonical applied backend intent and native ownership inventory"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
	}
}
