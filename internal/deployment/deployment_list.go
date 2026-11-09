package deployment

import (
	"encoding/json"
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

// CollectableOrphans lists the deployment records that would be stranded under
// a Target being detached: those whose installation owner is missing (an
// ownerless Target, keyed by UUID) or whose source repository no longer exists
// (reported as SOURCE=MISSING). The detach guard calls this to refuse while such
// records are attached. The record identity is untouched (the Target owner is
// not rewritten to carry an installation reference); ownership is simply
// reported so the operator can restore the Target and collect the records via
// the existing destroy contract. A clean Target (no orphans) returns nil, nil.
//
// The directory name is the deployment identity for an ownerless orphan (the
// one-installation-owner seam keys the deployments namespace by UUID), so a
// record is only collectable when its decoded DeploymentID and Target both match
// the folder name and the detached target exactly. Every malformed or mismatched
// record is excluded from the collectable set — including a record whose decoded
// DeploymentID or Target is missing or whitespace-only: a blank field cannot
// satisfy exact canonical equality with the folder/target name, so admitting it
// would advertise a record that does not prove its own identity as collectable.
// The owner-missing guard still refuses the detach (fail-closed), but no foreign
// or blank record is ever listed as collectable.
func CollectableOrphans(target string) ([]DeploymentRecord, error) {
	root, err := TargetStateRoot(target)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "deployments"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	// A populated deployment namespace that is not a real dir is a guard failure,
	// never a silent empty listing.
	deploymentDir := filepath.Join(root, "deployments")
	if info, statErr := os.Lstat(deploymentDir); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrDeploymentOwnerForeign
		}
	}
	ownerMissing := map[string]struct{}{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deploymentID := entry.Name()
		verr := VerifyDeploymentOwner(target, filepath.Join(root, "runtime"), deploymentID)
		if errors.Is(verr, ErrDeploymentOwnerMissing) {
			ownerMissing[deploymentID] = struct{}{}
		}
	}
	var orphans []DeploymentRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deploymentID := entry.Name()
		if _, missing := ownerMissing[deploymentID]; missing {
			path := filepath.Join(root, "deployments", deploymentID, "deployment.json")
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				// A missing/undecodable record cannot be attributed; leave it to
				// the operator. The owner-missing guard already refused the detach.
				continue
			}
			var record DeploymentRecord
			if decodeErr := json.Unmarshal(data, &record); decodeErr == nil {
				// The directory name IS the deployment identity for an ownerless
				// orphan: the one-installation-owner seam keys the deployments
				// namespace by UUID, so a genuine collectable orphan's embedded
				// DeploymentID must equal the folder it lives under exactly. A blank
				// or mismatched DeploymentID is a foreign or corrupt record: it does
				// not prove its identity, so it is excluded from the collectable
				// set. Fail-closed is preserved: the owner-missing guard still
				// refuses the detach, and no foreign or blank record is advertised
				// as collectable. The decoded Target must also equal the detached
				// target exactly; a blank or distinct embedded Target is likewise
				// excluded.
				if record.Identity.DeploymentID == "" || strings.TrimSpace(record.Identity.DeploymentID) != deploymentID {
					continue
				}
				if record.Identity.Target == "" || strings.TrimSpace(record.Identity.Target) != target {
					continue
				}
				orphans = append(orphans, record)
			}
		}
	}
	if len(orphans) == 0 {
		return nil, nil
	}
	sortDeploymentRecords(orphans)
	return orphans, nil
}

// ci-checkout-forcing-bumper
