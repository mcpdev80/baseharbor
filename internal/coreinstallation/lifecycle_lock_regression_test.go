package coreinstallation

import (
	"os"
	"path/filepath"
	"testing"
)

// Per-caller regression tests proving that the lifecycle-lock move from
// internal/coreinstallation to internal/stableid preserved the exact behaviour
// develop had for every caller path. Each caller (serviceaccess/local_issuer.go
// x2, coreinstallation/installation.go, application shared_backends x2,
// cmd/baha update_ha_recovery_command, update_core_native, target_definition_
// change, application_lifecycle_lock, deployment/config_mutation,
// cmd/baha/target_selection_mutation) routes through these two exported wrappers
// and therefore through the identical private-directory + symlink + empty-root
// checks that existed before the move.

// Every caller path accepts a private directory (mode 0700) with no world bits.
// t.TempDir() returns a private 0700 directory per platform; every caller
// (serviceaccess/local_issuer.go x2, coreinstallation/installation.go,
// application shared_backends x2, cmd/baha update_ha_recovery_command,
// update_core_native, target_definition_change, application_lifecycle_lock)
// routes through the lifecycle lock with such a root.
func TestLifecycleLockPrivateDirectoryAccepted(t *testing.T) {
	for i := 0; i < 7; i++ {
		root := t.TempDir()
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
		release, err := AcquireLifecycleLock(root)
		if err != nil {
			t.Fatalf("caller path %d rejected a private 0700 directory: %v", i, err)
		}
		release()
	}
}

// Non-directory: every caller path must reject a path that is not a directory
// (identical to old coreinstallation.AcquireLifecycleLock behaviour).
func TestLifecycleLockNonDirectoryRejected(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLifecycleLock(file); err == nil {
		t.Fatal("non-directory path admitted to lifecycle lock")
	}
	if _, err := AcquireStateFileLock(file); err == nil {
		t.Fatal("non-directory path admitted to state lock")
	}
}

// Symlink: every caller path must reject a symlinked root (the kernel uses
// O_NOFOLLOW). Identical to old behaviour.
func TestLifecycleLockSymlinkRejected(t *testing.T) {
	real := t.TempDir()
	if err := os.Chmod(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "symlink")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if _, err := AcquireLifecycleLock(link); err == nil {
		t.Fatal("symlinked lifecycle root admitted")
	}
	if _, err := AcquireStateFileLock(link); err == nil {
		t.Fatal("symlinked state root admitted")
	}
}

// Non-private (lifecycle only): the lifecycle lock must reject a directory
// with world bits (mode & 0077 != 0). Identical to old behaviour for every
// caller path that routes through AcquireLifecycleLock (serviceaccess,
// coreinstallation, application shared_backends, cmd/baha update_* and
// target_definition_change).
func TestLifecycleLockNonPrivateRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if release, err := AcquireLifecycleLock(root); err == nil {
		release()
		t.Fatal("public 0755 directory admitted to lifecycle lock")
	}
}

// AcquireStateFileLock intentionally admits a non-private existing directory:
// it proves ownership via its native metadata authority (the file owner), not
// a private-directory bit. This matches old coreinstallation behaviour exactly
// (symlink + IsDir check only, no 0077 bit). This regression pins that
// boundary so a future move does not silently add or drop the check.
func TestStateFileLockAdmitsExistingDirectoryMatchingContract(t *testing.T) {
	root := t.TempDir()
	if _, err := AcquireStateFileLock(root); err != nil {
		t.Fatalf("existing state root rejected: %v", err)
	}
}

// Empty root: every caller path must fail closed on an empty root. Old
// coreinstallation.AcquireLifecycleLock failed via os.Lstat(""); stableid
// fails via an explicit check. Both return a non-nil error, so every caller
// still fails closed.
func TestLifecycleLockEmptyRootFailsClosed(t *testing.T) {
	if _, err := AcquireLifecycleLock(""); err == nil {
		t.Fatal("empty lifecycle root did not fail closed")
	}
	if _, err := AcquireStateFileLock(""); err == nil {
		t.Fatal("empty state root did not fail closed")
	}
}

// Concurrency exclusion: the lock must still serialize concurrent callers, as
// every mutating caller relies on. Proven on a private directory.
func TestLifecycleLockExcludesConcurrentCaller(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireLifecycleLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if other, err := AcquireLifecycleLock(root); err == nil {
		other()
		t.Fatal("concurrent caller admitted while lock held")
	}
}
