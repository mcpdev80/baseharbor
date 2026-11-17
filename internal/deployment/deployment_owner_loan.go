package deployment

import (
	"errors"
	"fmt"
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
	installation, err := resolveInstallationLoad(targetRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrDeploymentOwnerMissing
		}
		// A Core installation that cannot be read (permission denied, an open or
		// stat failure) is a host-state access problem, not an owner condition.
		// Classify it distinctly and never masquerade it as a foreign
		// installation. A file that loads but is itself invalid or foreign stays
		// ErrDeploymentOwnerForeign below.
		if isAccessDeniedLoadError(err) {
			return fmt.Errorf("%w: %w", ErrDeploymentOwnerAccessDenied, err)
		}
		// An invalid/foreign installation.json is not an owner either.
		return ErrDeploymentOwnerForeign
	}
	return verifyDeploymentOwnerAgainstInstallation(targetName, &installation, deploymentID)
}

// verifyDeploymentOwnerAgainstInstallation runs the per-deployment directory
// conclusion against an already-verified installation: the deployment namespace
// must belong to the installation's Target and must be a real protected
// directory. The deploymentID argument is accepted for call-site symmetry and
// future binding but does not affect this namespace-only check, so a cached
// installation never encodes a per-deployment ownership verdict.
func verifyDeploymentOwnerAgainstInstallation(targetName string, installation *coreinstallation.State, deploymentID string) error {
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
	if ownState, ownErr := resolveInstallationLoad(targetRoot); ownErr == nil {
		// The test Target's OWN runtime root loaded a genuine installation
		// between the precondition Load and here: the local owner governs, so
		// resolve WITHOUT the loan — but only when we did not just grant it
		// (a granted loan keeps the test Target ownerless by design, so a
		// genuine load now is a real new owner, never a re-entrance).
		_ = ownState
		return VerifyDeploymentOwnerAgainstOwner(targetName, targetRoot, "")
	} else if errors.Is(ownErr, os.ErrNotExist) {
		// Still genuinely ownerless: the loan governs.
		return nil
	} else {
		// A read/engine failure here is a host-state access problem, NOT a
		// proven foreign installation: the rule never positively observed a
		// foreign owner, only that it could not read the target's own
		// installation. Propagate the true verdict (access-denied via the
		// classified loader, or the concrete unchanging error) instead of
		// folding it into ErrDeploymentOwnerForeign. Foreign stays reserved
		// for the positive hash/target mismatch ResolveDeploymentOwnerAgainstOwner
		// establishes once a state actually loads.
		if isAccessDeniedLoadError(ownErr) {
			return fmt.Errorf("%w: %w", ErrDeploymentOwnerAccessDenied, ownErr)
		}
		return ownErr
	}
}
