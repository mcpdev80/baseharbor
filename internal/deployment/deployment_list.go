package deployment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deploymentID := entry.Name()
		path := filepath.Join(root, deploymentID, "deployment.json")
		if err := verifyDeploymentOwnerAtSeam(target, deploymentID); err != nil {
			if bestEffort {
				warnings = append(warnings, &DeploymentRecordStateError{Identity: DeploymentIdentity{Target: target, DeploymentID: deploymentID}, Kind: "missing", Err: err})
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
