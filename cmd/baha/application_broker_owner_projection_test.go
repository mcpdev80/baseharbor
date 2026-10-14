package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestBrokerUnionUsesEachExistingDeploymentOwnerForSiblingAndPendingIntent(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	first := application.WithWorkloadComponents(application.New("first", "dev", false, false, true), "api")
	second := application.WithWorkloadComponents(application.New("second", "dev", false, false, true), "api")
	expected := map[string]application.Store{}
	var current resolvedApplication
	for index, manifest := range []application.Manifest{first, second} {
		id, err := deployment.NewDeploymentIdentity(target.Name, manifest.ApplicationID, manifest.Name, manifest.Environment)
		if err != nil {
			t.Fatal(err)
		}
		root, err := deployment.DeploymentRoot(id)
		if err != nil {
			t.Fatal(err)
		}
		store := application.Store{Root: filepath.Join(root, "state"), Namespace: target.Name}
		intent, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		record := deployment.DeploymentRecord{Identity: id, Applied: deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider}}
		if index == 1 {
			// An interrupted sibling removal still owns its prior broker.
			desired := manifest
			desired.Services.Secrets = false
			record.Applied.Intent, err = json.Marshal(desired)
			if err != nil {
				t.Fatal(err)
			}
			record.Applied.GeneratedState = map[string]string{"previous_intent": string(intent)}
		}
		if err := deployment.SaveDeploymentRecord(record); err != nil {
			t.Fatal(err)
		}
		expected[application.RuntimeBrokerDependency(target.Name, manifest).ID] = store
		if index == 0 {
			current = resolvedApplication{Target: target, Manifest: manifest, Store: store}
			current.Manifest.Services.Secrets = false
		}
	}
	owners, err := applicationBrokerOwners(current)
	if err != nil || len(owners) != 2 {
		t.Fatal("existing and interrupted sibling owners were lost", err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register(capability.ProviderInstance{ID: "openbao/control-plane", Provider: capability.OpenBao, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}); err != nil {
		t.Fatal(err)
	}
	snapshot := application.ResolutionSnapshot{Target: target.Name, Registry: registry, Facts: map[string]application.ProviderFact{}}
	if err := appendApplicationBrokerDependencies(current, nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, owner := range owners {
		instance := application.RuntimeBrokerDependency(target.Name, owner.manifest)
		store, found := expected[instance.ID]
		if !found || !owner.existing || owner.store != store {
			t.Fatal("broker borrowed another deployment's Store")
		}
		files := application.RuntimeFilesFor(store, owner.manifest)
		if snapshot.Facts[instance.ID].OwnerReference != files.Dir {
			t.Fatal("broker dependency projects a sibling's credentials/configuration path")
		}
	}
	// Raw native broker observations contain process ownership only. Their
	// null dependency diagnostic must be enriched from each retained owner,
	// including a sibling's interrupted capability removal.
	observations := map[string]application.ProviderFact{}
	for id, fact := range snapshot.Facts {
		fact.State, fact.Dependencies = application.ProviderRunning, nil
		observations[id] = fact
	}
	if err := appendApplicationBrokerDependencies(current, observations, &snapshot); err != nil {
		t.Fatal(err)
	}
	for id := range expected {
		fact := snapshot.Facts[id]
		if len(fact.Dependencies) != 1 || fact.Dependencies[0] != "openbao/control-plane" || fact.State != application.ProviderRunning || fact.OwnerReference != observations[id].OwnerReference {
			t.Fatal("raw broker observation lost its retained owner dependency or authority")
		}
	}
}
