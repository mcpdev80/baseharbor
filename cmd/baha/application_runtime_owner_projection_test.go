package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRuntimeOwnerProjectionPreservesSiblingStoreAndInterruptedIntents(t *testing.T) {
	target := configureTestTarget(t)
	prior := application.New("sibling", "dev", false, false, true)
	prior.Services.MessagingQueue = true
	current := prior
	current.Name = "sibling-renamed"
	current.Services.MessagingQueue = false
	current.Services.DocumentDatabase = true
	record := registerTestDeployment(t, target, current, t.TempDir(), "baseharbor.yaml")
	encoded, _ := json.Marshal(prior)
	record.Applied.GeneratedState = map[string]string{"previous_intent": string(encoded)}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DeploymentRoot(record.Identity)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "deployment.json"))
	if err != nil {
		t.Fatal(err)
	}
	other := resolvedApplication{Target: target, Manifest: application.New("current", "dev", false, false, false), Store: application.Store{Root: t.TempDir()}}
	owners, err := applicationRuntimeOwners(other)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, owner := range owners {
		if owner.Manifest.ApplicationID != current.ApplicationID {
			continue
		}
		if owner.Store.Root != filepath.Join(root, "state") || owner.DeploymentIdentity != record.Identity {
			t.Fatal("sibling observed through another application's Store")
		}
		seen[owner.Manifest.Name] = true
	}
	if len(seen) != 2 {
		t.Fatalf("interruption lost an authoritative intent: %v", seen)
	}
	sibling := resolvedApplication{Target: target, Manifest: current, Store: application.Store{Root: filepath.Join(root, "state")}, DeploymentIdentity: record.Identity}
	intents, err := nativeBackendOwnerIntents(sibling)
	if err != nil {
		t.Fatal(err)
	}
	files := application.RuntimeFilesFor(sibling.Store, current)
	actual, err := nativeBackendOwnedResourceUnion(intents, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	previous, _ := application.NativeBackendResources(prior, files)
	desired, _ := application.NativeBackendResources(current, files)
	if len(actual) <= len(previous) || len(actual) <= len(desired) {
		t.Fatal("native ownership omitted applied or previous backend")
	}
	brokers, err := applicationBrokerOwners(other)
	if err != nil || len(brokers) != 2 || brokers[0].store.Root != filepath.Join(root, "state") || brokers[1].store.Root != filepath.Join(root, "state") {
		t.Fatalf("retained broker owner lost: %+v %v", brokers, err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "deployment.json"))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read-only inventory changed recovery state")
	}
}

func TestRuntimeOwnerProjectionRejectsForeignRetainedOwnerWithoutMutation(t *testing.T) {
	target := configureTestTarget(t)
	m := application.New("sibling", "dev", false, false, false)
	record := registerTestDeployment(t, target, m, t.TempDir(), "baseharbor.yaml")
	foreign := m
	foreign.ApplicationID = application.MustNewApplicationID()
	encoded, _ := json.Marshal(foreign)
	record.Applied.GeneratedState = map[string]string{"retired_intent": string(encoded)}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	root, _ := deployment.DeploymentRoot(record.Identity)
	before, _ := os.ReadFile(filepath.Join(root, "deployment.json"))
	if _, err := applicationRuntimeOwners(resolvedApplication{Target: target, Manifest: m}); err == nil {
		t.Fatal("foreign recovery owner accepted")
	}
	after, _ := os.ReadFile(filepath.Join(root, "deployment.json"))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("denial changed authoritative owner")
	}
}
