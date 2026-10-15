package application

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestSharedBackendInventoryIsCanonicalAndReadOnly(t *testing.T) {
	data := t.TempDir()
	files := SharedBackendFilesAt(data, "isolated", "dev")
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "core", PostgresMembers: 1, PostgresAdminCredential: "credentials/admin", Applications: map[string]sharedBackendAppState{}}
	if err := writeSharedBackendState(files.State, state); err != nil {
		t.Fatal(err)
	}
	canonical := sharedBackendCompose(files, state)
	for path, content := range map[string]string{files.Compose: canonical, files.Env: "SHARED_POSTGRES_ADMIN_PASSWORD=test-only\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadDir(files.Dir)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := ExistingSharedBackendInventoryAt(data, "isolated", "dev")
	if err != nil || len(inventory.Services[capability.ProviderPostgreSQL]) == 0 || len(inventory.Services[capability.ProviderValkey]) != 0 || len(inventory.Resources) == 0 {
		t.Fatalf("SQL-only canonical inventory: %#v %v", inventory, err)
	}
	after, err := os.ReadDir(files.Dir)
	if err != nil || len(before) != len(after) {
		t.Fatalf("inventory created owner files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(files.Dir, "credentials")); !os.IsNotExist(err) {
		t.Fatal("inventory allocated credentials")
	}
	if err := os.WriteFile(files.Compose, []byte(canonical+"# foreign modification\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingSharedBackendInventoryAt(data, "isolated", "dev"); err == nil {
		t.Fatal("modified definition accepted")
	}
	actual, err := os.ReadFile(files.Compose)
	if err != nil || string(actual) != canonical+"# foreign modification\n" {
		t.Fatal("inventory rewrote modified definition")
	}
	if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(files.State, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingSharedBackendInventoryAt(data, "isolated", "dev"); err == nil {
		t.Fatal("unprotected owner state accepted")
	}
}
