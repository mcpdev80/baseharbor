package coreinstallation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLifecycleLockExcludesConcurrentUpdateAndReleases(t *testing.T) {
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

func TestLifecycleLockRejectsForeignRoot(t *testing.T) {
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
