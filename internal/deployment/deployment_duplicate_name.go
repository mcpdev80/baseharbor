package deployment

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ErrDuplicateApplicationName is returned when a brand-new deployment tries to
// register the same (target, application name, environment) triple as a
// DIFFERENT deployment directory. A record update of the SAME identity
// (its target directory already exists) is not a duplicate and never trips this.
var ErrDuplicateApplicationName = errors.New("duplicate application name")

// ErrDuplicateApplicationID is returned when a deployment registration would
// claim an (application_id, environment) pair on a Target that a DIFFERENT
// deployment directory already owns, or when an operator command reports such
// an existing registry poison. The application_id is the system identity
// (ADR 0017): exactly one deployment per Target and environment may carry it.
// Two records that share it poison the Target permanently — every consumer
// inventory read fails closed for EVERY application on the Target — so the
// double claim is refused at the write seam and reported at the read seam
// (bug hunt hp4 BF-01, card 0427-175).
var ErrDuplicateApplicationID = errors.New("duplicate application identity")

// ErrRegistryConflict is the sentinel for a Target deployment registry that
// ALREADY holds more than one record claiming the same (application_id,
// environment) pair — the poison a racing double registration used to
// create. The inventory seam (ListDeployments) resolves through it so every
// planning path fails with one clear, structured, repairable error instead
// of permanent silent damage (0427-175).
var ErrRegistryConflict = errors.New("deployment registry conflict")

// registryCollisionForID reports the foreign deployment ids of target that
// already claim the same (application_id, environment) pair as id. It is the
// single authority behind the write-seam guard, the poison detector and the
// repair path, so all three always agree on what a collision is.
func registryCollisionForID(target string, id DeploymentIdentity) ([]string, error) {
	records, err := ListDeployments(target)
	if err != nil {
		return nil, err
	}
	return collisionDeploymentIDs(records, target, id), nil
}

func collisionDeploymentIDs(records []DeploymentRecord, target string, id DeploymentIdentity) []string {
	var collisions []string
	for i := range records {
		existing := records[i]
		if existing.Identity.DeploymentID == id.DeploymentID {
			continue
		}
		if existing.Identity.Target != target ||
			existing.Identity.ApplicationID != id.ApplicationID ||
			existing.Identity.Environment != id.Environment ||
			strings.TrimSpace(existing.Identity.ApplicationID) == "" {
			continue
		}
		collisions = append(collisions, existing.Identity.DeploymentID)
	}
	sort.Strings(collisions)
	return collisions
}

// registryBusyMessage is the flock EWOULDBLOCK rendering the stableid lock
// primitive produces for a held lock. It is included in the classification so
// a busy-registry refusal round-trips through wrapped and unwrapped shapes.
const registryBusyMessage = "resource temporarily unavailable"

// IsRegistryConcurrentWriter classifies a registry-mutation error as the
// refusal taken because another process currently owns the Target's
// deployment-registry write lock, so command paths render one canonical
// actionable message for it.
func IsRegistryConcurrentWriter(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRegistryConcurrentWrite) {
		return true
	}
	return strings.Contains(err.Error(), registryBusyMessage)
}

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

// refuseDuplicateApplicationIDGuard is the write-seam half of the technical
// identity uniqueness (0427-175). SaveDeploymentRecord runs it for every
// first registration while holding the Target's deployment-registry write
// lock, so the registry scan and the record creation form ONE atomic claim:
// a second concurrent registration either sees the winner's record and refuses
// with ErrDuplicateApplicationID, or loses the lock and refuses with
// ErrRegistryConcurrentWrite. A duplicate application_id can no longer be
// created by racing.
func refuseDuplicateApplicationIDGuard(record DeploymentRecord) error {
	existingDir, dirErr := DeploymentRoot(record.Identity)
	if dirErr == nil {
		if _, statErr := os.Stat(existingDir); statErr == nil {
			// An update of the SAME identity never re-refuses: the durable
			// apply path must stay idempotent (0427-149 precedent).
			return nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect deployment directory for duplicate application id check: %w", statErr)
		}
	}
	records, err := ListDeployments(record.Identity.Target)
	if err != nil {
		return err
	}
	collisions := collisionDeploymentIDs(records, record.Identity.Target, record.Identity)
	if len(collisions) > 0 {
		return &DeploymentRecordStateError{
			Identity: record.Identity,
			Kind:     "duplicate",
			Err: fmt.Errorf("%w: application_id %s environment %s on target %s is already registered as deployment %s; two deployment records claiming one application identity make every planning command on the Target fail closed, so this registration is refused before any state is written; address the existing deployment by uuid (--uuid) instead of registering a second one",
				ErrDuplicateApplicationID,
				record.Identity.ApplicationID, record.Identity.Environment, record.Identity.Target,
				strings.Join(collisions, ", ")),
		}
	}
	return nil
}

// DetectApplicationIDCollisions scans the target's deployment registry and
// returns every (application_id, environment) group that MORE THAN ONE
// deployment directory claims, with the colliding deployment ids sorted
// deterministically. It tolerates the poison it detects: the plain registry
// list aborts on a damaged entry, so detection walks the per-record display
// listing and keeps its warnings rather than going blind exactly when it is
// needed (0427-175).
func DetectApplicationIDCollisions(target string) (map[DeploymentIdentity][]string, error) {
	records, _, err := ListDeploymentsForDisplay(target)
	if err != nil {
		return nil, err
	}
	return applicationIDCollisionsIn(records, target), nil
}

func applicationIDCollisionsIn(records []DeploymentRecord, target string) map[DeploymentIdentity][]string {
	byKey := map[DeploymentIdentity][]string{}
	for i := range records {
		id := records[i].Identity
		if id.Target != target || strings.TrimSpace(id.ApplicationID) == "" {
			continue
		}
		key := DeploymentIdentity{Target: id.Target, ApplicationID: id.ApplicationID, Environment: id.Environment}
		byKey[key] = append(byKey[key], id.DeploymentID)
	}
	collisions := map[DeploymentIdentity][]string{}
	for key, ids := range byKey {
		if len(ids) > 1 {
			sort.Strings(ids)
			collisions[key] = ids
		}
	}
	return collisions
}
