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
	a()
	b()

	// The per-app lock file lives under <root>/lock/<scope> (independent of the
	// deployments namespace) and is separate from the shared per-target
	// .installation.lock.
	for _, scope := range []string{"app-a", "app-b", "shared-app"} {
		if _, err := os.Stat(filepath.Join(root, "lock", scope, "app_installation.lock")); err != nil {
			t.Fatalf("per-app lock file missing for scope %q: %v", scope, err)
		}
	}
	// The per-target lock file must NOT have been created by the per-app path.
	if _, err := os.Stat(filepath.Join(root, ".installation.lock")); !os.IsNotExist(err) {
		t.Fatal("per-app lock path created the shared per-target lock file")
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
	if _, err := os.Stat(filepath.Join(root, "deployments", "app")); !os.IsNotExist(err) {
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
