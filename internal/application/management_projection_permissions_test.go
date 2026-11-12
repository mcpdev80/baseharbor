package application

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUIRuntimeProjectionOverridesRestrictiveUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)

	path := filepath.Join(t.TempDir(), "consumer-config.json")
	if err := writeUIRuntimeProjection(path, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("consumer projection mode = %04o, want 0644", got)
	}
}
