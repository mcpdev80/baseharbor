package deployment

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// TestDeploymentOwnerLoanRecheckPropagatesReadError guards finding 2: a read
// failure during the owner-loan recheck is a host-state access problem, not a
// proven foreign installation. The loan recheck must propagate the true verdict
// (access-denied via the classified loader, or the concrete unchanging error)
// instead of folding it into ErrDeploymentOwnerForeign. Foreign stays reserved
// for the positive hash/target mismatch VerifyDeploymentOwnerAgainstOwner
// establishes once a state actually loads.
func TestDeploymentOwnerLoanRecheckPropagatesReadError(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	target := "local"

	// A genuine ownerless Local-access Target with a loan verifier grants the
	// loan; the recheck then runs against the test Target's own runtime root.
	TargetLoanVerifier = func(_ string) (*coreinstallation.State, error) {
		s := &coreinstallation.State{}
		return s, nil
	}
	defer func() { TargetLoanVerifier = nil }()

	// Inject a deterministic permission-denied during the loan recheck load
	// (the test Target's runtime root has no installation.json yet, so the
	// recheck calls coreinstallation.Load on it and must NOT treat the
	// permission-denied as a foreign owner).
	permErr := newPermissionDeniedError()
	testLoadInstallationForOwner = func(root string) (coreinstallation.State, error) {
		return coreinstallation.State{}, permErr
	}
	defer restoreTestLoadOwner(t)

	err := VerifyDeploymentOwner(target, filepath.Join(data, "baseharbor", "targets", target, "runtime"), "11111111-1111-4111-8111-111111111111")
	if err == nil {
		t.Fatal("loan recheck admitted a target whose installation was unreadable")
	}
	// A permission-denied read during the recheck must NOT surface as foreign.
	if errors.Is(err, ErrDeploymentOwnerForeign) {
		t.Fatalf("loan recheck read failure must NOT be classified as foreign: %#v", err)
	}
	// It must fail closed on the true access-denied verdict.
	if !errors.Is(err, ErrDeploymentOwnerAccessDenied) {
		t.Fatalf("loan recheck read failure must propagate access-denied, got: %#v", err)
	}
}
