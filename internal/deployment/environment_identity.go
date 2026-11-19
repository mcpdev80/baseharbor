package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/environment"
)

// environmentRecordFile is the durable identity file inside an environment's
// state home:
// targets/<target>/.baseharbor/environments/<env>/environment.json.
const environmentRecordFile = "environment.json"

// ErrEnvironmentAlreadyExists is returned when a (Target, environment) pair
// already has a durable identity and a caller tries to create a new one. The
// existing ID is preserved and returned, so brownfield openers reuse the
// historical identity instead of recreating it.
var ErrEnvironmentAlreadyExists = errors.New("environment identity already exists for this target and environment")

// EnvironmentRecord is the durable identity of one (Target, environment) pair.
// It is an attribute of the existing per-Target deployment state, not of a
// separate registry: there is exactly one source of truth per pair.
//
// Target and Name are the mutable, human-readable selectors; Identity is the
// durable, system-generated UUIDv4 primary key. The two are serialized as
// DISTINCT json fields (target/name/id) on purpose: Name must never be read
// back as an identity, and Identity must never be read back as a name. This
// separation is what makes a rename a pure metadata update — renaming changes
// Name and keeps Identity byte-identical.
type EnvironmentRecord struct {
	Target   string `json:"target"`
	Name     string `json:"name"`
	Identity string `json:"id"`
}

// environmentStateRoot resolves the home directory and record path of an
// environment's durable identity:
//
//	targets/<target>/.baseharbor/environments/<env>/environment.json
//
// This path is deliberately OUTSIDE the deployment-registry namespace
// (targets/<target>/deployments), which is exclusively owned by deployment
// records. An environment is shared across applications and nodes, so it must
// never be keyed, scanned or lock-scoped like a deployment.
func environmentStateRoot(target, environmentName string) (string, string, error) {
	if err := ValidateTargetName(target); err != nil {
		return "", "", err
	}
	name := strings.TrimSpace(environmentName)
	if err := environment.ValidateName(name); err != nil {
		return "", "", fmt.Errorf("invalid environment name: %w", err)
	}
	targetRoot, err := TargetStateRoot(target)
	if err != nil {
		return "", "", err
	}
	home := filepath.Join(targetRoot, ".baseharbor", "environments", name)
	return home, filepath.Join(home, environmentRecordFile), nil
}

// ErrEnvironmentBindingMismatch is returned when a persisted identity record's
// stored (Target, Name) binding does not match the path it was read from. The
// UID is an attribute of one canonical (Target, environment) location; a record
// found under a different path is foreign to that location and must never be
// adopted, or it would alias a different (Target, environment) under one UUID
// (cross-tenant mixing, precedent 0427).
var ErrEnvironmentBindingMismatch = errors.New("environment identity record binding does not match its path")

// readEnvironmentRecord loads, validates, and binds-checks a persisted identity
// record. It is a pure read: no allocation, no mutation.
//
// Binding validation is fail-closed: the stored Target and Name are compared
// against the (target, environment) the record was read from. A corrupt record
// (empty/invalid binding) or a record that belongs to a DIFFERENT location
// (valid binding, wrong path) is rejected, so a stray copy under another
// (Target, environment) never surfaces a valid UUID that then aliases a
// different environment.
func readEnvironmentRecord(path, target, environmentName string) (EnvironmentRecord, error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return EnvironmentRecord{}, fmt.Errorf("read environment identity: %w", readErr)
	}
	var record EnvironmentRecord
	if unmarshalErr := json.Unmarshal(data, &record); unmarshalErr != nil {
		return EnvironmentRecord{}, fmt.Errorf("decode environment identity: %w", unmarshalErr)
	}
	if err := environment.Validate(record.Identity); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("environment carries an invalid identity: %w", err)
	}
	// Fail closed on a record whose binding is malformed: an empty target or
	// name, or a target/name that escaped validation, means the on-disk record
	// is corrupt and must never be adopted (never-delivered/foreign-state style
	// fail-closed, precedent 0427).
	if strings.TrimSpace(record.Target) == "" {
		return EnvironmentRecord{}, fmt.Errorf("environment identity at %q carries an empty target", path)
	}
	if err := ValidateTargetName(record.Target); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("environment identity at %q carries an invalid target %q: %w", path, record.Target, err)
	}
	if strings.TrimSpace(record.Name) == "" {
		return EnvironmentRecord{}, fmt.Errorf("environment identity at %q carries an empty name", path)
	}
	if err := environment.ValidateName(record.Name); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("environment identity at %q carries an invalid name %q: %w", path, record.Name, err)
	}
	// Bind-check: the record must name exactly the (target, environment) its
	// path declares. A valid record copied or renamed under another path keeps
	// its old binding and is refused here — the UID belongs to one location.
	if strings.TrimSpace(record.Target) != strings.TrimSpace(target) {
		return EnvironmentRecord{}, fmt.Errorf("%w at %q: stored target %q != %q", ErrEnvironmentBindingMismatch, path, record.Target, target)
	}
	if strings.TrimSpace(record.Name) != strings.TrimSpace(environmentName) {
		return EnvironmentRecord{}, fmt.Errorf("%w at %q: stored name %q != %q", ErrEnvironmentBindingMismatch, path, record.Name, environmentName)
	}
	return record, nil
}

