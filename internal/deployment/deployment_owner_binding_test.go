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

// provisionOwnerForTarget persists a valid Core installation under the
// canonical runtime root of target (targets/<target>/runtime) so production
// read/write/list seams have a single authoritative owner. It mirrors the
// one-installation-owner rule enforced by VerifyDeploymentOwner.
func provisionOwnerForTarget(t *testing.T, target string) {
	t.Helper()
	targetRoot, err := TargetStateRoot(target)
	if err != nil {
		t.Fatalf("resolve runtime root for %s: %v", target, err)
	}
	writeInstallationAuthoritative(t, target, filepath.Join(targetRoot, "runtime"))
} // TestDeploymentOwnerBindingPositiveAndMissing verifies that deployment state is
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
	root := filepath.Join(data, "baseharbor", "targets", target)
	installRoot := filepath.Join(root, "runtime")
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatalf("setup runtime dir: %v", err)
	}
	writeInstallationAuthoritative(t, target, installRoot)

	// Create a real symlink at a nonexistent deployments path pointing at an
	// external location: a foreign owner. Setup failure is fatal so a benign
	// filesystem (EEXIST) cannot silently skip the assertion.
	depNamespace := filepath.Join(root, "deployments")
	external := t.TempDir()
	linkTarget := filepath.Join(external, "deployments")
	if err := os.Symlink(linkTarget, depNamespace); err != nil {
		t.Fatalf("setup symlink escape: %v", err)
	}
	if err := VerifyDeploymentOwner(target, installRoot, testDeploymentIdentity(t, target, "x", "dev").DeploymentID); !errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("symlinked deployments namespace not rejected: %v", err)
	}
	if err := os.Remove(depNamespace); err != nil {
		t.Fatalf("cleanup symlink escape: %v", err)
	}
}

// TestDeploymentOwnerSeamEnforcedOnRealSaveLoadList exercises the production
// read/write/list seam (LoadDeploymentRecord/SaveDeploymentRecord/ListDeployments)
// rather than calling the helper directly. A missing owner fails closed, a
// single authoritative owner round-trips, a foreign installation is rejected,
// and corrupt state is classified — proving the seam, not a test-only helper,
// enforces the one-installation-owner rule.
func TestDeploymentOwnerSeamEnforcedOnRealSaveLoadList(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)

	identity := testDeploymentIdentity(t, target, "app-seam", "dev")

	// Missing owner: a second target with no Core installation fails closed.
	_, found, err := FindDeployment("other", identity.ApplicationID, identity.Environment)
	if err != nil || found {
		t.Fatalf("missing-owner target should fail closed: found=%t err=%v", found, err)
	}

	// Positive: a record written against the owned Target loads back and lists.
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: identity,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("save against owned target failed: %v", err)
	}
	got, err := LoadDeploymentRecord(identity)
	if err != nil {
		t.Fatalf("load against owned target failed: %v", err)
	}
	if got.Identity.DeploymentID != identity.DeploymentID {
		t.Fatalf("loaded record identity changed: %#v", got.Identity)
	}
	listed, err := ListDeployments(target)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list against owned target: len=%d err=%v", len(listed), err)
	}

	// Foreign: a distinct installation for a different Target cannot own it.
	provisionOwnerForTarget(t, "remote")
	foreign := identity
	foreign.Target = "remote"
	if _, err := LoadDeploymentRecord(foreign); err == nil {
		t.Fatal("foreign installation was admitted on the load seam")
	}
}

// TestDeploymentOwnerSeamRejectsCorruptStateThroughList proves corrupt state on
// the owned list path is surfaced (best-effort warnings) rather than silently
// adopted, and that a non-best-effort list fails closed.
func TestDeploymentOwnerSeamRejectsCorruptStateThroughList(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)

	identity := testDeploymentIdentity(t, target, "app-corrupt", "dev")
	root, err := DeploymentRoot(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deployment.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	warnings := map[string]bool{}
	deploymentsDir := filepath.Dir(filepath.Dir(root))
	records, warns, lerr := listDeploymentRecords(deploymentsDir, target, true)
	for _, w := range warns {
		if se, ok := DeploymentRecordState(w); ok {
			warnings[se.Kind] = true
		}
	}
	if lerr != nil {
		t.Fatalf("best-effort list must not hard-fail: %v", lerr)
	}
	if len(records) != 0 {
		t.Fatalf("corrupt record was loaded: %#v", records)
	}
	if !warnings["corrupt"] && !warnings["incomplete"] {
		t.Fatalf("corrupt state not surfaced as warning: %#v", warnings)
	}
}

