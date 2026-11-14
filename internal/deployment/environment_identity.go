package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	if name == "" {
		return "", "", fmt.Errorf("environment name is empty")
	}
	targetRoot, err := TargetStateRoot(target)
	if err != nil {
		return "", "", err
	}
	home := filepath.Join(targetRoot, ".baseharbor", "environments", name)
	return home, filepath.Join(home, environmentRecordFile), nil
}

// readEnvironmentRecord loads and validates a persisted identity record. It is
// a pure read: no allocation, no mutation.
func readEnvironmentRecord(path string) (EnvironmentRecord, error) {
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
	return record, nil
}

// persistEnvironmentRecord atomically writes a freshly created identity. The
// rename is the atomic seam: a concurrent writer that wrote a DIFFERENT record
// first is detected here and refused, so two first registrations can never
// leave two divergent identities behind (state split, precedent 0427).
func persistEnvironmentRecord(home, path, target string, id string) (EnvironmentRecord, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("create environment state dir: %w", err)
	}
	data, err := json.MarshalIndent(EnvironmentRecord{Target: target, Name: id, Identity: id}, "", "  ")
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
	// refuse rather than silently overwrite the historical identity.
	if existing, statErr := os.Stat(path); statErr == nil && !existing.IsDir() {
		if other, rErr := readEnvironmentRecord(path); rErr == nil && other.Identity != id {
			return EnvironmentRecord{}, fmt.Errorf("%w: %q", ErrEnvironmentAlreadyExists, other.Identity)
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return EnvironmentRecord{}, fmt.Errorf("persist environment identity: %w", err)
	}
	return EnvironmentRecord{Target: target, Name: strings.TrimSpace(id), Identity: id}, nil
}

// GetEnvironment loads the durable identity of a (Target, environment) pair.
//
// It is idempotent and brownfield-safe: when the identity does not yet exist it
// atomically creates one (fresh UUID, validated name) and persists it; when it
// already exists it returns the historical ID unchanged. Renames never reach
// this seam with a new ID, so the identity is stable across renames.
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
		return readEnvironmentRecord(path)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return EnvironmentRecord{}, fmt.Errorf("inspect environment identity: %w", statErr)
	}

	// First creation: allocate, validate the name, then claim the identity
	// atomically under the registry write lock.
	id, err := environment.New(strings.TrimSpace(environmentName))
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
		created, err = createEnvironmentUnderLock(target, home, path, id.ID)
		if err == nil {
			return created, nil
		}
		switch {
		case errors.Is(err, ErrRegistryConcurrentWrite):
			continue
		case errors.Is(err, ErrEnvironmentAlreadyExists):
			// A concurrent writer won. Read and return its identity so the
			// caller observes the single converged ID, not an empty record.
			return readEnvironmentRecord(path)
		default:
			return EnvironmentRecord{}, err
		}
	}
	// Exhaustion means we could never take the lock; fall back to reading the
	// record that a concurrent writer must have persisted, so the caller still
	// observes the single converged identity rather than an empty one.
	return readEnvironmentRecord(path)
}

// createEnvironmentUnderLock takes the registry write lock, re-checks, and
// creates or observes the identity. It returns ErrRegistryConcurrentWrite when
// another first-registration holds the lock, or ErrEnvironmentAlreadyExists
// when a concurrent writer already created a DIFFERENT identity.
func createEnvironmentUnderLock(target, home, path string, id string) (EnvironmentRecord, error) {
	var created EnvironmentRecord
	err := WithDeploymentRegistryWriteLock(target, func() error {
		// Re-check inside the lock: a concurrent writer may have created it.
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			existing, rErr := readEnvironmentRecord(path)
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
		created, persistErr = persistEnvironmentRecord(home, path, target, id)
		return persistErr
	})
	return created, err
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
