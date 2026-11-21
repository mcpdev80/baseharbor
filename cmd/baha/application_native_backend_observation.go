package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// The applied intent remains authoritative for removed services. During retry
// the existing deployment owner retains its original intent explicitly.
func retainedApplicationBackendIntent(resolved resolvedApplication) (application.Manifest, error) {
	return retainedApplicationBackendIntentWithLoader(resolved, deployment.LoadDeploymentRecord)
}

func retainedApplicationBackendIntentForPlan(resolved resolvedApplication) (application.Manifest, error) {
	return retainedApplicationBackendIntentWithLoader(resolved, deployment.LoadOptionalDeploymentRecord)
}

func retainedApplicationBackendIntentWithLoader(resolved resolvedApplication, load func(deployment.DeploymentIdentity) (deployment.DeploymentRecord, error)) (application.Manifest, error) {
	if resolved.DeploymentIdentity.DeploymentID == "" {
		return resolved.Manifest, nil
	}
	record, err := load(resolved.DeploymentIdentity)
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
			// A Target-registered application SQL or document-database instance
			// can be intentionally absent from the previous applied workload.
			// Derive only canonical inspection names from its retained binding;
			// the runtime inspector must still prove measured presence/absence.
			inspectionManifest, ok := nativeBackendBindingInspectionManifest(resolved.Manifest, instance, registry.Bindings)
			if !ok {
				continue
			}
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

func nativeBackendBindingInspectionManifest(manifest application.Manifest, instance capability.ProviderInstance, bindings []capability.ProviderBinding) (application.Manifest, bool) {
	if instance.Scope != capability.ScopeApplication || instance.Ownership != capability.OwnershipBaseHarbor ||
		instance.OwnerApplicationID != manifest.ApplicationID || instance.OwnerEnvironment != manifest.Environment {
		return application.Manifest{}, false
	}
	var kind capability.Kind
	var provider capability.ProviderKind
	var withNames func(application.Manifest, ...string) application.Manifest
	switch instance.Provider.Kind {
	case capability.ProviderPostgreSQL:
		kind, provider = capability.SQL, capability.ProviderPostgreSQL
		withNames = func(m application.Manifest, names ...string) application.Manifest {
			return application.WithSQLInstances(m, names...)
		}
	case capability.ProviderMongoDB:
		kind, provider = capability.DocumentDatabase, capability.ProviderMongoDB
		withNames = func(m application.Manifest, names ...string) application.Manifest {
			return application.WithDocumentDatabaseInstances(m, names...)
		}
	default:
		return application.Manifest{}, false
	}
	namesByResource := map[string]struct{}{}
	for _, binding := range bindings {
		if binding.ProviderInstanceID != instance.ID {
			continue
		}
		if binding.ApplicationID != manifest.ApplicationID || binding.Environment != manifest.Environment ||
			binding.Resource.Application != manifest.Name || binding.Resource.Kind != kind || binding.Resource.Provider != provider || binding.Resource.Name == "" {
			return application.Manifest{}, false
		}
		namesByResource[binding.Resource.Name] = struct{}{}
	}
	if len(namesByResource) == 0 {
		return application.Manifest{}, false
	}
	names := make([]string, 0, len(namesByResource))
	for name := range namesByResource {
		names = append(names, name)
	}
	sort.Strings(names)
	return withNames(manifest, names...), true
}
