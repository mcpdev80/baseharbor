package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

type applicationBrokerOwner struct {
	manifest application.Manifest
	existing bool
	store    application.Store
}

func applicationBrokerOwners(resolved resolvedApplication) ([]applicationBrokerOwner, error) {
	owners := map[string]applicationBrokerOwner{}
	add := func(m application.Manifest, existing bool, store application.Store) {
		if !application.RequiresRuntimeBroker(m) {
			return
		}
		id := application.RuntimeBrokerDependency(resolved.Target.Name, m).ID
		prior := owners[id]
		if prior.existing && !existing {
			store = prior.store
		}
		owners[id] = applicationBrokerOwner{manifest: m, existing: existing || prior.existing, store: store}
	}
	add(resolved.Manifest, false, resolved.Store)
	records, err := deployment.ListDeployments(resolved.Target.Name)
	if err != nil {
		return nil, invalidProviderConsumerInventory(err)
	}
	for _, record := range records {
		root, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return nil, invalidProviderConsumerInventory(err)
		}
		store := application.Store{Root: filepath.Join(root, "state"), Namespace: resolved.Target.Name}
		intents := []json.RawMessage{record.Applied.Intent}
		if prior := record.Applied.GeneratedState["previous_intent"]; prior != "" {
			intents = append(intents, json.RawMessage(prior))
		}
		if retained := record.Applied.GeneratedState["retired_intent"]; retained != "" {
			intents = append(intents, json.RawMessage(retained))
		}
		for index, intent := range intents {
			var m application.Manifest
			id := record.Identity
			if json.Unmarshal(intent, &m) != nil || m.ApplicationID != id.ApplicationID || (index == 0 && m.Name != id.Application) || m.Environment != id.Environment {
				return nil, invalidProviderConsumerInventory(fmt.Errorf("broker intent differs from deployment owner"))
			}
			if err := m.Validate(); err != nil {
				return nil, invalidProviderConsumerInventory(err)
			}
			add(m, true, store)
		}
	}
	keys := []string{}
	for key := range owners {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []applicationBrokerOwner{}
	for _, key := range keys {
		result = append(result, owners[key])
	}
	return result, nil
}

func appendApplicationBrokerDependencies(resolved resolvedApplication, observations map[string]application.ProviderFact, snapshot *application.ResolutionSnapshot) error {
	owners, err := applicationBrokerOwners(resolved)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		instance := application.RuntimeBrokerDependency(resolved.Target.Name, owner.manifest)
		files := application.RuntimeFilesFor(owner.store, owner.manifest)
		roots, err := application.RuntimeDependencyRoots(owner.manifest, snapshot.Registry)
		if err != nil {
			return err
		}
		state := application.ProviderNewRequired
		ownerReference := owner.manifest.ApplicationID
		if owner.existing {
			state = application.ProviderUnverifiable
			ownerReference = files.Dir
		}
		// Before first apply, the deployment UUID is only a fresh proposal on
		// each read. Bind consent to the canonical application owner instead of
		// claiming that proposal's runtime path as already allocated authority.
		// Existing deployments retain their actual protected runtime location.
		fact := application.ProviderFact{Target: resolved.Target.Name, Ancillary: true, OwnerReference: ownerReference, State: state, Core: application.CoreRequired, Dependencies: roots, MemoryBytes: application.Quantity{Classification: "unknown"}, Containers: application.Quantity{Classification: "unknown"}}
		if observed, ok := observations[instance.ID]; ok {
			fact = observed
			fact.Dependencies = roots
		}
		snapshot.Ancillary = append(snapshot.Ancillary, instance)
		snapshot.Facts[instance.ID] = fact
	}
	if application.RequiresRuntimeBroker(resolved.Manifest) {
		snapshot.DependencyRoots = append(snapshot.DependencyRoots, application.RuntimeBrokerDependency(resolved.Target.Name, resolved.Manifest).ID)
	}
	return nil
}

func observeApplicationBrokerFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	owners, err := applicationBrokerOwners(resolved)
	if err != nil || len(owners) == 0 {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return
	}
	for _, owner := range owners {
		if !owner.existing {
			continue
		}
		files := application.RuntimeFilesFor(owner.store, owner.manifest)
		protected := true
		for _, path := range []string{files.Dir, filepath.Join(files.Dir, "broker.compose.yaml")} {
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || (path == files.Dir && !info.IsDir()) || (path != files.Dir && !info.Mode().IsRegular()) {
				protected = false
				break
			}
		}
		if !protected {
			continue
		}
		project := runtimebroker.ProjectNameForRuntime(owner.manifest, files)
		resource := bhruntime.ProjectResource{Kind: "container", Name: project + "-broker-1"}
		if _, err := provider.InspectProjectResources(ctx, project, []bhruntime.ProjectResource{resource}); err != nil {
			continue
		}
		state := application.ProviderStoppedOwned
		count := int64(0)
		for _, container := range containers {
			if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, project, resource) {
				count++
				if container.Running {
					state = application.ProviderRunning
				}
			}
		}
		instance := application.RuntimeBrokerDependency(resolved.Target.Name, owner.manifest)
		facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, Ancillary: true, OwnerReference: files.Dir, State: state, Core: application.CoreRequired, MemoryBytes: application.Quantity{Classification: "unknown"}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "declared broker in protected deployment owner and inspected native container"}}
	}
}