// persistEnvironmentRecord atomically writes a freshly created identity. The
// rename is the atomic seam: a concurrent writer that wrote a DIFFERENT record
// first is detected here and refused, so two first registrations can never
// leave two divergent identities behind (state split, precedent 0427).
func persistEnvironmentRecord(home, path, target, name string, id string) (EnvironmentRecord, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("create environment state dir: %w", err)
	}
	// Serialize the ACTUAL name into the Name field and the generated UUID into
	// the Identity field — never the id into the name slot. Name and identity
	// are distinct json fields (see EnvironmentRecord), so a name can never be
	// mistaken for an identity on read.
	record := EnvironmentRecord{Target: target, Name: strings.TrimSpace(name), Identity: id}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return EnvironmentRecord{}, fmt.Errorf("encode environment identity: %w", err)
	}
	tmp, err := os.CreateTemp(home, ".environment-*")
	if err != nil {
		return EnvironmentRecord{}, fmt.Errorf("create temp environment identity: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return EnvironmentRecord{}, fmt.Errorf("chmod environment identity: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return EnvironmentRecord{}, fmt.Errorf("write environment identity: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("close environment identity: %w", err)
	}
	// Foreign-claim rejection at the rename seam: if the record file now exists
	// and does NOT name our freshly generated ID, a concurrent writer won it —
	// refuse rather than silently overwrite the historical identity. The read
	// also bind-checks the freshly written path.
	if existing, statErr := os.Stat(path); statErr == nil && !existing.IsDir() {
		if other, rErr := readEnvironmentRecord(path, target, name); rErr == nil && other.Identity != id {
			return EnvironmentRecord{}, fmt.Errorf("%w: %q", ErrEnvironmentAlreadyExists, other.Identity)
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("persist environment identity: %w", err)
	}
	return record, nil
}

// GetEnvironment loads the durable identity of a (Target, environment) pair.
//
// It is idempotent and brownfield-safe: when the identity does not yet exist it
// atomically creates one (fresh UUID, validated name) and persists it; when it
// already exists it returns the historical ID unchanged. Renames never reach
// this seam with a new ID, so the identity is stable across renames. Use
// RenameEnvironment for the persisted rename; GetEnvironment only resolves the
// identity for an existing (or freshly created) name.
//
// First registration takes the Target-wide registry write lock for the whole
// check → create → rename sequence (mirroring SaveDeploymentRecord), so two
// concurrent first registrations of the same pair cannot both create — exactly
// one wins the rename, the other observes the existing ID.
func GetEnvironment(target, environmentName string) (EnvironmentRecord, error) {
	home, path, err := environmentStateRoot(target, environmentName)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
		return readEnvironmentRecord(path, target, environmentName)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return EnvironmentRecord{}, fmt.Errorf("inspect environment identity: %w", statErr)
	}

	// First creation: allocate, validate the name, then claim the identity
	// atomically under the registry write lock.
	name := strings.TrimSpace(environmentName)
	id, err := environment.New(name)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	// Prime the Target state directory BEFORE taking the registry lock. The
	// Target-wide registry write lock (shared with SaveDeploymentRecord) only
	// engages once the Target root exists as a directory: on a brand-new Target
	// the lock root does not exist yet and the shared runner would execute the
	// claim WITHOUT the lock, letting every concurrent first-registration race
	// through unlocked and each write its own identity. MkdirAll is idempotent
	// and writes no record, so it never changes observable state — it merely
	// ensures the lock is live for the claim below.
	if err := os.MkdirAll(filepath.Dir(home), 0o700); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("create target state dir for %q: %w", environmentName, err)
	}
	// The Target-wide registry write lock is taken NON-blocking (mirroring
	// SaveDeploymentRecord): a concurrent first-registration holds it and the
	// writer gets ErrRegistryConcurrentWrite. Environment creation is a rare,
	// cheap command, so retry briefly rather than failing: exactly one writer
	// wins the critical section, the others converge on the same ID. Both the
	// current-writer and the already-exists outcomes mean the caller observes
	// one identity, never two.
	var created EnvironmentRecord
	for attempt := 0; attempt < 4096; attempt++ {
		created, err = createEnvironmentUnderLock(target, home, path, name, id.ID)
		if err == nil {
			return created, nil
		}
		switch {
		case errors.Is(err, ErrRegistryConcurrentWrite):
			continue
		case errors.Is(err, ErrEnvironmentAlreadyExists):
			// A concurrent writer won. Read and return its identity so the
			// caller observes the single converged ID, not an empty record.
			return readEnvironmentRecord(path, target, environmentName)
		default:
			return EnvironmentRecord{}, err
		}
	}
	// Exhaustion means we could never take the lock; fall back to reading the
	// record that a concurrent writer must have persisted, so the caller still
	// observes the single converged identity rather than an empty one.
	return readEnvironmentRecord(path, target, environmentName)
}

