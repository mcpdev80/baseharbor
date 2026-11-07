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
// t-20261106-r27-remote-up-podman-owner-missing (lane w286). It deliberately
// uses ONLY the exported seam and the pre-existing test helpers so it
// compiles — and fails — on the broken commit: the defect is that a
// marker-proven isolated test Target without its own installation.json is
// refused with deployment.ErrDeploymentOwnerMissing on the real Save/Load
// seam, which is exactly what the InCI compose-workload cells hit when they
// apply an application against their isolation target. On the broken commit
// every "expected" path below fails; at the fix commit the guarded TEST-SCOPE
// owner loan (see cmd/baha/deployment_record.go and
// internal/deployment/deployment_store.go) makes them pass while all
// fail-closed refusals stay refused.

// bahaTestIsolationMarkerFile and bahaTestIsolationMarkerSchema name the
// machine-readable marker file the loan verifier reads from a test Target's
// state root. The constants and the writer live in this test-only file so a
// production binary never contains the trigger; on the broken commit they are
// still valid inputs for the fixture, only the consumer is missing.
const bahaTestIsolationMarkerFile = "baseharbor.test-isolation.json"
const bahaTestIsolationMarkerSchema = "baseharbor.test-isolation/v1"

// writeBahaTestIsolationMarker persists the marker file in the Target's state
// root. Test code is the only writer: the loan trigger never comes from a
// caller-supplied environment variable.
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

