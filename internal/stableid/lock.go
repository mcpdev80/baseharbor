package stableid

import (
	"errors"
	"os"
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
