package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// This file is the RED-FIRST regression for card
// t-20261106-r27-remote-up-podman-owner-missing (card number 0427-117). It
// exercises the REAL remote-podman app-apply owner seam exactly like
// recordDeploymentBeforeMutation does: a marker-proven isolated podman Target
// with no own installation.json must resolve against the boot installation
// that the CLI's OWN effective-target selection resolves (activated target ->
// persisted -> config default -> single target -> "local").
//
// On the broken commit every "expected" path fails with
// deployment.ErrDeploymentOwnerMissing (RED); at the fix commit the guarded
// TEST-SCOPE owner loan makes them pass while all fail-closed refusals stay
// refused. The test uses ONLY the exported seam and pre-existing helpers so it
// compiles — and fails — on the broken commit.

// bahaTestIsolationMarkerFile / bahaTestIsolationMarkerSchema name the
// machine-readable marker the loan verifier reads. Kept in this test-only file
// so a production binary never contains the trigger.
const bahaTestIsolationMarkerFile = "baseharbor.test-isolation.json"
const bahaTestIsolationMarkerSchema = "baseharbor.test-isolation/v1"

func writeBahaTestIsolationMarker(t *testing.T, targetName string) {
	t.Helper()
	targetRoot, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		t.Fatalf("resolve target state root for %q: %v", targetName, err)
	}
	if err := os.MkdirAll(targetRoot, 0700); err != nil {
		t.Fatalf("create target state root for marker: %v", err)
	}
	info, err := os.Lstat(targetRoot)
	if err != nil {
		t.Fatalf("stat target state root: %v", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("target state root must be a protected directory, got %v", info.Mode())
	}
	marker := map[string]string{"version": bahaTestIsolationMarkerSchema, "target": strings.TrimSpace(targetName)}
	raw, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("encode test isolation marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetRoot, bahaTestIsolationMarkerFile), append(raw, '\n'), 0600); err != nil {
		t.Fatalf("write test isolation marker: %v", err)
	}
}

// seedBootInstallation provisions a genuine, READY Core installation under the
// runtime root of the boot Target (identity-less no-op steps, same fixture
// shape as internal/deployment/deployment_owner_binding_test.go).
func seedBootInstallation(t *testing.T, bootTarget, runtime string) coreinstallation.State {
	t.Helper()
	root, err := targetRuntimeStateRootByName(bootTarget)
	if err != nil {
		t.Fatalf("resolve runtime root for %q: %v", bootTarget, err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	spec := coreinstallation.Spec{Target: bootTarget, Runtime: runtime, MachineRole: coreinstallation.Development}
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
		t.Fatalf("provision boot installation: %v", err)
	}
	if !state.Ready {
		t.Fatal("fixture boot installation is not ready")
	}
	return state
}

// loanIsolatedPodmanConfig persists a boot Target that owns the installation
// and an isolated podman test Target without one. The boot Target is the
// configuration DEFAULT so the CLI effective-target selection resolves it —
// exactly like the remote-podman apply path selects its boot Core.
func loanIsolatedPodmanConfig(t *testing.T, boot, iso string) {
	access := map[string]deployment.AccessDefinition{
		"local": {Provider: "local", Reference: "local"},
	}
	bootDef := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "podman"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	isoDef := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "podman"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: boot,
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			boot: bootDef,
			iso:  isoDef,
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist isolated-podman loan config: %v", err)
	}
}

func loanIsolatedIdentity(t *testing.T, target, app string) deployment.DeploymentIdentity {
	t.Helper()
	return deployment.DeploymentIdentity{
		DeploymentID:  "77777777-7777-4777-8777-777777777777",
		ApplicationID: application.MustNewApplicationID(),
		Target:        target,
		Application:   app,
		Environment:   "dev",
	}
}

// isolatePodmanBootTarget sets up: XDG roots, a boot Target owning a genuine
// podman installation, and a separate isolated podman Target with an empty
// (but protected) state root. Returns the boot installation identity.
func isolatePodmanBootTarget(t *testing.T, boot, iso string) coreinstallation.State {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state := seedBootInstallation(t, boot, "podman")
	loanIsolatedPodmanConfig(t, boot, iso)
	isoRoot, err := deployment.TargetStateRoot(iso)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(isoRoot, 0700); err != nil {
		t.Fatal(err)
	}
	return state
}

