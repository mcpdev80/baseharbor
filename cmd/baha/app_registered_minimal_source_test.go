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

func TestNamedMinimalSourceUsesSameWorkloadAndIdentityWithoutWrites(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	t.Chdir(selection.RepositoryRoot)
	initialized, err := resolveRepositoryLifecycleIntent(t.Context(), target, selection, "init")
	if err != nil {
		t.Fatal(err)
	}
	id, err := deployment.NewDeploymentIdentity(target.Name, initialized.Manifest.ApplicationID, initialized.Manifest.Name, initialized.Manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(initialized.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{Identity: id, Source: deployment.DeploymentSource{Kind: "repository", Repository: selection.RepositoryRoot, Manifest: selection.ManifestPath}, Applied: deployment.AppliedDeployment{Intent: intent, RuntimeProvider: "docker"}}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	owner := application.Store{Root: filepath.Join(selection.RepositoryRoot, ".baseharbor", "apps")}
	_, canonicalPath, err := owner.Load(initialized.Manifest.Name)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{selection.ManifestPath, canonicalPath, filepath.Join(deploymentRoot, "deployment.json")} {
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range []string{"plan", "apply", "status", "doctor", "stop", "destroy"} {
		repository, err := resolveApplication(t.Context(), application.Store{}, nil, command)
		if err != nil {
			t.Fatal(err)
		}
		named, err := resolveApplication(t.Context(), application.Store{}, []string{initialized.Manifest.Name}, command)
		if err != nil {
			t.Fatal(err)
		}
		if !named.FromRepository || !named.SourceAvailable || !application.HasExplicitWorkload(named.Manifest) || !reflect.DeepEqual(named.Manifest, repository.Manifest) || named.DeploymentIdentity != repository.DeploymentIdentity {
			t.Fatalf("%s: named and repository-local minimal workload resolution differ", command)
		}
	}
	for path, content := range before {
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(content) {
			t.Fatal("source resolution rewrote authored, identity or deployment state")
		}
	}
	// Another protected deployment claims a different identity for this source.
	// Named resolution must reject it just like repository-local resolution.
	foreignID, err := deployment.NewDeploymentIdentity(target.Name, application.MustNewApplicationID(), "other-owner", "dev")
	if err != nil {
		t.Fatal(err)
	}
	record.Identity = foreignID
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveApplication(t.Context(), application.Store{}, []string{initialized.Manifest.Name}, "plan"); err == nil {
		t.Fatal("named source resolution ignored conflicting protected source ownership")
	}
}
