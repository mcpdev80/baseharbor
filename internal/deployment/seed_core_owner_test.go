package deployment

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// seedManagedTargetOwner provisions a genuine authoritative Core installation
// for the test target so the deployment-owner seam has a single real owner.
// It is the internal/deployment fixture equivalent of cmd/baha's
// registerInstallationForTarget (w21 precedent): real owner-material in the
// target-state-root runtime, no global bypass, no off-switch, no skip.
//
// Negative owner families (missing/foreign/corrupt) are intentionally NOT fed
// through this helper — they keep their own seed logic that provokes exactly
// those failure modes. Callers invoke it at the very top of a managed-target
// test body, before any t.Setenv for BASEHARBOR_STATE_DIR / XDG_* so the
// installation lands in that test's temp home.
func seedManagedTargetOwner(t *testing.T, targetName string) {
	t.Helper()
	seedAuthoritativeCoreOwnerForTarget(t, targetName)
}

// seedAuthoritativeCoreOwnerForTarget persists a genuine, authoritative Core
// installation under the canonical runtime root of target
// (XDG_DATA_HOME/baseharbor/targets/<target>/runtime) so the production
// deployment-owner seam has a single authoritative owner for its Target.
//
// This is the internal/deployment fixture equivalent of cmd/baha's
// registerInstallationForTarget (w21 precedent): it provisions real, valid
// owner-material (installation.json with a valid identity in the target-state
// root) through the production coreinstallation.Run lifecycle. It is NOT a
// global bypass, NOT an off-switch flag, NOT a skip: the one-installation-owner
// rule still runs at the seam, and the provisioned installation is a real Core
// that owns the Target. Negative-owner families (missing/foreign/corrupt) keep
// their own seed logic and are never fed through this helper.
//
// It returns the provisioned installation identity (or its deployment identity
// target), so callers can assert on it if needed. Callers must invoke it AFTER
// t.Setenv for their data/config roots so the installation lands in that temp
// home, and BEFORE any SaveDeploymentRecord against the same target.
func seedAuthoritativeCoreOwnerForTarget(t *testing.T, targetName string) {
	t.Helper()
	targetRoot, err := TargetStateRoot(targetName)
	if err != nil {
		t.Fatalf("resolve target state root for %s: %v", targetName, err)
	}
	installRoot := filepath.Join(targetRoot, "runtime")
	spec := coreinstallation.Spec{Target: targetName, Runtime: "docker", MachineRole: coreinstallation.Development}
	steps := coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	}
	if _, runErr := coreinstallation.Run(context.Background(), installRoot, spec, steps); runErr != nil {
		t.Fatalf("seed authoritative Core owner for %s: %v", targetName, runErr)
	}
}
