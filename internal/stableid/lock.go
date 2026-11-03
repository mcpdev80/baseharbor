package stableid

import (
	"errors"
	"os"
)

// Installation lock path is the shared, provider-neutral kernel lock file used
// across a single installation state root. It lives in stableid so that the
// lock primitive is a leaf dependency: coreinstallation and serviceaccess both
// depend on it without importing each other, which keeps the dependency graph
// acyclic. The platform-specific `lock` body is provided per-OS via build tags.

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
