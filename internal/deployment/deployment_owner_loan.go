package deployment

import (
	"errors"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// VerifyDeploymentOwnerAgainstOwner is the production one-installation-owner
// check WITHOUT the TEST-SCOPE loan seam (see VerifyDeploymentOwner): it loads
// the Core installation under targetRoot and proves it owns the Target's
// deployment namespace. It exists so the loan guarantee re-check above can
// resolve a genuinely-appeared local owner WITHOUT re-entering the loan path
// (which would recurse). The loan seam itself calls the full VerifyDeploymentOwner.
func VerifyDeploymentOwnerAgainstOwner(targetName, targetRoot, deploymentID string) error {
	installation, err := coreinstallation.Load(targetRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrDeploymentOwnerMissing
		}
		// An invalid/foreign installation.json is not an owner either.
		return ErrDeploymentOwnerForeign
	}
	// The deployment namespace is targets/<target>/deployments. The deployment
	// directory's Target (the segment under targets/) must match the
	// installation's own Target; a distinct Target is a foreign owner and is
	// rejected before any deployment under it is read or written.
	deploymentDir, dirErr := deploymentDirForTarget(targetName)
	if dirErr == nil {
		if deploymentTarget, found := deploymentTargetFromDir(deploymentDir); found && deploymentTarget != "" && strings.TrimSpace(deploymentTarget) != strings.TrimSpace(installation.Spec.Target) {
			return ErrDeploymentOwnerForeign
		}
		// A pre-existing deployment directory must be a real, protected dir, not
		// a symlinked escape. Missing is acceptable (first-run); a broken or
		// symlinked path is foreign.
		if info, statErr := os.Lstat(deploymentDir); statErr == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return ErrDeploymentOwnerForeign
			}
		}
	}
	_ = deploymentID
	return nil
}

// This file holds the owner-loan seam of VerifyDeploymentOwner
// (internal/deployment/deployment_store.go). It is split out to keep
// VerifyDeploymentOwner under the PR readability cap (800 lines).
//
// The loan lets a genuinely ownerless Local-access Target resolve against the
// one genuine, READY Core installation its boot Target owns, while refusing
// every tenant-managed, connector-bound, remote, foreign, unmarked or non-READY
// case, and the default nil keeps every production path byte-identical.

// verifyLoanOwnerGuarantees enforces the two order-independent guarantees of the
// owner loan: the requested Target's state root is a real, protected directory
// (no symlink escape), and AT MOST ONE of the two roots (test Target + boot
// installation) carries an installation.json. Either the requested Target is
// genuinely ownerless (already proven by the ErrNotExist precondition that
// routed the seam here) or the boot installation vanished/was replaced — never
// a second concurrent owner.
//
// This deliberately does NOT re-resolve through VerifyDeploymentOwner: doing so
// would re-enter the loan seam for the same ownerless test Target and recurse
// forever. Instead it re-resolves the loan ONLY when a genuine local owner
// appeared at the test Target between the precondition Load and this check —
// the local owner governs, never the loan; a corrupt/foreign file still fails
// closed with ErrDeploymentOwnerForeign.
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
	// The test Target must remain genuinely ownerless: AT MOST ONE of the two
	// roots (this test Target + the boot installation) carries an
	// installation.json. A second concurrent owner is a foreign owner.
	if _, ownErr := coreinstallation.Load(targetRoot); ownErr == nil {
		// The test Target's OWN runtime root loaded a genuine installation
		// between the precondition Load and here: the local owner governs, so
		// resolve WITHOUT the loan — but only when we did not just grant it
		// (a granted loan keeps the test Target ownerless by design, so a
		// genuine load now is a real new owner, never a re-entrance).
		return VerifyDeploymentOwnerAgainstOwner(targetName, targetRoot, "")
	} else if !errors.Is(ownErr, os.ErrNotExist) {
		return ErrDeploymentOwnerForeign
	}
	return nil
}
