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

// TestResolveRegisteredApplicationPropagatesOwnerStateNotNotFound guards the
// CLI finding that a non-missing owner error (an unreadable installation, or a
// present-but-malformed Core state) must reach the CLI as its true
// access-denied / malformed-state verdict instead of being folded into the
// generic "no registered deployment" lie. This regression corrupts the Core
// installation.json with invalid content so coreinstallation.Load fails on
// content validation; the named CLI entry (a bare `baha app status NAME` with
// no repository context) must surface ErrDeploymentOwnerMalformedState, never
// "not found".
func TestResolveRegisteredApplicationPropagatesOwnerStateNotNotFound(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	if err := registerInstallationForTarget(t, target.Name); err != nil {
		t.Fatal(err)
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	m := application.New("demo", "dev", true, true, false)
	id, err := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		t.Fatal(err)
	}
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	store := application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name}
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}

	// Corrupt the Core installation.json (present but invalid) so the owner
	// seam cannot validate it: a malformed-state signal, not a missing owner.
	installRoot, ierr := installationRuntimeRoot(t, target.Name)
	if ierr != nil {
		t.Fatal(ierr)
	}
	installPath := filepath.Join(installRoot, "installation.json")
	if err := os.WriteFile(installPath, []byte("{ not valid json"), 0o600); err != nil {
		t.Fatalf("corrupt installation.json: %v", err)
	}

	// Bare named entry (no repository context) must propagate the true owner
	// state, not a "not found" lie.
	_, err = resolveRegisteredApplication(t.Context(), target, targetRoot, "demo", "", "status", "")
	if err == nil {
		t.Fatal("expected an owner-state error for a malformed Core installation")
	}
	if errors.Is(err, deployment.ErrDeploymentOwnerForeign) {
		t.Fatalf("malformed owner must NOT be reported as foreign at the CLI: %#v", err)
	}
	var typed *machine.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected a machine.Error for the propagated owner state, got: %#v", err)
	}
	if typed.CauseCode != "deployment_owner_malformed_state" && typed.CauseCode != "deployment_owner_access_denied" {
		t.Fatalf("CLI must propagate a distinct owner-state cause code, got cause=%q: %v", typed.CauseCode, err)
	}
}
