package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"go.yaml.in/yaml/v3"
)

func TestSharedBackendInventoryAcceptsOnlyLegacyFixedUIDOverrides(t *testing.T) {
	files := SharedBackendFilesAt(t.TempDir(), "isolated", "core")
	state := sharedBackendState{
		Version: sharedBackendStateVersion, Environment: "core",
		ValkeyAdminCredential: "credentials/admin", ValkeyMembers: 1,
		Applications: map[string]sharedBackendAppState{
			"core/shared": {Application: "core", Environment: "shared", Cache: map[string]sharedValkeyResource{"default": {}}},
		},
	}
	canonical := []byte(sharedBackendCompose(files, state))
	var document map[string]any
	if err := yaml.Unmarshal(canonical, &document); err != nil {
		t.Fatal(err)
	}
	services := document["services"].(map[string]any)
	for _, raw := range services {
		service := raw.(map[string]any)
		if legacyUID, ok := legacySharedBackendUserForImage(service["image"].(string)); ok {
			service["user"] = legacyUID
		}
	}
	legacy, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if !sharedBackendComposeMatchesCanonical(legacy, canonical) {
		t.Fatal("previous renderer with only its known fixed-UID overrides must remain upgradeable")
	}

	for _, mutate := range []func(map[string]any){
		func(services map[string]any) {
			services["shared-valkey-core-shared-default"].(map[string]any)["user"] = "0"
		},
		func(services map[string]any) {
			services["shared-valkey-core-shared-default"].(map[string]any)["restart"] = "no"
		},
	} {
		var changed map[string]any
		if err := yaml.Unmarshal(legacy, &changed); err != nil {
			t.Fatal(err)
		}
		mutate(changed["services"].(map[string]any))
		bytes, err := yaml.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if sharedBackendComposeMatchesCanonical(bytes, canonical) {
			t.Fatal("unrecognized identity or runtime modification accepted")
		}
	}
	if strings.Contains(string(canonical), "user:") {
		t.Fatal("new shared backend renderer must leave container user unset")
	}
}

func TestSharedBackendInventoryIsCanonicalAndReadOnly(t *testing.T) {
	data := t.TempDir()
	files := SharedBackendFilesAt(data, "isolated", "dev")
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "core", PostgresMembers: 3, PostgresAdminCredential: "credentials/admin", Applications: map[string]sharedBackendAppState{}}
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
	var legacyDocument map[string]any
	if err := yaml.Unmarshal([]byte(canonical), &legacyDocument); err != nil {
		t.Fatal(err)
	}
	legacyDocument["services"].(map[string]any)["postgres-access"].(map[string]any)["user"] = "99:99"
	legacyCompose, err := yaml.Marshal(legacyDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, legacyCompose, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingSharedBackendInventoryAt(data, "isolated", "dev"); err != nil {
		t.Fatalf("known legacy fixed-UID owner must be inventoryable for upgrade: %v", err)
	}
	modified := strings.Replace(canonical, "restart: unless-stopped", "restart: no", 1)
	if modified == canonical {
		t.Fatal("test fixture has no restart policy to mutate")
	}
	if err := os.WriteFile(files.Compose, []byte(modified), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingSharedBackendInventoryAt(data, "isolated", "dev"); err == nil {
		t.Fatal("modified definition accepted")
	}
	actual, err := os.ReadFile(files.Compose)
	if err != nil || string(actual) != modified {
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
