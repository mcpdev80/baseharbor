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
	// appLifecycleLockFile is the per-Application lifecycle lock. It lives as a
	// hidden dotfile <root>/deployments/.<scope>.app_installation.lock inside the
	// deployments namespace, so that two independent service-less applications
	// that merely share a host Target do not serialize each other, while two
	// writers to the SAME application still do. The leading dot keeps this lock
	// file out of listDeploymentRecords (directories only) and from the
	// never-delivered empty check (dotfiles ignored), so a lock file is never
	// read as a partial deployment record.
	appLifecycleLockFile = "app_installation.lock"
	// AppLifecycleLockFileName is the per-Application lifecycle lock file name,
	// exported so callers can recognize a deployments dir that holds only this
	// lock (e.g. never-delivered detection in cmd/baha).
	AppLifecycleLockFileName = "app_installation.lock"
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
// across releases. The key is a hidden dotfile lock
// (<root>/deployments/.<scope>.app_installation.lock) inside the deployments
// namespace but as a dotfile, so listDeploymentRecords (directories only) and
// the never-delivered empty check (dotfiles ignored) never see it as a partial
// deployment. Two independent applications that share a host Target therefore
// do NOT serialize each other: each holds its own per-app lock. A second writer
// to the SAME application still fails with the kernel flock contention. The
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
	// Key the lock at <root>/deployments/.<scope>.app_installation.lock. The
	// deployments namespace already exists for a local application target;
	// create it if missing. The leading dot keeps this lock file out of
	// listDeploymentRecords (which only treats directories as deployment
	// records) and from the never-delivered empty check (which ignores
	// dotfiles), so a lock file is never read as a partial deployment record.
	// lockNamed(root, name) locks root/<name> as a single dotfile, so the
	// deployments dir is the lock root and the full dotfile name (dot + scope
	// + "." + lock file) is the name.
	deploymentsDir := filepath.Join(root, "deployments")
	if err := os.MkdirAll(deploymentsDir, 0700); err != nil {
		return nil, err
	}
	lockName := "." + scope + "." + appLifecycleLockFile
	return lockNamed(deploymentsDir, lockName)
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
