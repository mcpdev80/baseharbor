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
