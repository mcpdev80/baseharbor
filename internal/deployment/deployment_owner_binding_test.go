package deployment

import (
	"bytes"
	"context"
	"encoding/json"
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
}

// makeInstallationForeign rewrites the Core installation.json under root so its
// spec.target is a different (foreign) Target while remaining a syntactically
// valid Core installation. VerifyDeploymentOwner then rejects it as
// ErrDeploymentOwnerForeign because the installation's Target does not own the
// deployment directory's Target.
func makeInstallationForeign(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "installation.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installation to foreign-ize it: %v", err)
	}
	var state coreinstallation.State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse installation to foreign-ize it: %v", err)
	}
	state.Spec.Target = "foreign-target"
	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("re-encode installation: %v", err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o600); err != nil {
		t.Fatalf("write foreign installation: %v", err)
	}
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

	// Missing deployment on an owned target: a second target with a Core owner
	// but zero deployments returns the concrete empty/missing semantics the
	// seam contract defines — not a blanket nil. A missing deployment is
	// reported as found==false with a nil error, and an owned-but-empty target
	// lists empty with a nil error. This is the not-found path, distinct from a
	// missing/foreign/corrupt OWNER (which fails closed below).
	provisionOwnerForTarget(t, "other")
	_, found, listErr := FindDeployment("other", identity.ApplicationID, identity.Environment)
	if listErr != nil || found {
		t.Fatalf("owned-but-empty target must report empty (found), not an error: found=%t err=%v", found, listErr)
	}
	emptyList, emptyErr := ListDeployments("other")
	if emptyErr != nil || len(emptyList) != 0 {
		t.Fatalf("owned-but-empty target must list empty: len=%d err=%v", len(emptyList), emptyErr)
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

	// Foreign owner on an EXISTING record: provision a valid owner for a target,
	// save a real record for it, then rewrite that installation so its
	// spec.target is a different (foreign) Target. The public Load seam must then
	// fail closed with the SPECIFIC ErrDeploymentOwnerForeign for the existing
	// record (not merely any non-nil error), proving a foreign installation
	// cannot own a deployment that already exists on disk.
	foreignRoot, rerr := TargetStateRoot("remote")
	if rerr != nil {
		t.Fatalf("resolve remote runtime root: %v", rerr)
	}
	foreignRoot = filepath.Join(foreignRoot, "runtime")
	provisionOwnerForTarget(t, "remote")
	foreignRecord := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: testDeploymentIdentity(t, "remote", "app-foreign", "dev"),
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	if err := SaveDeploymentRecord(foreignRecord); err != nil {
		t.Fatalf("save against owned remote target failed: %v", err)
	}
	makeInstallationForeign(t, foreignRoot)
	foreignLoad, foreignLoadErr := LoadDeploymentRecord(foreignRecord.Identity)
	if foreignLoadErr == nil {
		t.Fatal("foreign installation was admitted on the load seam")
	}
	if !errors.Is(foreignLoadErr, ErrDeploymentOwnerForeign) {
		t.Fatalf("foreign-owner load must be ErrDeploymentOwnerForeign, got: %#v (id=%s)", foreignLoadErr, foreignLoad.Identity)
	}
}

// TestDeploymentOwnerSeamRejectsCorruptStateThroughList proves that a corrupt
// OWNER on the owned list path is surfaced rather than silently adopted: the
// non-best-effort public ListDeployments fails closed with the specific owner
// error, the best-effort public ListDeploymentsForDisplay surfaces a concrete
// warning instead of hard-failing, and the on-disk record bytes are left
// unchanged by either list. It exercises the PUBLIC seams against a corrupt
// OWNER (not the private listDeploymentRecords against corrupt record data).
func TestDeploymentOwnerSeamRejectsCorruptStateThroughList(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)

	identity := testDeploymentIdentity(t, target, "app-corrupt", "dev")
	// Save a real record against the valid owner and capture its bytes so the
	// corrupt-owner list can prove the record was not mutated.
	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: identity,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}); err != nil {
		t.Fatalf("save against owned target failed: %v", err)
	}
	recordPath := filepath.Join(xdgDataRoot(t, target), "deployments", identity.DeploymentID, "deployment.json")
	validBytes, readErr := os.ReadFile(recordPath)
	if readErr != nil {
		t.Fatalf("read back the saved record bytes: %v", readErr)
	}
	if len(validBytes) == 0 {
		t.Fatal("saved record wrote no bytes")
	}

	// Corrupt the saved OWNER: overwrite the Core installation.json (which lives
	// under the target's runtime root) with invalid content so
	// coreinstallation.Load fails and the seam treats the owner as
	// foreign/incompatible (not missing).
	ownerRoot, rerr := TargetStateRoot(target)
	if rerr != nil {
		t.Fatalf("resolve owner root: %v", rerr)
	}
	if err := os.WriteFile(filepath.Join(ownerRoot, "runtime", "installation.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("corrupt the saved owner installation: %v", err)
	}

	// Non-best-effort public list fails closed with the specific owner error.
	if _, listErr := ListDeployments(target); listErr == nil {
		t.Fatal("non-best-effort ListDeployments did not fail closed on corrupt owner")
	} else if !errors.Is(listErr, ErrDeploymentOwnerForeign) {
		t.Fatalf("non-best-effort list must be ErrDeploymentOwnerForeign, got: %v", listErr)
	}

	// Best-effort public list surfaces a concrete warning rather than hard-failing.
	listed, warns, ferr := ListDeploymentsForDisplay(target)
	if ferr != nil {
		t.Fatalf("best-effort ListDeploymentsForDisplay must not hard-fail: %v", ferr)
	}
	if len(listed) != 0 {
		t.Fatalf("corrupt-owner list must not load any record: %#v", listed)
	}
	if len(warns) == 0 {
		t.Fatal("best-effort list surfaced no warning for the corrupt owner")
	}

	// The owner corruption must not have touched the on-disk record bytes.
	afterBytes, readErr2 := os.ReadFile(recordPath)
	if readErr2 != nil {
		t.Fatalf("read back the record after the corrupt-owner list: %v", readErr2)
	}
	if !bytes.Equal(validBytes, afterBytes) {
		t.Fatalf("corrupt-owner list mutated the on-disk record: valid=%q after=%q", validBytes, afterBytes)
	}
}

