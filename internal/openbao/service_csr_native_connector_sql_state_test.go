package openbao

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func TestNativeSQLRegistryKeepsRuntimeStorageEnvironment(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "native-runtime-storage")
	t.Setenv("XDG_DATA_HOME", storage)
	project := targetsession.ProjectRecord{Version: 1,
		Scope:    targetenrollment.Scope{TenantID: "00000000-0000-0000-0000-000000000001", TargetID: "node-target", NodeID: "node-a", Runtime: "podman"},
		BundleID: "owned-sql", Directory: "bundles/.object-" + strings.Repeat("a", 32),
		Files: []targetsession.ProjectFileRecord{{Path: "runtime.env", SHA256: strings.Repeat("b", 64), Mode: 0600}},
	}
	restored := persistNativeSQLProject(t, t.TempDir(), application.New("demo", "dev", true, false, false), project)
	if os.Getenv("XDG_DATA_HOME") != storage || !reflect.DeepEqual(restored, project) {
		t.Fatal("registry restoration changed native runtime storage selection")
	}
}

// Persist through the production protected registry rather than retaining an
// in-memory receipt. This is provider qualification, not an Application apply.
func persistNativeSQLProject(t *testing.T, root string, manifest application.Manifest, project targetsession.ProjectRecord) targetsession.ProjectRecord {
	t.Helper()
	var restored deployment.DeploymentRecord
	if !t.Run("protected-sql-registry", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", filepath.Join(root, "sql-deployment-registry"))
		restored = roundTripNativeSQLProject(t, manifest, project)
	}) {
		t.Fatal("native SQL registry round-trip failed")
	}
	return *restored.Applied.RemoteProject
}

func roundTripNativeSQLProject(t *testing.T, manifest application.Manifest, project targetsession.ProjectRecord) deployment.DeploymentRecord {
	t.Helper()
	identity, err := deployment.NewDeploymentIdentity(project.Scope.TargetID, manifest.ApplicationID, manifest.Name, manifest.Environment)
	if err != nil {
		t.Fatal("native SQL deployment identity failed", err)
	}
	record := deployment.DeploymentRecord{
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "native-provider-qualification"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: project.Scope.Runtime, RemoteProject: &project},
		Observed: deployment.ObservedDeployment{State: "provider_published"},
	}
	// The deployment-owner seam (ADR-0022 one-installation-owner) requires the
	// Target to carry an authoritative Core installation before its state is
	// loadable. Seed the Core owner here so the round-trip exercises the
	// production protected registry, not a Coreless fail-closed. Real
	// installation.json in the target-state-root runtime; no global bypass.
	if seedNativeSQLTargetOwner(t, project.Scope.TargetID, project.Scope.Runtime); err != nil {
		t.Fatal("seed authoritative Core owner for native SQL target", err)
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal("native SQL project registry persistence failed", err)
	}
	restored, err := deployment.LoadDeploymentRecord(identity)
	if err != nil || restored.Applied.RemoteProject == nil || !reflect.DeepEqual(*restored.Applied.RemoteProject, project) {
		t.Fatal("native SQL durable registry restoration changed the project", err)
	}
	return restored
}

// seedNativeSQLTargetOwner provisions an authoritative Core installation for the
// native-SQL test Target so the deployment-owner seam (verifyDeploymentOwnerAtSeam
// -> coreinstallation.Load) passes before the protected registry round-trip. The
// Core's Target matches targetName; a bare installation.json in the
// target-state-root runtime, no global bypass, no skip, no off-switch.
func seedNativeSQLTargetOwner(t *testing.T, targetName, runtime string) error {
	t.Helper()
	targetRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		t.Fatalf("resolve target state root for %s: %v", targetName, err)
	}
	installRoot := filepath.Join(targetRoot, "runtime")
	spec := coreinstallation.Spec{Target: targetName, Runtime: runtime, MachineRole: coreinstallation.Development}
	if err := os.MkdirAll(installRoot, 0700); err != nil {
		t.Fatalf("create Core runtime root for %s: %v", targetName, err)
	}
	if _, runErr := coreinstallation.Run(context.Background(), installRoot, spec, coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	}); runErr != nil {
		t.Fatalf("seed authoritative Core owner for %s: %v", targetName, runErr)
	}
	return nil
}
