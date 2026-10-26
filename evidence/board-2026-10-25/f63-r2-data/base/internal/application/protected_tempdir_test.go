package application

import (
	"os"
	"testing"
)

// protectedTempDir returns a deterministic, owner-safe data root.
//
// t.TempDir() creates its numbered directory with os.Mkdir(dir, 0o777), so the
// effective mode is the machine umask: a group-writable umask such as 002
// yields 0775, which the protected-owner guards must reject fail-closed. The
// CI umask 022 hid this; normalizing to 0700 keeps one real code path on every
// machine without skipping and preserves the guards' contract, including the
// tests that chmod deliberately unprotected modes to assert rejection.
func protectedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}
