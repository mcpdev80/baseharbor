package stableid

import (
	"errors"
	"fmt"
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
	// section, then create the per-app dotfile inside it. This is the F3
	// production coordination: the .installation.lock is the single critical
	// section shared by the per-app dotfile creation AND the target-wide scan.
	// While a target-wide mutation holds .installation.lock to run its scan,
	// this acquirer gets flock contention and fails — a new app cannot begin
	// during a held target-wide scan (direction 2 of the regression). When no
	// scan holds it, this app creates its dotfile and releases the target-wide
	// lock immediately, so unrelated apps still converge in parallel and the
	// target-wide mutation can grab the lock to scan. The dotfile creation and
	// the scan share the same lock critical section, so there is no
	// check-then-act race. The .installation.lock is released immediately after
	// creation (only the per-app dotfile persists as the active signal), so the
	// scan a target-wide mutation later runs observes this held dotfile.
	wideUnlock, err := lock(root)
	if err != nil {
		return nil, err
	}
	// 0427-180: while holding the shared .installation.lock, reclaim a per-app
	// dotfile whose recorded holder is provably stale (dead, or pid reused on a
	// different boot / start second) BEFORE creating this app's dotfile. The
	// decision runs inside the same critical section the target-wide scan
	// shares, so the read-decide-remove is atomic against that scan and a live
	// holder is never reclaimed. The kernel flock still blocks a live same-scope
	// writer here; only a confirmed-stale receipt is removed — this is self-help
	// for the SIGKILLed-holder case, not a bypass of cross-process contention.
	if _, serr := reclaimStaleAppLock(deploymentsDir, lockName); serr != nil {
		wideUnlock()
		return nil, serr
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
//
// Fail-closed contract (card 0427-176 r3): a ReadDir error on an existing
// deployments namespace is NOT a scan result and must never be treated as
// "no active app". The caller must fail closed on any read error, because a
// corrupt or unreadable deployments namespace is a production-owner failure, not
// a free pass for a target-wide mutation to run during an app lifecycle. A
// MISSING deployments namespace (its parent is a real, readable directory but
// the deployments child does not yet exist) is treated as "no active app": a
// remote target or a never-staged target has no per-app dotfiles to protect. The
// scan and any per-app dotfile creation share the same .installation.lock
// critical section, so the scan cannot observe a half-created or half-removed
// dotfile.
func deploymentsHaveAppLocks(root string) (bool, error) {
	deploymentsDir := filepath.Join(root, "deployments")
	entries, err := os.ReadDir(deploymentsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No deployments namespace yet (remote target, never-staged target):
			// nothing holds a per-app dotfile, so the scan is empty.
			return false, nil
		}
		// Any other read error (permission, I/O, truncated directory) fails
		// closed: it is not a pass to run during an app lifecycle.
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") && strings.HasSuffix(e.Name(), "."+appLifecycleLockFile) {
			return true, nil
		}
	}
	return false, nil
}

// reclaimStaleAppLock inspects the per-app dotfile at <deploymentsDir>/<lockName>
// and removes it ONLY when its recorded holder is provably stale (dead, or a
// different boot-id / start second than the current process). It is called by
// the per-app acquire path while holding the shared .installation.lock, so the
// read-decide-remove is atomic against the target-wide scan: a live holder is
// never reclaimed, and a torn/SIGKILLed receipt is removed under the same
// critical section the scan shares, so the scan can never observe a
// half-created or half-removed dotfile.
//
// A missing dotfile is "nothing to reclaim"; an unreadable deployments
// namespace fails closed (never a free pass); a foreign or malformed receipt
// is treated as a live holder and left in place (do not destroy another
// writer's state on a decode guess).
func reclaimStaleAppLock(deploymentsDir, lockName string) (reclaimed bool, err error) {
	receiptPath := filepath.Join(deploymentsDir, lockName)
	buf, rerr := os.ReadFile(receiptPath)
	if rerr != nil {
		if errors.Is(rerr, os.ErrNotExist) {
			return false, nil
		}
		return false, rerr
	}
	rec, ok := parseLockHolderRecord(buf)
	if !ok {
		// Not a baseharbor receipt we wrote: leave it untouched.
		return false, nil
	}
	if lockHolderLive(rec) {
		return false, nil
	}
	// Confirmed stale (dead holder, or pid reused on a different boot /
	// different start second): remove the receipt under the lock. If the dotfile
	// disappears or changes under us, another writer is racing — fail closed
	// rather than remove someone else's fresh receipt.
	if rerr := os.Remove(receiptPath); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		return false, rerr
	}
	return true, nil
}

// AcquireTargetWideLifecycleLock is the target-wide production acquire path for
// a whole-Target lifecycle mutation (destroy/stop of the target, or a
// Target-definition change). It holds the shared .installation.lock EXCLUSIVELY
// for the whole mutation and scans the deployments namespace for held per-app
// dotfiles while holding it, so:
//
//   - direction 1: a running local app lifecycle blocks the mutation — the held
//     per-app dotfile is observed under the same critical section and the caller
//     fails closed;
//   - direction 2: a concurrent local app cannot create its dotfile while the
//     mutation holds .installation.lock — AcquireAppLifecycleLock gets flock
//     contention and fails (see its doc).
//
// Fail-closed contract (card 0427-176 r3): any .installation.lock contention or
// deployments-namespace read error fails closed — a held lock is never a "busy,
// retry" for the mutation, and a read error is never treated as an empty scan.
// The returned unlock releases the target-wide lock; callers hold it for the
// whole mutation. Unlike AcquireLifecycleLock this caller must prove the same
// private-directory owner rule before entering.
func AcquireTargetWideLifecycleLock(root string) (func(), error) {
	if root == "" {
		return nil, errors.New("lock root is required")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("Target lifecycle lock requires a private state directory")
	}
	// Hold the shared target-wide owner lock exclusively. If a local app is
	// creating its dotfile right now, or another target-wide mutation holds it,
	// this fails closed.
	unlock, err := lock(root)
	if err != nil {
		return nil, err
	}
	// Fail-closed on scan error: an unreadable deployments namespace is a
	// production-owner failure, not a pass to run during an app lifecycle.
	if held, scanErr := deploymentsHaveAppLocks(root); scanErr != nil {
		unlock()
		return nil, fmt.Errorf("target-wide lifecycle scan failed to read the deployments namespace of %q: %w", root, scanErr)
	} else if held {
		unlock()
		return nil, fmt.Errorf("a local app lifecycle is active on %q; the target-wide lifecycle mutation cannot overlap it", root)
	}
	return unlock, nil
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