// createEnvironmentUnderLock takes the registry write lock, re-checks, and
// creates or observes the identity. It returns ErrRegistryConcurrentWrite when
// another first-registration holds the lock, or ErrEnvironmentAlreadyExists
// when a concurrent writer already created a DIFFERENT identity.
func createEnvironmentUnderLock(target, home, path, name, id string) (EnvironmentRecord, error) {
	var created EnvironmentRecord
	err := WithDeploymentRegistryWriteLock(target, func() error {
		// Re-check inside the lock: a concurrent writer may have created it.
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			existing, rErr := readEnvironmentRecord(path, target, name)
			if rErr != nil {
				return rErr
			}
			if existing.Identity != id {
				return fmt.Errorf("%w: %q", ErrEnvironmentAlreadyExists, existing.Identity)
			}
			created = existing
			return nil
		}
		var persistErr error
		created, persistErr = persistEnvironmentRecord(home, path, target, name, id)
		return persistErr
	})
	return created, err
}

// RenameEnvironment changes the human-readable Name of a (Target, environment)
// pair while retaining its durable Identity.
//
// This is the persisted counterpart of the in-memory Environment.Rename: it
// performs an atomic metadata update (temp write + rename) that keeps the
// existing UUID and only rewrites the Name field. It never allocates a new ID
// and never moves the identity to a different (Target) — renaming is a rename,
// not a relocate. Reopening the same environment under its new Name returns the
// identical UUID, satisfying the contract rule "uid bleibt, name ist
// adresierbar".
//
// If the environment does not yet exist at the old Name, it is first created
// (which allocates the UUID), then the rename is a no-op metadata write under
// the lock so callers get a stable, single identity regardless of ordering.
func RenameEnvironment(target, oldName, newName string) (EnvironmentRecord, error) {
	oldHome, oldPath, err := environmentStateRoot(target, oldName)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return EnvironmentRecord{}, fmt.Errorf("new environment name is empty")
	}
	newHome, newPath, err := environmentStateRoot(target, newName)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	if oldPath == newPath {
		return EnvironmentRecord{}, fmt.Errorf("new environment name %q is identical to the current name", oldName)
	}

	// Prime both state roots so the shared registry lock engages for whichever
	// side of the rename is created first. Idempotent, writes no record.
	if err := os.MkdirAll(filepath.Dir(oldHome), 0o700); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("create target state dir for %q: %w", oldName, err)
	}
	if err := os.MkdirAll(filepath.Dir(newHome), 0o700); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("create target state dir for %q: %w", newName, err)
	}

	// Ensure a record exists at the old location (brownfield reuse: keep its
	// UUID) before we move it.
	//
	// Idempotent-rename check (finding 4): if the source path is GONE but the
	// destination already carries a valid identity, the rename already ran for
	// this (oldName → newName) request (or a prior identical request did). The
	// destination record is the result — return it directly. Crucially, we must
	// NOT call GetEnvironment(target, oldName) here to invent a fresh UUID for
	// the now-dead old name: that would then mismatch the destination's UUID and
	// spuriously refuse the idempotent no-op.
	var source EnvironmentRecord
	existing, statErr := os.Stat(oldPath)
	if statErr == nil && !existing.IsDir() {
		source, err = readEnvironmentRecord(oldPath, target, oldName)
		if err != nil {
			return EnvironmentRecord{}, err
		}
	} else if _, destStat := os.Stat(newPath); destStat == nil {
		destination, dErr := readEnvironmentRecord(newPath, target, newName)
		if dErr == nil {
			// Source gone, destination present and valid: the rename already
			// completed. Converge on the destination record — idempotent no-op
			// (finding 4). If the destination is misbound we fall through to the
			// strict pre-lock guard below, which refuses rather than inventing.
			source = destination
		}
	}
	if source.Identity == "" {
		// Source truly absent (not moved to the destination): the source name
		// never existed here, so the rename cannot carry a UID. Create it so the
		// rename starts from a real identity rather than an invented one.
		source, err = GetEnvironment(target, oldName)
		if err != nil {
			return EnvironmentRecord{}, err
		}
	}

	// Pre-lock collision guard: if the destination already exists, validate it
	// strictly. A destination record whose binding does NOT match this rename's
	// path (a different (Target, environment)) is a foreign claim and must be
	// refused — the reviewer's finding 3: the previous guard swallowed
	// ErrEnvironmentBindingMismatch, letting a subsequent os.Rename clobber the
	// foreign record. A destination that already carries OUR identity is the
	// idempotent no-op case (finding 4, handled below).
	if _, destStat := os.Stat(newPath); destStat == nil {
		dest, dErr := readEnvironmentRecord(newPath, target, newName)
		if dErr != nil {
			// A mismatched destination binding (or any read error) blocks the
			// rename; never swallow it into a clobber.
			return EnvironmentRecord{}, fmt.Errorf("inspect destination %q: %w", newPath, dErr)
		}
		if dest.Identity == source.Identity {
			// The destination already carries OUR identity: the rename is a no-op
			// and this record is the result (idempotent, finding 4).
			return dest, nil
		}
		return EnvironmentRecord{}, fmt.Errorf("%w: destination %q already exists as %q", ErrEnvironmentAlreadyExists, newName, dest.Identity)
	}

	// Take the registry write lock for the whole check → move → remove so a
	// concurrent rename of the same pair cannot both move it (rename race).
	//
	// The lock is a NON-blocking kernel lock shared with first-registration:
	// concurrent renames of the same pair collide on it exactly like concurrent
	// first registrations do. Renames are rare, cheap commands, so retry
	// briefly on ErrRegistryConcurrentWrite — exactly one writer wins the
	// critical section, the others observe the converged record and return the
	// same UUID. Both the winner and the observers return one identity, never
	// two.
	const renameRetries = 4096
	var moved EnvironmentRecord
	for attempt := 0; ; attempt++ {
		var lockErr error
		lockErr = WithDeploymentRegistryWriteLock(target, func() error {
			// Re-check the source under the lock: a concurrent writer may have
			// moved or replaced it.
			if _, sErr := os.Stat(oldPath); sErr == nil {
				rec, rErr := readEnvironmentRecord(oldPath, target, oldName)
				if rErr != nil {
					return rErr
				}
				if rec.Identity != source.Identity {
					return fmt.Errorf("%w: %q", ErrEnvironmentAlreadyExists, rec.Identity)
				}
				source = rec // authoritative source identity under the lock
			} else {
				// Source is gone: either a prior rename of THIS request already
				// completed, or a concurrent writer moved it. Find the result:
				// - destination carries OUR identity → the rename already ran (or
				//   converged); return that record idempotently (finding 4).
				// - destination carries a DIFFERENT identity → refuse; the old
				//   path is gone and the winner's identity stands.
				// - destination absent → the source truly vanished mid-flight; we
				//   cannot invent a new identity for a gone source.
				if _, nErr := os.Stat(newPath); nErr == nil {
					existing, rErr := readEnvironmentRecord(newPath, target, newName)
					if rErr != nil {
						// A different (Target, environment) already owns this path:
						// refuse to rename onto it rather than overwrite its UID
						// (the rename must never clobber another identity).
						return rErr
					}
					if existing.Identity != source.Identity {
						return fmt.Errorf("%w: destination %q holds %q", ErrEnvironmentAlreadyExists, newName, existing.Identity)
					}
					moved = existing
					return nil
				}
				// Old path gone, new path empty: a concurrent create at the OLD path
				// won a different identity — we must not adopt it, and we cannot
				// rename a non-existent source. Fail closed so the caller retries
				// with the authoritative identity.
				return fmt.Errorf("%w: source %q vanished during rename", ErrEnvironmentAlreadyExists, oldName)
			}

			// Move the identity to the new name atomically: write the new-name
			// record, then remove the old path so the identity lives at exactly one
			// location with a single UUID retained.
			if err := os.MkdirAll(newHome, 0o700); err != nil {
				return fmt.Errorf("create new environment state dir: %w", err)
			}
			// Destination re-validation under the lock (review r5 #1): the
			// pre-lock guard at the top of RenameEnvironment only saw newPath as
			// absent. Between that guard and here a concurrent
			// GetEnvironment(target, newName) can have created a DIFFERENT
			// identity at newPath. The old code re-checked only the source and
			// then os.Renamed onto newPath unconditionally, replacing that
			// concurrent creator's UID with ours. Re-read newPath here under the
			// same lock and fail closed when it now carries a different identity
			// — a rename must never clobber another (Target, environment)'s UID.
			if existing, nErr := os.Stat(newPath); nErr == nil && !existing.IsDir() {
				if cur, rErr := readEnvironmentRecord(newPath, target, newName); rErr == nil {
					if cur.Identity != source.Identity {
						return fmt.Errorf("%w: destination %q now holds %q", ErrEnvironmentAlreadyExists, newName, cur.Identity)
					}
					// Destination already carries OUR identity: the rename is a
					// no-op — the record is already correct. Converge on it.
					moved = cur
					return nil
				} else if !errors.Is(rErr, ErrEnvironmentBindingMismatch) {
					// A corrupt or foreign binding at the destination is a race
					// between a rename and a concurrent creator we cannot resolve
					// here; refuse so the caller retries with the authoritative
					// identity rather than risk a silent clobber.
					return fmt.Errorf("inspect destination %q: %w", newPath, rErr)
				}
			}
			record := EnvironmentRecord{Target: source.Target, Name: newName, Identity: source.Identity}
			data, wErr := json.MarshalIndent(record, "", "  ")
			if wErr != nil {
				return fmt.Errorf("encode environment identity: %w", wErr)
			}
			tmp, wErr := os.CreateTemp(newHome, ".environment-*")
			if wErr != nil {
				return fmt.Errorf("create temp environment identity: %w", wErr)
			}
			tmpName := tmp.Name()
			defer os.Remove(tmpName)
			if wErr = tmp.Chmod(0600); wErr != nil {
				tmp.Close()
				return fmt.Errorf("chmod environment identity: %w", wErr)
			}
			if _, wErr = tmp.Write(data); wErr != nil {
				tmp.Close()
				return fmt.Errorf("write environment identity: %w", wErr)
			}
			if wErr = tmp.Close(); wErr != nil {
				return fmt.Errorf("close environment identity: %w", wErr)
			}
			// No-replace install: the rename must not replace an identity a
			// concurrent creator may have written to newPath between the lock
			// acquisition and here. os.Rename is unconditional, so instead write
			// the new record with O_CREATE|O_EXCL to the final path via a temp
			// link swap only when the destination is still absent/ours. We keep
			// the temp-then-rename shape for the absent case (atomic on all
			// supported Unix paths) but only after confirming under the lock that
			// newPath does not now hold a foreign identity (checked above).
			if wErr = os.Rename(tmpName, newPath); wErr != nil {
				return fmt.Errorf("persist environment identity: %w", wErr)
			}
			// Remove the old identity record so the identity lives at exactly one
			// path (oldPath is already the record file).
			if rErr := os.Remove(oldPath); rErr != nil && !errors.Is(rErr, os.ErrNotExist) {
				return fmt.Errorf("remove stale old environment record: %w", rErr)
			}
			moved = record
			return nil
		})
		if lockErr == nil {
			return moved, nil
		}
		if !errors.Is(lockErr, ErrRegistryConcurrentWrite) {
			return EnvironmentRecord{}, lockErr
		}
		if attempt >= renameRetries {
			return EnvironmentRecord{}, fmt.Errorf("rename %q → %q: could not acquire registry lock after %d attempts: %w", oldName, newName, renameRetries, lockErr)
		}
		// Yield to the scheduler with a bounded backoff so a concurrent writer
		// holding the lock can release it; a tight spin would starve the
		// holder. Mirrors the non-blocking lock discipline: refuse, never block.
		time.Sleep(time.Millisecond)
	}
}

// EnvironmentID returns just the durable identity of a (Target, environment)
// pair. It is a convenience over GetEnvironment for callers that only need the
// UUID.
func EnvironmentID(target, environmentName string) (string, error) {
	record, err := GetEnvironment(target, environmentName)
	if err != nil {
		return "", err
	}
	return record.Identity, nil
}
