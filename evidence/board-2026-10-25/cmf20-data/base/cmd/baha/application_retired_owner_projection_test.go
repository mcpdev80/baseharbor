package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRuntimeRetiredOwnerProjectionKeepsCanonicalEmptyIntentAndArchivedResources(t *testing.T) {
	target := configureTestTarget(t)
	prior := application.WithDocumentDatabaseInstances(application.WithWorkloadComponents(application.New("retired", "dev", false, false, false), "api"), "records")
	retired := prior
	retired.Services = application.Services{}
	retired.Workload = application.WorkloadConfig{}
	if retired.ValidateIntent() != nil || retired.Validate() == nil {
		t.Fatal("fixture does not represent non-executable retired intent")
	}
	record := registerTestDeployment(t, target, retired, t.TempDir(), "baseharbor.yaml")
	encoded, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	record.Applied.GeneratedState = map[string]string{"retired_intent": string(encoded)}
	record.Observed.State = "runtime-retired"
	record.Observed.Ready = false
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
	resolved := resolvedApplication{Target: target, Manifest: retired, Store: application.Store{Root: filepath.Join(root, "state"), Namespace: target.Name}, DeploymentIdentity: record.Identity}
	intents, err := nativeBackendOwnerIntents(resolved)
	if err != nil || len(intents) != 2 {
		t.Fatal("retired owner projection lost canonical withdrawal or archived intent", err)
	}
	files := application.RuntimeFilesFor(resolved.Store, prior)
	resources, err := nativeBackendOwnedResourceUnion(intents, files, nil)
	expected, expectedErr := application.NativeBackendResources(prior, files)
	if err != nil || expectedErr != nil || len(resources) != len(expected) || len(resources) == 0 {
		t.Fatal("retired owner lost actual archived renderer inventory", err)
	}
	for _, resource := range expected {
		if !application.ResourceNamedExists(resources, resource) {
			t.Fatal("retired union invented or dropped canonical resource")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("owner projection changed retained recovery record")
	}
}