// seedLoanTestInstallation provisions a genuine, READY Core installation under
// the runtime root of the named Target (identity-less no-op steps, same
// fixture shape as internal/deployment/deployment_owner_binding_test.go).
func seedLoanTestInstallation(t *testing.T, targetName, runtime string) coreinstallation.State {
	t.Helper()
	root, err := targetRuntimeStateRootByName(targetName)
	if err != nil {
		t.Fatalf("resolve runtime root for %q: %v", targetName, err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	spec := coreinstallation.Spec{Target: targetName, Runtime: runtime, MachineRole: coreinstallation.Development}
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

// loanTestConfig persists a two-Target config: a boot Target that owns the
// installation and an isolated test/production-shaped Target without one.
func loanTestConfig(t *testing.T, boot, other string, connectorAccess bool) {
	t.Helper()
	access := map[string]deployment.AccessDefinition{
		"local": {Provider: "local", Reference: "local"},
	}
	otherDef := deployment.TargetDefinition{
		Runtime: deployment.RuntimeDefinition{Provider: "docker"},
		Access:  deployment.TargetAccess{Reference: "local"},
		Scope:   "local",
	}
	if connectorAccess {
		access["connector"] = deployment.AccessDefinition{Provider: "baseharbor-node-connector", Reference: "node-a"}
		otherDef = deployment.TargetDefinition{
			Runtime: deployment.RuntimeDefinition{Provider: "docker"},
			Access:  deployment.TargetAccess{Reference: "connector"},
		}
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: boot,
		Access:        access,
		Targets: map[string]deployment.TargetDefinition{
			boot: {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local"},
				Scope:   "local",
			},
			other: otherDef,
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("persist loan test config: %v", err)
	}
}

func loanTestIdentity(t *testing.T, target, app string) deployment.DeploymentIdentity {
	t.Helper()
	return deployment.DeploymentIdentity{
		DeploymentID:  "77777777-7777-4777-8777-777777777777",
		ApplicationID: application.MustNewApplicationID(),
		Target:        target,
		Application:   app,
		Environment:   "dev",
	}
}

// loanTestFixture builds: XDG roots, a boot Target owning a genuine
// installation, and a separate Target with an empty (but protected) state
// root. It returns the boot installation's identity.
func loanTestFixture(t *testing.T, boot, other string, connectorAccess bool) coreinstallation.State {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", other)
	state := seedLoanTestInstallation(t, boot, "docker")
	loanTestConfig(t, boot, other, connectorAccess)
	otherRoot, err := deployment.TargetStateRoot(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherRoot, 0700); err != nil {
		t.Fatal(err)
	}
	return state
}

// saveThroughSeam attempts a deployment record save for the Target via the
// PUBLIC seam exactly like recordDeploymentBeforeMutation does: Verify first,
// then Save (Save re-verifies internally; the pre-check keeps the error shape
// identical to the apply path).
func saveThroughSeam(t *testing.T, target, app string) error {
	t.Helper()
	identity := loanTestIdentity(t, target, app)
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

// TestOwnerLoanRegressionIsolatedTestTargetPassesSeam is the exact-defect
// cell: a marker-proven isolated test Target whose runtime root has no
// installation.json must pass the owner seam against the boot installation.
// On the broken commit the save fails with ErrDeploymentOwnerMissing (RED).
func TestOwnerLoanRegressionIsolatedTestTargetPassesSeam(t *testing.T) {
	const boot, iso = "w286-loan-boot", "w286-loan-iso"
	state := loanTestFixture(t, boot, iso, false)

	// Defect precondition: the isolated target has no owner of its own.
	if _, err := coreinstallation.Load(mustLoanTargetRuntimeRoot(t, iso)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated target must have no installation.json: err=%v", err)
	}
	writeBahaTestIsolationMarker(t, iso)

	if err := saveThroughSeam(t, iso, "loan-isolated"); err != nil {
		t.Fatalf("marker-proven isolated test target must pass the owner seam (card t-20261106-r27-remote-up-podman-owner-missing): %v", err)
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
	bootState, err := coreinstallation.Load(mustLoanTargetRuntimeRoot(t, boot))
	if err != nil {
		t.Fatal(err)
	}
	if bootState.ID != state.ID {
		t.Fatal("boot installation identity changed through the loan path")
	}
}

// TestOwnerLoanRegressionRefusalsStayFailClosed pins the guard set: shapes
// that do NOT look like a freshly isolated test cell keep failing closed with
// ErrDeploymentOwnerMissing — before and after the fix.
func TestOwnerLoanRegressionRefusalsStayFailClosed(t *testing.T) {
	t.Run("unmarked-target-stays-refused", func(t *testing.T) {
		const boot, prod = "w286-loan-boot", "w286-loan-prod"
		_ = loanTestFixture(t, boot, prod, false)
		// Deliberately NO marker: a production-shaped ownerless Target must
		// never receive a loan.
		err := saveThroughSeam(t, prod, "loan-prod")
		if !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
			t.Fatalf("unmarked target must stay fail-closed owner-missing: %v", err)
		}
	})
	t.Run("connector-target-stays-refused", func(t *testing.T) {
		const boot, node = "w286-loan-boot", "w286-loan-node"
		_ = loanTestFixture(t, boot, node, true)
		// The w273 remote (connector) shape: even with a marker file present
		// on disk the loan must not reach it — that path stays fail-closed.
		writeBahaTestIsolationMarker(t, node)
		err := saveThroughSeam(t, node, "loan-node")
		if !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
			t.Fatalf("connector-bound target must stay fail-closed owner-missing: %v", err)
		}
	})
}

func mustLoanTargetRuntimeRoot(t *testing.T, name string) string {
	t.Helper()
	root, err := targetRuntimeStateRootByName(name)
	if err != nil {
		t.Fatalf("resolve runtime root for %q: %v", name, err)
	}
	return root
}

// TestOwnerLoanRegressionStartupBoundAuthorityRefused pins the w273-relevant
// TestOwnerLoanRegressionStartupBoundAuthorityRefused pins the w273-relevant
// guard: a connector-bound Target that carries the marker file still never
// receives a loan. Both the broken tree (no loan at all) and the fixed tree
// (connector guard refuses) must fail the write closed with the SAME sentinel
// ErrDeploymentOwnerMissing, so the cell compiles and asserts on both.
func TestOwnerLoanRegressionStartupBoundAuthorityRefused(t *testing.T) {
	const boot, node = "w286-loan-boot", "w286-loan-node"
	_ = loanTestFixture(t, boot, node, true)
	writeBahaTestIsolationMarker(t, node)
	if err := saveThroughSeam(t, node, "loan-node"); !errors.Is(err, deployment.ErrDeploymentOwnerMissing) {
		t.Fatalf("connector-bound target must stay fail-closed owner-missing: %v", err)
	}
}
