package deployment

import (
	"errors"
	"fmt"
	"os"
)

// ErrDuplicateApplicationName is returned when a brand-new deployment tries to
// register the same (target, application name, environment) triple as a
// DIFFERENT deployment directory. A record update of the SAME identity
// (its target directory already exists) is not a duplicate and never trips this.
var ErrDuplicateApplicationName = errors.New("duplicate application name")

// refuseDuplicateApplicationName closes the silent duplicate-registration gap
// (0427-149): before writing a deployment record for the first time, it scans
// the target's deployment registry for any OTHER directory (different uuid)
// that already claims the identical (target, application name, environment)
// triple. A match fails closed with a message that names the existing
// deployment so the operator has an actionable uuid/-e disambiguation instead
// of a later opaque "application machine: denied" on edit.
//
// Updates to an already-persisted record of the SAME identity are not
// collisions: the deployment directory already exists, so a scan is
// unnecessary and an idempotent retry of a failed apply must not re-refuse.
//
// The duplicate is defined by the user-visible identity: the same
// (target, application NAME, environment) triple claimed by a DIFFERENT
// deployment uuid. The opaque system application_id is an internal key that a
// legitimate rename keeps unchanged while the display name moves, so the guard
// keys on the name, never the application_id. The environment filter runs
// first so a benign same-name/different-environment pair is never misread as a
// collision.
func refuseDuplicateApplicationName(record DeploymentRecord) error {
	existingDir, dirErr := DeploymentRoot(record.Identity)
	if dirErr == nil {
		if _, statErr := os.Stat(existingDir); statErr == nil {
			// The deployment directory already exists: this is an update/re-save
			// of the SAME identity, not a first registration. Never a collision.
			return nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect deployment directory for duplicate name check: %w", statErr)
		}
	}
	records, err := ListDeployments(record.Identity.Target)
	if err != nil {
		return err
	}
	for i := range records {
		existing := records[i]
		if existing.Identity.DeploymentID == record.Identity.DeploymentID {
			continue
		}
		if existing.Identity.Target != record.Identity.Target ||
			existing.Identity.Environment != record.Identity.Environment {
			continue
		}
		if existing.Identity.Application == record.Identity.Application {
			return &DeploymentRecordStateError{
				Identity: record.Identity,
				Kind:     "duplicate",
				Err: fmt.Errorf("%w: %s/%s/%s already registered as deployment %s; reconcile the two identities (edit by uuid or -e/--environment) before registering another",
					ErrDuplicateApplicationName,
					existing.Identity.Target, existing.Identity.Application, existing.Identity.Environment,
					existing.Identity.DeploymentID),
			}
		}
	}
	return nil
}