// saveThroughApplySeam mimics recordDeploymentBeforeMutation: Verify first,
// then Save (Save re-verifies internally); keeps the error shape identical to
// the real apply path.
func saveThroughApplySeam(t *testing.T, target, app string) error {
	t.Helper()
	identity := loanIsolatedIdentity(t, target, app)
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.VerifyDeploymentOwner(target, filepath.Join(root, "runtime"), identity.DeploymentID); err != nil {
		return err
	}
	record := deployment.DeploymentRecord{Identity: identity}
	record.Applied.Intent = json.RawMessage(`{"version":1}`)
	return deployment.SaveDeploymentRecord(record)
}

func mustBootRuntimeRoot(t *testing.T, name string) string {
	t.Helper()
	root, err := targetRuntimeStateRootByName(name)
	if err != nil {
		t.Fatalf("resolve runtime root for %q: %v", name, err)
	}
	return root
}

func mustLoadCfg(t *testing.T) deployment.Config {
	t.Helper()
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

// TestPodmanOwnerLoanIsolatedApplyPasses is the exact-defect cell: a
// marker-proven isolated podman Target with no installation.json must pass the
// owner seam against the boot installation the CLI selects. On the broken
// commit the save fails with ErrDeploymentOwnerMissing (RED).
func TestPodmanOwnerLoanIsolatedApplyPasses(t *testing.T) {
	const boot, iso = "w427-podman-boot", "w427-podman-iso"
	state := isolatePodmanBootTarget(t, boot, iso)

	if _, err := coreinstallation.Load(mustBootRuntimeRoot(t, iso)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated podman target must have no installation.json: err=%v", err)
	}
	writeBahaTestIsolationMarker(t, iso)

	t.Logf("BASEHARBOR_TARGET=%q DefaultTarget=%q", os.Getenv("BASEHARBOR_TARGET"), mustLoadCfg(t).DefaultTarget)
	if err := saveThroughApplySeam(t, iso, "podman-isolated"); err != nil {
		t.Fatalf("marker-proven isolated podman target must pass the owner seam (card t-20261106-r27-remote-up-podman-owner-missing): %v", err)
	}

	// The loan must never materialize a second installation.json: the boot
	// installation stays the single durable owner (ADR 0021).
	isoRoot, err := deployment.TargetStateRoot(iso)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(isoRoot, "runtime", "installation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loan must not write an installation.json at the test target: err=%v", err)
	}
	bootState, err := coreinstallation.Load(mustBootRuntimeRoot(t, boot))
	if err != nil {
		t.Fatal(err)
	}
	if bootState.ID != state.ID {
		t.Fatal("boot installation identity changed through the loan path")
	}
}

// TestPodmanOwnerLoanConnectorBoundStaysFailClosed pins the fail-closed guard:
// a connector-bound isolated target never receives a loan, before AND after
// the fix, with the SAME sentinel ErrDeploymentOwnerMissing.
func TestPodmanOwnerLoanConnectorBoundStaysFailClosed(t *testing.T) {
	const boot, node = "w427-podman-boot", "w427-podman-node"
	access := map[string]deployment.AccessDefinition{
		"local":                     {Provider: "local", Reference: "local"},
		"baseharbor-node-connector": {Provider: "baseharbor-node-connector", Reference: "node-a"},
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: boot,
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			boot: {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "local"}, Scope: "local"},
			node: {Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "baseharbor-node-connector"}},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist loan config: %v", err)
	}
	seedBootInstallation(t, boot, "podman")
	if _, err := coreinstallation.Load(mustBootRuntimeRoot(t, node)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("connector node must have no installation.json: err=%v", err)
	}
	writeBahaTestIsolationMarker(t, node)
	if err := saveThroughApplySeam(t, node, "podman-node"); !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("connector-bound isolated target must stay fail-closed owner-missing: %v", err)
	}
}

// TestPodmanOwnerLoanUnmarkedStaysFailClosed pins the unmarked guard: an
// isolated target WITHOUT the isolation marker never receives a loan.
func TestPodmanOwnerLoanUnmarkedStaysFailClosed(t *testing.T) {
	const boot, prod = "w427-podman-boot", "w427-podman-prod"
	isolatePodmanBootTarget(t, boot, prod)
	// Deliberately NO marker: a production-shaped ownerless Target must never
	// receive a loan.
	if err := saveThroughApplySeam(t, prod, "podman-prod"); !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("unmarked isolated target must stay fail-closed owner-missing: %v", err)
	}
}
