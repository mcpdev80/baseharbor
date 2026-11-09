package deployment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// listAllDeployments walks every target under the data root and returns the
// deployment records for each. bestEffort turns a per-target listing error
// into a warning instead of aborting the whole walk (used by the display paths
// that must tolerate one unreadable target).
//
// This function lives in its own translation unit (deployment_list.go), not
// deployment_store.go, so the store file stays under the 800-line readability
// ceiling enforced by scripts/source-readability-audit.sh in CI.
func listAllDeployments(bestEffort bool) ([]DeploymentRecord, []error, error) {
	root, err := DataRoot()
	if err != nil {
		return nil, nil, err
	}
	targetsRoot := filepath.Join(root, "targets")
	entries, err := os.ReadDir(targetsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		items, itemWarnings, err := listDeploymentRecords(filepath.Join(targetsRoot, entry.Name(), "deployments"), entry.Name(), bestEffort)
		if err != nil {
			if bestEffort {
				warnings = append(warnings, fmt.Errorf("target %s: %w", entry.Name(), err))
				continue
			}
			return nil, nil, err
		}
		records = append(records, items...)
		warnings = append(warnings, itemWarnings...)
	}
	sortDeploymentRecords(records)
	return records, warnings, nil
}

// listDeploymentRecords reads the deployment.json in every deployment directory
// under root, verifying each directory against the Target's Core installation
// before its record is read. Fail-closed on an unavailable owner, unless a
// best-effort listing is requested, in which case the mismatch surfaces as a
// per-target warning rather than aborting the whole list.
func listDeploymentRecords(root, target string, bestEffort bool) ([]DeploymentRecord, []error, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	// Deployment directories are the only registry entries; in-progress
	// registrations build their record at "deployment.json.<pid>.tmp" and
	// rename it atomically, so those temp siblings are skipped (0427-175).
	// A directory whose record file is not durable yet is reported as an
	// incomplete-state warning, never silently skipped: it is either a
	// half-finished registration or an unregistered state directory, and
	// discovery must disclose both (0427-144/160 precedent).
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		deploymentID := entry.Name()
		// Canonical directory-name invariant (bug-hunt BF-03): a deployment
		// directory is a per-deploy UUIDv4. A directory whose name is not a
		// valid UUIDv4 can never be a deployment — the registry path re-derives
		// fresh UUIDs (`NewDeploymentIdentity`/`ValidateDeploymentID`), so a
		// malformed name is not a truncated deployment and must NOT be advertised
		// as deployment damage. Gate on the shape BEFORE classifying so a
		// non-UUID entry (a foreign app's directory, a stray folder, etc.) is
		// discarded rather than mistaken for a damaged deployment. The
		// damaged-signature / artifact logic below is unchanged for a valid UUID.
		if err := ValidateDeploymentID(deploymentID); err != nil {
			continue
		}
		path := filepath.Join(root, deploymentID, "deployment.json")
		// Damaged deployment state precedes every downstream check: a retained
		// deploy directory without its authoritative deployment.json is the
		// signature from bug-hunt BF-03. A damaged directory is already broken,
		// so the one-installation-owner check must not mask it with a
		// "no authoritative installation owner" result — the precise,
		// recoverable damaged-state error wins. Listing it as a plain
		// "incomplete" warning would let the caller fall through to a generic
		// repository inspection (the "file limit exceeded" path) instead of the
		// real condition.
		if damaged, damageErr := DamagedDirectoryState(filepath.Join(root, deploymentID)); damageErr == nil && damaged {
			if bestEffort {
				warnings = append(warnings, &DamagedDeploymentError{Identity: DeploymentIdentity{Target: target, DeploymentID: deploymentID}, Root: filepath.Join(root, deploymentID)})
				continue
			}
			return nil, nil, &DamagedDeploymentError{Identity: DeploymentIdentity{Target: target, DeploymentID: deploymentID}, Root: filepath.Join(root, deploymentID)}
		}
		if err := verifyDeploymentOwnerAtSeam(target, deploymentID); err != nil {
			if bestEffort {
				// A non-missing owner error (an unreadable installation, or a
				// present-but-malformed Core state) is a host-state problem, not a
				// "missing" record: folding it into the generic missing-owner kind
				// would let the CLI path (resolveRegisteredApplication) re-classify
				// it as "no registered deployment". Keep it distinct so the CLI
				// can propagate the true access-denied / malformed-state verdict
				// instead of the lie.
				kind := classifyOwnerWarning(err)
				warnings = append(warnings, &DeploymentRecordStateError{Identity: DeploymentIdentity{Target: target, DeploymentID: deploymentID}, Kind: kind, Err: err})
				continue
			}
			return nil, nil, err
		}
		record, err := loadDeploymentRecordFile(target, deploymentID, path, nil)
		if err != nil {
			if bestEffort {
				warnings = append(warnings, err)
				continue
			}
			return nil, nil, err
		}
		records = append(records, record)
	}
	sortDeploymentRecords(records)
	return records, warnings, nil
}

// ci-checkout-forcing-bumper

// TestOnlyRefuseApplicationIDCollisions exposes the shared conflict detector
// to consumer-package regressions (0427-175). It is the exact function the
// inventory seam runs, so the order-independence of detection is pinned
// against the production rule, not a copy.
func TestOnlyRefuseApplicationIDCollisions(target string, records []DeploymentRecord) error {
	return refuseApplicationIDCollisions(target, records)
}
