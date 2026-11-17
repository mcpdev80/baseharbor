package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// restoreTestLoadOwner restores the real loader after a test that injects a
// deterministic permission-denied error through the test seam.
func restoreTestLoadOwner(t *testing.T) {
	t.Helper()
	testLoadInstallationForOwner = nil
}

// TestDeploymentOwnerAccessDeniedIsNotForeign proves the core bug fix: a Core
// installation that CANNOT BE READ (permission denied) is classified as
// ErrDeploymentOwnerAccessDenied — a named host-state access error, distinct
// from both ErrDeploymentOwnerMissing and ErrDeploymentOwnerForeign. The
// DeploymentRecordState kind surfaces access-denied, and the save/load/list
// seams all carry it, while the on-disk record bytes are left unchanged. The
// fake permission-denied error is injected through a package-internal test seam
// (testLoadInstallationForOwner) so the assertion is deterministic in CI and
// does not depend on real file permissions, which are root-dependent on the
// runner.
func TestDeploymentOwnerAccessDeniedIsNotForeign(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"
	provisionOwnerForTarget(t, target)
	provisionOwnerForTarget(t, "other")

	identity := testDeploymentIdentity(t, target, "app-access", "dev")
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: identity,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("save against owned target failed: %v", err)
	}
	recordPath := filepath.Join(xdgDataRoot(t, target), "deployments", identity.DeploymentID, "deployment.json")
	validBytes, readErr := os.ReadFile(recordPath)
	if readErr != nil {
		t.Fatalf("read back saved record bytes: %v", readErr)
	}
	if len(validBytes) == 0 {
		t.Fatal("saved record wrote no bytes")
	}

	// Inject a deterministic permission-denied through the test seam instead of
	// relying on real file permissions (root-dependent on CI runners).
	permErr := newPermissionDeniedError()
	testLoadInstallationForOwner = func(root string) (coreinstallation.State, error) {
		return coreinstallation.State{}, permErr
	}
	defer restoreTestLoadOwner(t)

	// VerifyDeploymentOwner must be ErrDeploymentOwnerAccessDenied, never
	// ErrDeploymentOwnerForeign or ErrDeploymentOwnerMissing.
	ownerErr := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), identity.DeploymentID)
	if ownerErr == nil {
		t.Fatal("access-denied owner was admitted on the seam")
	}
	if !errors.Is(ownerErr, ErrDeploymentOwnerAccessDenied) {
		t.Fatalf("access-denied owner must be ErrDeploymentOwnerAccessDenied, got: %#v", ownerErr)
	}
	if errors.Is(ownerErr, ErrDeploymentOwnerForeign) {
		t.Fatalf("access-denied owner must NOT be classified as foreign: %#v", ownerErr)
	}
	if errors.Is(ownerErr, ErrDeploymentOwnerMissing) {
		t.Fatalf("access-denied owner must NOT be classified as missing: %#v", ownerErr)
	}

	// The wrapped error must carry the underlying fs cause so the CLI can name
	// the path and reason: the sentinel is reachable and the permission error is
	// chainable through errors.Is.
	if !errors.Is(ownerErr, ErrDeploymentOwnerAccessDenied) {
		t.Fatalf("access-denied error must carry the sentinel: %#v", ownerErr)
	}
	if !errors.Is(ownerErr, newPermissionDeniedError()) {
		t.Fatalf("access-denied error must surface the underlying permission error: %#v", ownerErr)
	}

	// The load seam must carry access-denied too (not a silent load, not foreign).
	_, loadErr := LoadDeploymentRecord(identity)
	if loadErr == nil {
		t.Fatal("LoadDeploymentRecord admitted a record whose owner is unreadable")
	} else if !errors.Is(loadErr, ErrDeploymentOwnerAccessDenied) {
		t.Fatalf("load with unreadable owner must be ErrDeploymentOwnerAccessDenied, got: %#v", loadErr)
	}

	// The save seam must fail closed on access-denied and not mutate the record.
	if saveErr := SaveDeploymentRecord(record); saveErr == nil {
		t.Fatal("SaveDeploymentRecord admitted a record whose owner is unreadable")
	} else if !errors.Is(saveErr, ErrDeploymentOwnerAccessDenied) {
		t.Fatalf("save with unreadable owner must be ErrDeploymentOwnerAccessDenied, got: %#v", saveErr)
	}
	if after, aerr := os.ReadFile(recordPath); aerr != nil || !equalBytes(validBytes, after) {
		t.Fatalf("access-denied seams mutated record: readErr=%v before=%q after=%q", aerr, validBytes, after)
	}

	// The non-best-effort list must fail closed with access-denied.
	if _, listErr := ListDeployments(target); listErr == nil {
		t.Fatal("ListDeployments returned a silent listing for an unreadable owner")
	} else if !errors.Is(listErr, ErrDeploymentOwnerAccessDenied) {
		t.Fatalf("list with unreadable owner must be ErrDeploymentOwnerAccessDenied, got: %#v", listErr)
	}
	// The best-effort list surfaces a concrete access-denied warning.
	if listed, warns, ferr := ListDeploymentsForDisplay(target); ferr != nil {
		t.Fatalf("best-effort list must not hard-fail: %v", ferr)
	} else if len(listed) != 0 {
		t.Fatalf("access-denied list must not load any record: %#v", listed)
	} else if len(warns) == 0 {
		t.Fatal("best-effort list surfaced no warning for the unreadable owner")
	}

	// The DeploymentRecordState kind must classify the access-denied as a
	// distinct missing-owner kind (not foreign, not a silent empty result).
	_, loadErr = LoadDeploymentRecord(identity)
	state, ok := DeploymentRecordState(loadErr)
	if !ok {
		t.Fatalf("access-denied load must be a DeploymentRecordStateError: %#v", loadErr)
	}
	if state.Kind != "missing" {
		t.Fatalf("access-denied must classify as missing-owner kind, got kind=%q: %#v", state.Kind, state)
	}
	if !errors.Is(state.Err, ErrDeploymentOwnerAccessDenied) {
		t.Fatalf("state error must carry ErrDeploymentOwnerAccessDenied: %#v", state.Err)
	}
}

