package deployment

import (
	"errors"
	"os"
)

// This translation unit holds the owner-error classification helpers that keep
// VerifyDeploymentOwner / VerifyDeploymentOwnerAgainstOwner / the best-effort
// list under the source-readability ceiling (800 lines) enforced by
// scripts/source-readability-audit.sh in CI. The deployment_store.go file that
// calls them stays under the cap once these helpers are extracted here.
//
// The classification is the contract of card t-20261108-bughunt-permission-lies-
// foreign-install (0427-174 r2): an unreadable Core installation is a
// host-state access problem (ErrDeploymentOwnerAccessDenied), a present-but-
// unvalidatable installation is its own named fail-closed error
// (ErrDeploymentOwnerMalformedState), and only a positively-loaded identity
// whose Target diverges is a foreign owner (ErrDeploymentOwnerForeign). None of
// these may be folded into a generic "missing" or a "not found" lie.

// classifyOwnerWarning maps an owner-verification failure to the distinct
// DeploymentRecordStateError Kind the best-effort display list should attach.
// An unreadable owner is "access-denied"; a present-but-malformed Core state is
// "malformed"; everything else (a genuinely absent installation) stays the
// generic "missing" so the CLI propagates the true verdict instead of folding
// it into a "no registered deployment" lie.
func classifyOwnerWarning(err error) string {
	switch {
	case IsDeploymentOwnerAccessDenied(err):
		return "access-denied"
	case errors.Is(err, ErrDeploymentOwnerMalformedState):
		return "malformed"
	default:
		return "missing"
	}
}

// isAccessDeniedLoadError reports whether a coreinstallation.Load failure is a
// host-state access/read problem rather than a foreign-owner condition. A
// permission denied, or any OS-level stat/open failure that is neither a
// "does not exist" nor an "already exists" signal, means the installation.json
// could not be read — an access-denied, not a foreign installation. Only the
// file loaded but was itself invalid stays foreign (ErrDeploymentOwnerForeign).
func isAccessDeniedLoadError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	// A genuine permission-denied is unambiguous.
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	// A stat/open failure on the installation file that is not a "missing" or
	// "already exists" signal is an access/read problem, never a content one.
	// A file that loaded but failed content validation is NOT access-denied;
	// that path does not reach here because coreinstallation.Load returns a
	// domain error (foreign/incompatible), not an fs error, in that case.
	var fsErr interface{ Is(error) bool }
	if errors.As(err, &fsErr) {
		// Preserve the original fs error for the caller-facing message.
		return !errors.Is(err, os.ErrNotExist)
	}
	return false
}

// isMalformedInstallationLoadError reports whether a coreinstallation.Load
// failure is a content-validation problem (the installation.json LOADED but is
// itself malformed, incomplete, or incompatible) rather than an access/read
// problem or a proven foreign-owner condition. A malformed state is its own
// fail-closed named error (ErrDeploymentOwnerMalformedState); a genuinely
// foreign identity (wrong owner/target) stays ErrDeploymentOwnerForeign.
func isMalformedInstallationLoadError(err error) bool {
	if err == nil {
		return false
	}
	// An fs signal (missing / open / permission) is not a content validation
	// failure; those are handled by the access-denied branch.
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return false
	}
	var fsErr interface{ Is(error) bool }
	if errors.As(err, &fsErr) {
		return false
	}
	// coreinstallation.Load returns a domain (fs-free) error for any content
	// validation failure. A genuinely foreign/incompatible identity and a
	// malformed/incomplete/inconsistent state are BOTH domain errors here;
	// the caller distinguishes foreignness only by a POSITIVE loaded identity
	// whose Target diverges (deploymentDirFromTarget / Spec.Target check below),
	// which this Load-failure branch never reaches for a positive load. So any
	// domain validation error reaching this point is unclassifiable-as-foreign
	// and fails closed as malformed.
	return true
}
