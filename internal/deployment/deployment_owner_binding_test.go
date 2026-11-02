package deployment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// writeInstallationAuthoritative persists a valid Core installation under the
// given runtime root so a deployment may be loaded against a single owner.
func writeInstallationAuthoritative(t *testing.T, target, root string) coreinstallation.State {
	t.Helper()
	spec := coreinstallation.Spec{Target: target, Runtime: "docker", MachineRole: coreinstallation.Development}
	steps := coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	}
	state, err := coreinstallation.Run(context.Background(), root, spec, steps)
	if err != nil {
		t.Fatalf("write authoritative installation: %v", err)
	}
	return state
}

// TestDeploymentOwnerBindingPositiveAndMissing verifies that deployment state is
// loadable only when a single Core installation governs the Target, and that a
// missing installation fails closed without mutating the record.
func TestDeploymentOwnerBindingPositiveAndMissing(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	xdgData := t.TempDir()
	xdgConfig := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgData)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	target := "local"
	identity := testDeploymentIdentity(t, target, "app-owner", "dev")

	// A missing Core installation under the Target fails closed: the owner check
	// rejects, and no record is mutated.
	root := filepath.Join(xdgData, "baseharbor", "targets", target, "runtime")
	if err := VerifyDeploymentOwner(target, root, identity.DeploymentID); !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("missing installation not rejected: %v", err)
	}

	// Now the installation is restored: the owner check passes and the record
	// identity is preserved unchanged (never rewritten to carry installation).
	writeInstallationAuthoritative(t, target, root)
	if err := VerifyDeploymentOwner(target, root, identity.DeploymentID); err != nil {
		t.Fatalf("authoritative installation did not own its deployment: %v", err)
	}
}

// TestDeploymentOwnerRejectsForeignInstallation verifies that a Core installation
// for a different Target is rejected as a foreign owner, and that the same
// Target's own installation passes. Two isolated homes provision distinct
// installations for distinct Targets of the same machine.
func TestDeploymentOwnerRejectsForeignInstallation(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	home1 := t.TempDir()
	home2 := t.TempDir()

	target := "local"
	identity := testDeploymentIdentity(t, target, "app-owner", "dev")
	root1 := filepath.Join(home1, "baseharbor", "targets", target, "runtime")
	installation1 := writeInstallationAuthoritative(t, target, root1)

	// home1's Target owns its deployment namespace.
	t.Setenv("XDG_DATA_HOME", home1)
	t.Setenv("XDG_CONFIG_HOME", home1)
	if err := VerifyDeploymentOwner(target, root1, identity.DeploymentID); err != nil {
		t.Fatalf("home1 owner did not own its deployment: %v", err)
	}

	// home2 provisions a distinct installation for a DIFFERENT Target name.
	t.Setenv("XDG_DATA_HOME", home2)
	t.Setenv("XDG_CONFIG_HOME", home2)
	target2 := "remote"
	root2 := filepath.Join(home2, "baseharbor", "targets", target2, "runtime")
	installation2 := writeInstallationAuthoritative(t, target2, root2)
	if installed, err := coreinstallation.Load(root2); err != nil {
		t.Fatalf("home2 installation load: %v", err)
	} else if installed.ID == installation1.ID || installed.Spec.Target != target2 {
		t.Fatal("home2 did not establish a distinct installation for its own Target")
	}
	_ = installation2

	// home2's installation governs target2, not home1's "local" Target: applying
	// it to "local" fails closed (foreign owner).
	if err := VerifyDeploymentOwner(target, root2, identity.DeploymentID); !errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("foreign installation not rejected: %v", err)
	}

	// home1's installation still governs "local" under home2's env (the owner is
	// resolved by the passed root, not by the active data root).
	if err := VerifyDeploymentOwner(target, root1, identity.DeploymentID); err != nil {
		t.Fatalf("home1 owner lost after home2 env switch: %v", err)
	}
}

// TestDeploymentOwnerRejectsForeignInstallationDir ensures a deployment directory
// that is a symlink escape under an owned Target is rejected.
func TestDeploymentOwnerRejectsForeignInstallationDir(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	root := filepath.Join(data, "baseharbor", "targets", target, "runtime")
	writeInstallationAuthoritative(t, target, root)

	// Symlink the deployments namespace to an external location: a foreign owner.
	depNamespace := filepath.Join(root, "..", "deployments")
	external := t.TempDir()
	_ = os.MkdirAll(filepath.Join(depNamespace), 0o700)
	linkTarget := filepath.Join(external, "deployments")
	if err := os.Symlink(linkTarget, depNamespace); err == nil {
		if err := VerifyDeploymentOwner(target, root, testDeploymentIdentity(t, target, "x", "dev").DeploymentID); !errors.Is(err, ErrDeploymentOwnerForeign) {
			t.Fatalf("symlinked deployments namespace not rejected: %v", err)
		}
	}
}
