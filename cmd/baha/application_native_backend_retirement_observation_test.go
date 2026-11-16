package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestNativeBackendRetirementRetainsMeasuredCanonicalAbsence(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	prior := resolved.Manifest
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: resolved.Target.Name, PlanID: "native-retirement", Complete: false, Impacts: []application.ProviderImpact{}}
	for pass := range 2 {
		if _, err := recordPendingApplicationRuntimeRetirement(resolved, plan); err != nil {
			t.Fatal(err)
		}
		withdrawn := resolved
		withdrawn.Manifest = applicationRuntimeRetirementIntent(resolved.Manifest)
		if err := application.ReleaseApplicationProviderReferencesAt(resolved.TargetStateRoot, prior); err != nil {
			t.Fatal(err)
		}
		files, _, all, err := nativeBackendInventory(withdrawn)
		if err != nil {
			t.Fatalf("pass %d canonical inventory: %v", pass, err)
		}
		if len(all) == 0 {
			t.Fatalf("pass %d lost original native resources", pass)
		}
		intents, err := nativeBackendOwnerIntents(withdrawn)
		if err != nil {
			t.Fatal(err)
		}
		registry, err := (capability.RegistryStore{Path: resolved.TargetStateRoot + "/provider-registry.json"}).Load()
		if err != nil {
			t.Fatal(err)
		}
		for _, instance := range registry.Instances {
			if instance.Provider.Kind != capability.ProviderMongoDB {
				continue
			}
			selected, err := nativeBackendOwnedResourceUnion(intents, files, &instance)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(files.Compose)
			if err != nil {
				t.Fatal(err)
			}
			n, state := observeCanonicalProviderContainers("docker", files.Project, selected, nil)
			if n != 0 || state != application.ProviderStoppedOwned {
				t.Fatalf("pass %d stopped native backend is unverifiable: %d %s, %v", pass, n, state, selected)
			}
			zero := int64(0)
			facts := map[string]application.ProviderFact{instance.ID: {Target: resolved.Target.Name, State: state, Containers: application.Quantity{Value: &zero, Classification: "measured"}}}
			verification := application.ProviderPlan{Changes: []application.ProviderChange{{InstanceID: instance.ID, Action: "stop"}}}
			if err := verifyApplicationRuntimeRetirement(resolved, verification, facts); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(files.Compose)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("observation changed canonical bytes")
			}
		}
		if err := recordVerifiedApplicationRuntimeRetirement(resolved, plan, nil, nil); err != nil {
			t.Fatal(err)
		}
		if pass == 0 {
			resolved.Manifest.Services.DocumentDatabaseInstances = nil
			resolved.Manifest.Services.DocumentDatabase = false
			canonical, err := application.RuntimeComposeYAMLForProject(resolved.Manifest, files.ResourceProject)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
				t.Fatal(err)
			}
			if err := recordAppliedDeployment(context.Background(), resolved, files); err != nil {
				t.Fatal(err)
			}
			if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, prior); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := recordPendingApplicationRuntimeRetirement(resolved, plan); err != nil {
		t.Fatal(err)
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	history, err := retiredRuntimeHistory(record)
	if err != nil || len(history) != 1 {
		t.Fatalf("repeat duplicated or lost original retired owner: %d %v", len(history), err)
	}
}

func TestRetiredRuntimeHistoryRejectsForeignAndInvalidOwners(t *testing.T) {
	for _, kind := range []string{"malformed", "foreign", "environment", "placements", "unbounded"} {
		t.Run(kind, func(t *testing.T) {
			resolved := retainedNativeBackendFixture(t)
			record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
			if err != nil {
				t.Fatal(err)
			}
			intent := resolved.Manifest
			if kind == "foreign" {
				intent.ApplicationID = "21e1f284-93d6-4923-a159-80a4d7103dd9"
			}
			if kind == "environment" {
				intent.Environment = "production"
			}
			raw, err := json.Marshal(intent)
			if err != nil {
				t.Fatal(err)
			}
			owner := retiredRuntimeOwner{Intent: raw}
			if kind == "placements" {
				owner.BackendPlacements = `{"postgresql":{"scope":"external","ownership":"baseharbor"}}`
			}
			history := []retiredRuntimeOwner{owner}
			if kind == "unbounded" {
				for len(history) < 65 {
					history = append(history, owner)
				}
			}
			encoded, _ := json.Marshal(history)
			if kind == "malformed" {
				encoded = []byte(`{invalid`)
			}
			record.Applied.GeneratedState["retired_runtime_history"] = string(encoded)
			if err := deployment.SaveDeploymentRecord(record); err != nil {
				t.Fatal(err)
			}
			root, err := deployment.DeploymentRoot(record.Identity)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "deployment.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := applicationRuntimeOwners(resolved); err == nil {
				t.Fatal("untrusted archived runtime owner accepted")
			}
			if _, err := applicationBrokerOwners(resolved); err == nil {
				t.Fatal("untrusted broker owner accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected read changed owner")
			}
		})
	}
}