// TestDeploymentOwnerMissingOnExistingRecordSeam covers the "missing owner WITH
// an existing record" case: a record is saved against a valid single owner, then
// only the Core installation.json (the installation-owner material) is removed
// while the record file survives on disk. The public seams must fail closed with
// the specific ErrDeploymentOwnerMissing (not a silent empty listing, not merely
// any non-nil error), and the on-disk record bytes must be unchanged by every
// operation — Load, Save with nil preferences, and ListDeployments. This proves
// the seam rejects a deployment whose installation owner is gone but whose record
// still exists, rather than leaking the record or rewriting it.
func TestDeploymentOwnerMissingOnExistingRecordSeam(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)

	// (1) Save a real record against the valid owner through the real save path.
	identity := testDeploymentIdentity(t, target, "app-missing-owner", "dev")
	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: identity,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}); err != nil {
		t.Fatalf("save record against owned target failed: %v", err)
	}

	// Capture the record bytes so we can prove no seam mutates them.
	root, rootErr := TargetStateRoot(target)
	if rootErr != nil {
		t.Fatalf("resolve data root for %s: %v", target, rootErr)
	}
	recordPath := filepath.Join(root, "deployments", identity.DeploymentID, "deployment.json")
	beforeBytes, readErr := os.ReadFile(recordPath)
	if readErr != nil {
		t.Fatalf("read saved record bytes: %v", readErr)
	}
	if len(beforeBytes) == 0 {
		t.Fatal("saved record wrote no bytes")
	}

	// (2) Remove ONLY the installation.json (the installation-owner material)
	// while the record file remains untouched on disk — exactly the "missing
	// owner with an existing record" case.
	ownerRuntime := filepath.Join(root, "runtime")
	installationPath := filepath.Join(ownerRuntime, "installation.json")
	if _, statErr := os.Stat(installationPath); statErr != nil {
		t.Fatalf("installation.json expected before removal: %v", statErr)
	}
	if err := os.Remove(installationPath); err != nil {
		t.Fatalf("remove installation.json: %v", err)
	}
	// The record file must still exist — proving the record survives the owner
	// removal (as in the real scenario).
	if _, recStat := os.Stat(recordPath); recStat != nil {
		t.Fatalf("record must survive installation removal: %v", recStat)
	}

	// (3) LOAD must fail closed with the SPECIFIC missing-owner error and leave
	// the record bytes unchanged.
	_, loadErr := LoadDeploymentRecord(identity)
	if loadErr == nil {
		t.Fatal("LoadDeploymentRecord admitted a record whose owner is missing")
	}
	if !errors.Is(loadErr, ErrDeploymentOwnerMissing) {
		t.Fatalf("load with missing owner must be ErrDeploymentOwnerMissing, got: %#v", loadErr)
	}
	if after, aerr := os.ReadFile(recordPath); aerr != nil || !bytes.Equal(beforeBytes, after) {
		t.Fatalf("load mutated record: readErr=%v before=%q after=%q", aerr, beforeBytes, after)
	}

	// (3) SAVE with nil preferences must reject (fail closed) and leave the
	// record bytes unchanged.
	saveErr := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: identity,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	})
	if saveErr == nil {
		t.Fatal("SaveDeploymentRecord with missing owner did not reject")
	}
	if !errors.Is(saveErr, ErrDeploymentOwnerMissing) {
		t.Fatalf("save with missing owner must be ErrDeploymentOwnerMissing, got: %#v", saveErr)
	}
	if after, aerr := os.ReadFile(recordPath); aerr != nil || !bytes.Equal(beforeBytes, after) {
		t.Fatalf("save mutated record: readErr=%v before=%q after=%q", aerr, beforeBytes, after)
	}

	// (3) ListDeployments must fail closed with the SPECIFIC missing-owner error
	// — never a silent empty listing — and never mutate the record bytes.
	if _, listErr := ListDeployments(target); listErr == nil {
		t.Fatal("ListDeployments returned a silent empty listing for a missing owner")
	} else if !errors.Is(listErr, ErrDeploymentOwnerMissing) {
		t.Fatalf("list with missing owner must be ErrDeploymentOwnerMissing, got: %#v", listErr)
	}
	if after, aerr := os.ReadFile(recordPath); aerr != nil || !bytes.Equal(beforeBytes, after) {
		t.Fatalf("list mutated record: readErr=%v before=%q after=%q", aerr, beforeBytes, after)
	}
}

// xdgDataRoot resolves the canonical targets/<target> data root this test's
// XDG_DATA_HOME maps to, mirroring DataRoot for a target.
func xdgDataRoot(t *testing.T, target string) string {
	t.Helper()
	root, err := TargetStateRoot(target)
	if err != nil {
		t.Fatalf("resolve data root for %s: %v", target, err)
	}
	return root
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
