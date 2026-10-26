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

func TestNamedApplicationProjectsMinimalRepositoryWorkloadWithoutWrites(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	repo := t.TempDir()
	authored := []byte("version: 1\napp:\n  name: named-workload\n  environment: prod\n")
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, authored, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	m := application.WithWorkloadComponents(application.New("named-workload", "prod", false, false, false), "api")
	id, err := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	store := application.Store{Root: filepath.Join(root, "state"), Namespace: target.Name}
	canonicalPath, err := store.Create(m)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{Identity: id, Source: deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: manifestPath}, Applied: deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider}}); err != nil {
		t.Fatal(err)
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{manifestPath, canonicalPath, filepath.Join(root, "deployment.json")}
	before := map[string][]byte{}
	for _, path := range paths {
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, operation := range []string{"plan", "apply", "status", "doctor"} {
		resolved, err := resolveRegisteredApplication(t.Context(), target, targetRoot, m.Name, m.Environment, operation)
		if err != nil {
			t.Fatal(operation, err)
		}
		if !resolved.FromRepository || !application.HasExplicitWorkload(resolved.Manifest) || resolved.Manifest.ApplicationID != m.ApplicationID || resolved.Manifest.Services.SQL {
			t.Fatalf("%s lost repository workload or canonical identity: %+v", operation, resolved)
		}
		for _, path := range paths {
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before[path], after) {
				t.Fatalf("%s resolution wrote owner or authored state at %s: %v", operation, path, err)
			}
		}
	}
}
