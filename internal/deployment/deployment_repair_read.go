package deployment

import (
	"path/filepath"
)

// LoadDeploymentRecordForRepair reads the authoritative deployment record at
// the path derived from (target, deploymentID) and validates it with the SAME
// rules as LoadDeploymentRecord — file parse, version, delivery selection,
// identity, references, remote project and the identity-vs-storage-path
// binding. What it deliberately does NOT do is compare the record against a
// CALLER-SUPPLIED application_id.
//
// That omission is the point (0427-175): the uuid-addressed repair of a
// registry poisoned by two records claiming ONE application_id has to read the
// selected record while the other twin is still present. The caller can only
// know the shared application_id — which names both twins — so a caller-side
// comparison cannot distinguish the selected record from its sibling and would
// fail closed on the very command the conflict message advertises. The
// deployment id remains the sole authority for WHERE the record lives, and the
// storage-path check inside the loader still proves the file there carries
// exactly that target and deployment id; nothing is read from a path the
// operator did not select.
func LoadDeploymentRecordForRepair(target, deploymentID string) (DeploymentRecord, error) {
	if err := verifyDeploymentOwnerAtSeam(target, deploymentID); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: DeploymentIdentity{Target: target, DeploymentID: deploymentID}, Kind: "missing", Err: err}
	}
	root, err := DeploymentRoot(DeploymentIdentity{Target: target, DeploymentID: deploymentID})
	if err != nil {
		return DeploymentRecord{}, err
	}
	return loadDeploymentRecordFile(target, deploymentID, filepath.Join(root, "deployment.json"), nil)
}

// DeploymentRecordIdentityIs reports whether the record at the selected path is
// exactly the given identity. It is the pre-deletion question the uuid repair
// must answer from the RECORD, never from a caller-side copy (0427-175).
func DeploymentRecordIdentityIs(record DeploymentRecord, target, deploymentID string) bool {
	return record.Identity.Target == target && record.Identity.DeploymentID == deploymentID
}
