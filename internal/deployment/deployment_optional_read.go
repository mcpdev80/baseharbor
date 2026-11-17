package deployment

import (
	"errors"
	"os"
)

// LoadOptionalDeploymentRecord returns os.ErrNotExist when the exact
// deployment namespace has never been created. If any namespace exists, the
// normal owner verification remains mandatory before its record is read.
// Callers may use this only when absence means "no prior realization".
func LoadOptionalDeploymentRecord(id DeploymentIdentity) (DeploymentRecord, error) {
	root, err := DeploymentRoot(id)
	if err != nil {
		return DeploymentRecord{}, err
	}
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		// Preserve the normal loader's typed incomplete-record result whenever
		// a Core owner exists. Only the ownerless, never-created case has no
		// persisted deployment state that could be attributed or disclosed.
		record, loadErr := LoadDeploymentRecord(id)
		if errors.Is(loadErr, ErrDeploymentOwnerMissing) {
			return DeploymentRecord{}, os.ErrNotExist
		}
		return record, loadErr
	} else if err != nil {
		return DeploymentRecord{}, err
	}
	return LoadDeploymentRecord(id)
}
