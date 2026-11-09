package stableid

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Installation lock path is the shared, provider-neutral kernel lock file used
// across a single installation state root. It lives in stableid so that the
// lock primitive is a leaf dependency: coreinstallation and serviceaccess both
// depend on it without importing each other, which keeps the dependency graph
// acyclic. The platform-specific `lock` body is provided per-OS via build tags.
const (
	lifecycleLockFile = ".installation.lock"
	bootstrapLockFile = ".bootstrap.lock"
	// appLifecycleLockFile is the per-Application lifecycle lock. It lives under
	// a per-app lock directory (root/lock/<scope>) that is INDEPENDENT of the
	// deployments namespace, so that two independent service-less applications
	// that merely share a host Target do not serialize each other, while two
	// writers to the SAME application still do. The deployments namespace is left
	// untouched so owner/never-delivered checks that key off it keep working. The
	// Target itself is protected by an INDEPENDENT owner lock (the per-target
	// lifecycle lock), not by this per-app lock.
	appLifecycleLockFile = "app_installation.lock"
	appLockNamespace     = "lock"
)

// AcquireLifecycleLock serializes bootstrap and provider upgrades of one
// installation, across releases. The kernel releases the lock on process exit;
// durable update receipts still govern recovery after an interrupted mutation.
// The caller must release the returned lock after all verification completes.
func AcquireLifecycleLock(root string) (func(), error) {
	if root == "" {
		return nil, errors.New("lock root is required")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("Core lifecycle lock requires a private state directory")
	}
	return lock(root)
}

// AcquireAppLifecycleLock serialises lifecycle mutation of a SINGLE application
// across releases. The key is a per-app lock directory
// (<root>/lock/<scope>) that is independent of the deployments namespace, so
// the caller's owner/never-delivered checks that key off deployments keep
// working. Two independent applications that share a host Target therefore do
// NOT serialize each other: each holds its own per-app lock. A second writer to
// the SAME application still fails with the kernel flock contention. The kernel
// releases the lock on process exit; durable update receipts still govern
// recovery after an interrupted mutation. The caller must release the returned
// lock after all verification completes.
//
// The Target as a whole is still protected by an independent owner lock
// (AcquireTargetOwnerLock), which the CLI layers on top of this per-app lock.
// This split is what lets independent apps converge in parallel while the same
// app stays strictly serialized.
func AcquireAppLifecycleLock(root string, scope string) (func(), error) {
	if root == "" {
		return nil, errors.New("lock root is required")
	}
	if strings.TrimSpace(scope) == "" {
		return nil, errors.New("application lifecycle lock scope is required")
	}
	// The state root must itself be private BEFORE we create anything inside
	// it: a world/group-readable root is rejected without modifying the
	// filesystem, mirroring the per-target lifecycle lock contract.
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("Application lifecycle lock requires a private per-app state directory")
	}
	appRoot := filepath.Join(root, appLockNamespace, scope)
	if err := os.MkdirAll(appRoot, 0700); err != nil {
		return nil, err
	}
	return lockNamed(appRoot, appLifecycleLockFile)
}

// AcquireStateFileLock reuses the existing kernel lock for another existing
// state owner. The caller proves directory/file ownership and protection using
// its native metadata authority before entering this serialization boundary.
func AcquireStateFileLock(root string) (func(), error) {
	if root == "" {
		return nil, errors.New("lock root is required")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("state lock requires its existing directory")
	}
	return lock(root)
}

// AcquireBootstrapLock serialises bootstrap of a durable state root that does
// not yet carry a genuine installation. It is a SEPARATE kernel lock from
// AcquireLifecycleLock (a distinct .bootstrap.lock file) so bootstrap and
// provider upgrades never contend with each other. The caller creates the root
// (MkdirAll) and provisions it while the returned lock is held, so exactly one
// process ever owns the new durable authority; a second competing fresh
// bootstrap is refused. The caller must release the returned lock after
// provisioning completes.
func AcquireBootstrapLock(root string) (func(), error) {
	if root == "" {
		return nil, errors.New("lock root is required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	unlock, berr := bootstrapLock(root)
	if berr != nil {
		if strings.Contains(berr.Error(), "resource temporarily unavailable") ||
			strings.Contains(berr.Error(), "busy") {
			return nil, errors.New("bootstrap root already locked by a concurrent writer")
		}
		return nil, berr
	}
	return unlock, nil
}
