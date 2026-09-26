package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestResolveRegisteredApplicationReportsIncompleteDeploymentState(t *testing.T) {
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