// TestDeploymentOwnerMissingStillMissingAndForeignStillForeign guards the
// boundaries of the access-denied split: a genuinely absent installation stays
// ErrDeploymentOwnerMissing, and a loaded-but-foreign installation (wrong
// spec.target) stays ErrDeploymentOwnerForeign — neither must collapse to the
// new access-denied sentinel.
func TestDeploymentOwnerMissingStillMissingAndForeignStillForeign(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"

	// Absent installation -> missing, not access-denied, not foreign.
	if err := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerMissing) {
		t.Fatalf("absent installation must stay missing, got: %#v", err)
	}

	// Foreign installation (loaded, wrong target) -> foreign, not access-denied.
	provisionOwnerForTarget(t, target)
	if err := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), "11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatalf("authoritative installation should own its deployment: %v", err)
	}
	installPath := filepath.Join(data, "baseharbor", "targets", target, "runtime", "installation.json")
	raw, readErr := os.ReadFile(installPath)
	if readErr != nil {
		t.Fatalf("read installation to foreign-ize: %v", readErr)
	}
	var state coreinstallation.State
	if err := unmarshalInstallation(raw, &state); err != nil {
		t.Fatalf("parse installation: %v", err)
	}
	state.Spec.Target = "foreign-target"
	out, marshalErr := marshalInstallation(state)
	if marshalErr != nil {
		t.Fatalf("re-encode installation: %v", marshalErr)
	}
	if writeErr := os.WriteFile(installPath, out, 0o600); writeErr != nil {
		t.Fatalf("write foreign installation: %v", writeErr)
	}
	if err := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), "11111111-1111-4111-8111-111111111111"); !errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("loaded-but-foreign installation must stay foreign, got: %#v", err)
	}
}

// TestDeploymentOwnerEmptyTargetUsageError proves that an empty/invalid target
// name is reported as a usage error naming the offending input, never as a
// foreign installation. This preserves BF-HD6-2: the InvalidTargetName error
// propagates through the seam as-is.
func TestDeploymentOwnerEmptyTargetUsageError(t *testing.T) {
	// An empty target name is a usage error naming the offending input.
	if err := ValidateTargetName(""); err == nil {
		t.Fatal("empty target name must be a usage error")
	} else if !containsString(err.Error(), `"`) {
		t.Fatalf("usage error must name the offending input: %v", err)
	}
	// ListDeployments("") surfaces the usage error, never ErrDeploymentOwnerForeign.
	if _, listErr := ListDeployments(""); listErr == nil {
		t.Fatal("empty target must not list silently")
	} else if errors.Is(listErr, ErrDeploymentOwnerForeign) {
		t.Fatalf("empty target must NOT be classified as foreign: %#v", listErr)
	} else if !containsString(listErr.Error(), `invalid target name`) {
		t.Fatalf("empty target must be an InvalidTargetName usage error, got: %#v", listErr)
	}
}

// equalBytes compares two byte slices for equality without importing testing in
// this helper (keeps the helper usable from non-test code if needed).
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// containsString reports whether needle appears in haystack.
func containsString(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOfString(haystack, needle) >= 0)
}

func indexOfString(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
