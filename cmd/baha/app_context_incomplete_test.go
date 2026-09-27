package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestResolveRegisteredApplicationRecoversIncompleteDeploymentFromProtectedState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	id := deployment.DeploymentIdentity{Target: target.Name, Application: "demo", Environment: "dev"}
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	store := application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name}
	m := application.New("demo", "dev", true, true, false)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveRegisteredApplication(target, targetRoot, "demo", "", "status")
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.IncompleteDeployment {
		t.Fatal("expected recovered deployment to remain marked incomplete")
	}
	if resolved.Manifest.Name != "demo" || resolved.Manifest.Environment != "dev" {
		t.Fatalf("unexpected recovered manifest: %#v", resolved.Manifest)
	}
	if resolved.FromRepository || resolved.SourceAvailable {
		t.Fatalf("incomplete state must not invent source availability: %#v", resolved)
	}
}

func TestResolveRegisteredApplicationFailsClosedWhenIncompleteStateCannotBeReconstructed(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	incomplete := filepath.Join(targetRoot, "deployments", "demo", "dev")
	if err := os.MkdirAll(incomplete, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err = resolveRegisteredApplication(target, targetRoot, "demo", "", "status")
	if err == nil {
		t.Fatal("expected incomplete deployment error")
	}
	var typed *machine.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected machine error, got %T: %v", err, err)
	}
	if typed.Code != machine.ErrorOwnershipAmbiguous || typed.CauseCode != "INCOMPLETE_DEPLOYMENT_STATE" {
		t.Fatalf("unexpected machine error: %#v", typed)
	}
	if typed.Resource != "local/demo/dev" {
		t.Fatalf("resource = %q", typed.Resource)
	}
}
