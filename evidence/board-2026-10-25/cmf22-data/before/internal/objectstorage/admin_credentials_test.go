package objectstorage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContainerAdminCredentialPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.env")
	data := []byte("ACCESS_KEY_ID=test-access\nSECRET_ACCESS_KEY=test-secret\n")

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAdminCredentials(path); err == nil {
		t.Fatal("provider-state loader accepted group/world-readable credentials")
	}
	if _, err := LoadContainerAdminCredentials(path); err != nil {
		t.Fatalf("container loader rejected read-only projection: %v", err)
	}

	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadContainerAdminCredentials(path); err == nil {
		t.Fatal("container loader accepted group/world-writable credentials")
	}
}