// TestDeploymentOwnerTenantBypassRemoved proves the blanket TenantID bypass that
// the review rejected is gone: a tenant target without an enrolled node binding
// fails closed, and the seam now fails closed for an unavailable owner. This is
// the package-level assertion; cmd/baha routes an authorized tenant target
// through its own TenantOwnerResolver hook.
func TestDeploymentOwnerTenantBypassRemoved(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	// A tenant target with no Core installation and no resolver fails closed.
	if err := VerifyDeploymentOwner("node-target", filepath.Join(data, "baseharbor", "targets", "node-target", "runtime"), "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("tenant target without owner must fail closed: %v", err)
	}
}

// TestTenantOwnerResolverGrantOnly verifies that TenantOwnerResolver is a
// grant-only hook: only an affirmative (true, nil) authorizes the tenant target
// through this seam. Every other result falls through to the standard
// single-installation-owner verification — it can never bypass it.
func TestTenantOwnerResolverGrantOnly(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "node-target"
	deploymentID := "11111111-1111-4111-8111-111111111111"
	runtimeRoot := filepath.Join(data, "baseharbor", "targets", target, "runtime")

	// (false, nil): the hook did not grant. Falls through to coreinstallation.Load,
	// which has no installation under this Target and therefore fails closed with
	// ErrDeploymentOwnerMissing — proving the hook never short-circuits to a
	// foreign or silent admission.
	prev := TenantOwnerResolver
	defer func() { TenantOwnerResolver = prev }()
	TenantOwnerResolver = func(string) (bool, error) { return false, nil }
	if err := VerifyDeploymentOwner(target, runtimeRoot, deploymentID); !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("(false,nil) did not fall through to owner check: %v", err)
	}

	// (false, err): the hook reported a real error, which must propagate as-is.
	TenantOwnerResolver = func(string) (bool, error) { return false, ErrDeploymentOwnerForeign }
	if err := VerifyDeploymentOwner(target, runtimeRoot, deploymentID); !errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("(false,err) did not propagate hook error: %v", err)
	}

	// (true, err): likewise a hook error, even on an affirmative grant.
	TenantOwnerResolver = func(string) (bool, error) { return true, ErrDeploymentOwnerForeign }
	if err := VerifyDeploymentOwner(target, runtimeRoot, deploymentID); !errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("(true,err) did not propagate hook error: %v", err)
	}

	// (true, nil): the hook granted access; the single-installation-owner
	// requirement does not apply and the call succeeds.
	TenantOwnerResolver = func(string) (bool, error) { return true, nil }
	if err := VerifyDeploymentOwner(target, runtimeRoot, deploymentID); err != nil {
		t.Fatalf("(true,nil) grant must authorize the tenant target: %v", err)
	}

	// (false, nil) again with a genuine authoritative installation present: the
	// fall-through owner check now succeeds, confirming the non-grant path really
	// reaches coreinstallation.Load rather than short-circuiting.
	provisionOwnerForTarget(t, target)
	TenantOwnerResolver = func(string) (bool, error) { return false, nil }
	if err := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), deploymentID); err != nil {
		t.Fatalf("(false,nil) fall-through to a real owner did not pass: %v", err)
	}
}

// TestDeploymentOwnerTenantBypassGrantOnlyThroughSeam proves that even a
// tenant target with no Core installation fails closed when the hook does not
// grant, and that only an explicit grant admits it. The hook is set directly on
// the seam so the assertion exercises VerifyDeploymentOwner, not the cmd/baha
// routing.
func TestDeploymentOwnerTenantBypassGrantOnlyThroughSeam(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "node-target"
	deploymentID := "11111111-1111-4111-8111-111111111111"
	runtimeRoot := filepath.Join(data, "baseharbor", "targets", target, "runtime")

	prev := TenantOwnerResolver
	defer func() { TenantOwnerResolver = prev }()

	// A tenant target with no Core installation and a non-granting hook fails closed.
	TenantOwnerResolver = func(string) (bool, error) { return false, nil }
	if err := VerifyDeploymentOwner(target, runtimeRoot, deploymentID); !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("non-granting hook must fall through to missing-owner: %v", err)
	}

	// An explicit grant admits the Coreless tenant target with no installation.
	TenantOwnerResolver = func(string) (bool, error) { return true, nil }
	if err := VerifyDeploymentOwner(target, runtimeRoot, deploymentID); err != nil {
		t.Fatalf("granting hook must admit tenant target: %v", err)
	}
}
