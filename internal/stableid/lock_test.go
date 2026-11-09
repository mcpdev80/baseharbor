package stableid

import (
	"os"
	"path/filepath"
	"testing"
)

// The lifecycle-lock primitive now lives here as a stdlib-only leaf so that
// coreinstallation and serviceaccess can both depend on it without an import
// cycle. These tests preserve the coreinstallation lifecycle-lock contract
// exactly (error messages, concurrency exclusion, symlink/permission checks).

func TestAcquireLifecycleLockExcludesConcurrentUpdateAndReleases(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireLifecycleLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	if other, err := AcquireLifecycleLock(root); err == nil {
		other()
		t.Fatal("concurrent Core lifecycle admitted")
	}
	release()
	release = nil
	other, err := AcquireLifecycleLock(root)
	if err != nil {
		t.Fatalf("released lock unavailable: %v", err)
	}
	other()
}

func TestAcquireLifecycleLockRejectsForeignRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if release, err := AcquireLifecycleLock(root); err == nil {
		release()
		t.Fatal("public lifecycle root accepted")
	}
	if _, err := os.Stat(filepath.Join(root, ".installation.lock")); !os.IsNotExist(err) {
		t.Fatal("unsafe directory was modified")
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "foreign-state")
	if err := os.Symlink(root, link); err != nil {
		t.Skip(err)
	}
	if release, err := AcquireLifecycleLock(link); err == nil {
		release()
		t.Fatal("symlink lifecycle root accepted")
	}
}

func TestAcquireStateFileLockRejectsSymlinkedRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireStateFileLock(root); err != nil {
		t.Fatalf("existing state root rejected: %v", err)
	}
	link := filepath.Join(t.TempDir(), "symlink")
	if err := os.Symlink(root, link); err != nil {
		t.Skip(err)
	}
	if _, err := AcquireStateFileLock(link); err == nil {
		t.Fatal("symlinked state root admitted")
	}
}

// TestAcquireAppLifecycleLockIsPerAppScope proves the per-app lock is keyed by
// scope: two writers to the SAME app scope on one root exclude each other,
// while two writers to DIFFERENT app scopes on the same root do NOT exclude
// each other (independent apps on one target converge in parallel).
func TestAcquireAppLifecycleLockIsPerAppScope(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}

	// Same scope: second writer is excluded.
	alpha, err := AcquireAppLifecycleLock(root, "shared-app")
	if err != nil {
		t.Fatalf("acquire app scope: %v", err)
	}
	if _, err := AcquireAppLifecycleLock(root, "shared-app"); err == nil {
		alpha()
		t.Fatal("second writer to the same app scope was admitted")
	}
	alpha()

	// Different scope on the same root: second writer is admitted immediately.
	a, err := AcquireAppLifecycleLock(root, "app-a")
	if err != nil {
		t.Fatalf("acquire app-a: %v", err)
	}
	b, err := AcquireAppLifecycleLock(root, "app-b")
	if err != nil {
		a()
		t.Fatalf("independent app scope blocked by another app on the same root (err=%v); independent apps must not serialize", err)
	}
	// While the independent apps hold their per-app locks, each dotfile must be
	// present (the per-app "active" signal a target-wide scan observes). The
	// dotfiles are removed on release (see release), so assert presence before
	// releasing. shared-app was released earlier (alpha()), so assert only the
	// still-held apps.
	for _, scope := range []string{"app-a", "app-b"} {
		lockPath := filepath.Join(root, "deployments", "."+scope+".app_installation.lock")
		if _, err := os.Stat(lockPath); err != nil {
			a()
			b()
			t.Fatalf("per-app lock file missing for scope %q while held: %v", scope, err)
		}
	}

	a()
	b()
}

// TestLocalAppLifecycleCannotOverlapTargetWideMutation proves cross-operation
// coordination: a local app holds its per-app lock while a target-wide stop
// (which takes the shared .installation.lock exclusively and scans for per-app
// dotfiles) must wait, and a target-wide stop blocks a concurrent local app.
func TestLocalAppLifecycleCannotOverlapTargetWideMutation(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "deployments"), 0700); err != nil {
		t.Fatal(err)
	}

	// (1) A local app holds its per-app dotfile; a target-wide scan that holds
	// .installation.lock must observe the active per-app lock and refuse to
	// overlap it.
	localUnlock, err := AcquireAppLifecycleLock(root, "app-held")
	if err != nil {
		t.Fatalf("local app acquire: %v", err)
	}
	if held, scanErr := deploymentsHaveAppLocks(root); scanErr != nil {
		localUnlock()
		t.Fatalf("target-wide scan error: %v", scanErr)
	} else if !held {
		localUnlock()
		t.Fatal("target-wide scan did not observe the held per-app dotfile")
	}

	// (2) A target-wide scan blocks a concurrent local app: a local app cannot
	// create its per-app dotfile while a target-wide mutation holds
	// .installation.lock. A second local app must fail while the target-wide
	// lock is held by a scan.
	scanUnlock, err := lock(root) // simulate a target-wide mutation
	if err != nil {
		t.Fatalf("target-wide scan acquire: %v", err)
	}
	if _, err := AcquireAppLifecycleLock(root, "app-blocked"); err == nil {
		scanUnlock()
		localUnlock()
		t.Fatal("a local app created a per-app dotfile during a target-wide mutation")
	}
	// The blocked app must not have staged a per-app dotfile.
	if _, statErr := os.Stat(filepath.Join(root, "deployments", ".app-blocked.app_installation.lock")); !os.IsNotExist(statErr) {
		scanUnlock()
		localUnlock()
		t.Fatal("blocked app staged a per-app dotfile during a target-wide mutation")
	}

	// Releasing the target-wide scan frees the target; a new local app can now
	// create its per-app dotfile (the creation critical section is open again).
	scanUnlock()
	nextUnlock, err := AcquireAppLifecycleLock(root, "app-next")
	if err != nil {
		localUnlock()
		t.Fatalf("local app could not acquire after the target-wide scan released: %v", err)
	}

	// Release the held local app and the next app; the last holder removes the
	// last per-app dotfile so the target is free for a target-wide mutation.
	nextUnlock()
	localUnlock()
	if held, scanErr := deploymentsHaveAppLocks(root); scanErr != nil {
		t.Fatalf("scan error after release: %v", scanErr)
	} else if held {
		t.Fatal("per-app dotfiles remain after all local apps released")
	}
}

// TestAcquireAppLifecycleLockRejectsForeignRoot mirrors the lifecycle lock's
// private-directory contract for the per-app deployments directory.
func TestAcquireAppLifecycleLockRejectsForeignRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireAppLifecycleLock(root, "app"); err == nil {
		t.Fatal("foreign per-app root admitted")
	}
	if _, err := os.Stat(filepath.Join(root, "deployments", ".app.app_installation.lock")); !os.IsNotExist(err) {
		t.Fatal("unsafe directory was modified by the per-app lock")
	}
}

// The lock file must not be creatable on a directory that fails the private
// directory check, so a concurrent reader observes no state change.
func TestAcquireLifecycleLockEmptyRootRejected(t *testing.T) {
	if _, err := AcquireLifecycleLock(""); err == nil {
		t.Fatal("empty root accepted")
	}
	if _, err := AcquireStateFileLock(""); err == nil {
		t.Fatal("empty state root accepted")
	}
}
