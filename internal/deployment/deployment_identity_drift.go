package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// DeleteDeploymentRecord removes the deployment directory that owns the given
// identity. Before the destructive step it re-reads the record on disk and
// validates that the identity stored there is exactly the one being deleted: a
// same-name record replacement between authorization and deletion (or any change
// to the record that lives at this path) is a drift. The deletion fails closed
// and leaves every byte intact, so a drifted replacement is never removed on
// the strength of a stale in-memory copy. A read, parse, version or identity
// mismatch all fail closed.
func DeleteDeploymentRecord(id DeploymentIdentity) error {
	root, err := DeploymentRoot(id)
	if err != nil {
		return err
	}
	if err := assertOnDiskIdentityMatches(root, id); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

// assertOnDiskIdentityMatches re-reads the deployment.json at root and verifies
// it still carries exactly the given deployment identity before the destructive
// delete. A missing file is not a drift: the record was already removed (idempotent
// destroy, orphan adoption), so the caller may proceed. A present record whose
// identity no longer matches (a same-name replacement between authorization and
// deletion) fails closed with IDENTITY_DRIFT; a malformed record also fails
// closed rather than removing a record that is not the authorized one.
func assertOnDiskIdentityMatches(root string, id DeploymentIdentity) error {
	data, err := os.ReadFile(filepath.Join(root, "deployment.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // already gone: idempotent removal, not a drift
		}
		return fmt.Errorf("read deployment record before delete %s: %w", root, err)
	}
	var record DeploymentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return &machine.Error{Code: machine.ErrorInternal, CauseCode: "DEPLOYMENT_RECORD_MALFORMED", Message: fmt.Sprintf("deployment record %s is not a valid deployment record: %v", root, err), Resource: root, Next: "Re-authorize the current record; do not remove a record whose storage is corrupt.", Remediation: "operator_review"}
	}
	if record.Version != DeploymentRecordVersion {
		return &machine.Error{Code: machine.ErrorInternal, CauseCode: "DEPLOYMENT_RECORD_VERSION", Message: fmt.Sprintf("deployment record %s has version %d, expected %d", root, record.Version, DeploymentRecordVersion), Resource: root, Next: "Re-authorize the current record; do not remove a record whose storage version is unsupported.", Remediation: "operator_review"}
	}
	if record.Identity != id {
		return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "IDENTITY_DRIFT", Message: fmt.Sprintf("deployment record %s identity %v drifted from authorized %v", root, record.Identity, id), Resource: root, Next: "The deployment record changed between authorization and deletion; re-run to authorize the current record.", Remediation: "operator_review"}
	}
	return nil
}
