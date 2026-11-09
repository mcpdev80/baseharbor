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
// Cross-operation coordination (card 0427-176 r2): the per-app dotfile is
// created inside a brief exclusive critical section guarded by the shared
// target-wide owner lock (.installation.lock under <root>). The per-app dotfile
// persists as the per-app "active" signal; a target-wide mutation (stop/destroy
// of the whole target, or a Target-definition change) takes the same
// .installation.lock exclusively and scans the deployments namespace for held
// per-app dotfiles, so it can never overlap a local application lifecycle while
// unrelated apps still converge in parallel. Dotfile creation and the target-wide
// scan share the .installation.lock critical section, so there is no
// check-then-act race. This is proven by
// TestLocalAppLifecycleCannotOverlapTargetWideMutation: a local app holds its
// per-app lock while a target-wide stop must wait, and a target-wide stop
// blocks a concurrent local app.
//
// The returned unlock releases the per-app dotfile and removes the dotfile so
// the target-wide scan no longer sees it as an active app.
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
	// Take the shared target-wide owner lock exclusively as a brief critical
	// section, then create the per-app dotfile inside it so a target-wide scan
	// that also holds this lock never races dotfile creation. The
	// .installation.lock is released immediately after (only the per-app dotfile
	// persists as the active signal), so unrelated apps still converge in
	// parallel and a target-wide mutation can grab the target-wide lock freely
	// while scanning. A concurrent target-wide mutation already holds
	// .installation.lock, so this acquires nothing and the dotfile is never
	// created — a local app cannot overlap a target-wide mutation.
	wideUnlock, err := lock(root)
	if err != nil {
		return nil, err
	}
	appUnlock, err := lockNamed(deploymentsDir, lockName)
	if err != nil {
		wideUnlock()
		return nil, err
	}
	wideUnlock()
	return func() { _ = os.Remove(deploymentsDir + "/" + lockName); appUnlock() }, nil
}

// deploymentsHaveAppLocks reports whether the deployments namespace still holds a
// per-app lifecycle dotfile. Only dotfiles ending in <dot>.app_installation.lock
// count; a genuine deployment directory (a normal directory entry) is a real
// deployment, not a per-app lock, and does not count as an active app. This is
// the target-wide scan a target-wide mutation (stop/destroy of the whole target,
// or a Target-definition change) uses to prove a local app lifecycle cannot
// overlap it.
func deploymentsHaveAppLocks(root string) bool {
	deploymentsDir := filepath.Join(root, "deployments")
	entries, err := os.ReadDir(deploymentsDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") && strings.HasSuffix(e.Name(), "."+appLifecycleLockFile) {
			return true
		}
	}
	return false
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
