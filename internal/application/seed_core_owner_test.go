package application

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// seedManagedTargetOwner provisions a genuine authoritative Core installation
// for the test target so the deployment-owner seam (verifyDeploymentOwnerAtSeam
// -> coreinstallation.Load(runtime)) has a single real owner. It is the
// internal/application fixture equivalent of cmd/baha's
// registerInstallationForTarget (w21 precedent) and of the
// internal/deployment seedAuthoritativeCoreOwnerForTarget (r3 precedent): real
// owner-material in the target-state-root runtime, no global bypass, no
// off-switch, no skip.
//
// Callers invoke it at the top of a managed-target test body, AFTER
// t.Setenv for their data/config roots so the installation lands in that test's
// temp home, and BEFORE any SaveDeploymentRecord against the same target.
func seedManagedTargetOwner(t *testing.T, targetName string) {
	t.Helper()
	targetRoot, err := deployment.TargetStateRoot(targetName)
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
