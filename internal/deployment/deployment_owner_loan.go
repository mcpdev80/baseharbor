package deployment

import (
	"errors"
	"os"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// This file holds the TEST-SCOPE ONLY owner-loan seam of
// VerifyDeploymentOwner (internal/deployment/deployment_store.go). It is split
// out from deployment_store.go so that storage seam stays within the single-file
// readability cap while the loan guard — a distinct, audited concern — lives in
// its own file. No other symbol is moved; the seam calls TargetLoanVerifier
// from deployment_store.go after the tenant hook has refused and the Target's
// own runtime root returns os.ErrNotExist.

// TargetLoanVerifier resolves a marker-proven isolated test Target against the
// single installation its boot Target owns (TEST SCOPE ONLY; the consumer binds
// it in cmd/baha/deployment_record.go and only after the tenant hook has
// refused). Every refusal — an error or a nil state — keeps the seam's
// original ErrDeploymentOwnerMissing sentinel; it can never authorize a
// tenant-managed, connector-bound or production Target, and the default nil
// keeps every production path byte-identical.
var TargetLoanVerifier func(targetName string) (*coreinstallation.State, error)

// verifyLoanOwnerGuarantees enforces the two order-independent guarantees of the
// TEST-SCOPE owner loan: the marker-proven test Target's state root is a real,
// protected directory (no symlink escape), and AT MOST ONE of the two roots
// (test Target + boot installation) carries an installation.json. Either the
// test Target is genuinely ownerless (already proven by the ErrNotExist
// precondition that routed the seam here) or the boot installation
// vanished/was replaced — never a second concurrent owner. This deliberately
// does not re-Load the test root: a re-Load would race with a concurrent
// first-apply provisioning and refuse a legitimate in-flight grant with the
// wrong sentinel (that race cost this lane a false-red podman cell). If a
// genuine owner appeared at the test Target between the precondition Load and
// this check, re-resolve WITHOUT the loan against that owner — the local owner
// governs, never the loan; a corrupt/foreign file still fails closed with
// ErrDeploymentOwnerForeign.
func verifyLoanOwnerGuarantees(targetName, targetRoot string) error {
	targetRootDir, rootErr := TargetStateRoot(targetName)
	if rootErr != nil {
		return rootErr
	}
	info, statErr := os.Lstat(targetRootDir)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return ErrDeploymentOwnerMissing
		}
		return statErr
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrDeploymentOwnerForeign
	}
	if _, ownErr := coreinstallation.Load(targetRoot); ownErr == nil {
		return VerifyDeploymentOwner(targetName, targetRoot, "")
	} else if !errors.Is(ownErr, os.ErrNotExist) {
		return ErrDeploymentOwnerForeign
	}
	return nil
}
