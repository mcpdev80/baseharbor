package deployment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// registryLockFile is the Target-scoped deployment-registry write lock inside
// the Target state root. First registrations of ANY application take it for
// the whole claim → scan → create sequence, which is what makes a concurrent
// double registration structurally impossible rather than merely unlikely
// (0427-175, bug hunt hp4 BF-01). A dedicated file keeps the registry writer
// out of the .installation.lock ownership contract while using the same
// kernel discipline (LOCK_EX|LOCK_NB, holder PID inside, auto-release on exit).
const registryLockFile = ".registry.lock"

// ErrRegistryConcurrentWrite reports that another process currently holds the
// Target's registry write lock. The mutation is refused, never queued or
// silently retried: registration is a rare, cheap command to re-run, and a
// blocking wait here would reintroduce the very lock-ordering hazard (0427-149)
// that the non-blocking lock avoids.
var ErrRegistryConcurrentWrite = errors.New("another registration is in progress on this target")

// WithDeploymentRegistryWriteLock runs fn while holding the Target's registry
// write lock. It is ONLY taken for first registrations (see SaveDeploymentRecord),
// never for an idempotent re-save of an already-claimed identity, so the lock
// graph of every running lifecycle command stays exactly as it was.
//
// The lock lives inside the Target state root, so it never creates state by
// itself: when the root does not exist there is no registry to race over yet
// (the write seam's owner check below fails closed exactly like before), and
// fn runs directly. That keeps the lock free of any MkdirAll side effect on
// Targets that merely do not exist yet.
func WithDeploymentRegistryWriteLock(target string, fn func() error) error {
	root, err := TargetStateRoot(target)
	if err != nil {
		return err
	}
	if info, statErr := os.Lstat(root); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return fn()
		}
		return statErr
	} else if !info.IsDir() {
		return fn()
	}
	release, err := stableid.AcquireRegistryLock(root, registryLockFile)
	if err != nil {
		if IsRegistryConcurrentWriter(err) {
			return fmt.Errorf("%w: %s", ErrRegistryConcurrentWrite, err)
		}
		return err
	}
	defer release()
	return fn()
}

// DeploymentClaimExists reports whether the identity's deployment path already
// exists (with the deployment.json path), i.e. whether a previous save of
// THIS deployment uuid claimed it. It is the pure existence probe the
// registration-split in SaveDeploymentRecord needs; a path error (an invalid
// identity or an unreadable tree) fails closed.
func DeploymentClaimExists(id DeploymentIdentity) (bool, string, error) {
	root, err := DeploymentRoot(id)
	if err != nil {
		return false, "", err
	}
	path := filepath.Join(root, "deployment.json")
	if _, statErr := os.Stat(path); statErr == nil {
		return true, path, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return false, "", statErr
	}
	if _, statErr := os.Stat(root); statErr == nil {
		return true, path, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return false, "", statErr
	}
	return false, path, nil
}
