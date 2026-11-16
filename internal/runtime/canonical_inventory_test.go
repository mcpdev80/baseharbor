package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const canonicalInventoryFixture = "services:\n  provider:\n    image: isolated-provider\nnetworks:\n  owned:\n    name: exact-owned-network\n  shared:\n    external: true\n    name: foreign-network\nvolumes:\n  data: {}\n  byo:\n    external: true\n    name: foreign-volume\n"

func TestExistingCanonicalInventoryRejectsForeignDefinitionAndRecoveryAuthority(t *testing.T) {
	for _, mutation := range []string{"definition", "symlink", "hardlink", "permissions", "linked-parent"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			compose, env := filepath.Join(dir, "compose.yaml"), filepath.Join(dir, "runtime.env")
			if err := os.WriteFile(compose, []byte(canonicalInventoryFixture), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(env, []byte("protected-owner\n"), 0600); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "definition":
				if err := os.WriteFile(compose, []byte(canonicalInventoryFixture+"  injected:\n    name: foreign-data\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(env); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(compose, env); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(env, filepath.Join(t.TempDir(), "foreign-reference")); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.Chmod(env, 0644); err != nil {
					t.Fatal(err)
				}
			case "linked-parent":
				link := filepath.Join(t.TempDir(), "redirected-owner")
				if err := os.Symlink(dir, link); err != nil {
					t.Fatal(err)
				}
				compose, env = filepath.Join(link, "compose.yaml"), filepath.Join(link, "runtime.env")
			}
			if _, err := ExistingCanonicalProjectInventory("isolated", compose, canonicalInventoryFixture, env); err == nil {
				t.Fatal("unverified ownership granted inventory authority")
			}
		})
	}
}

func TestExistingCanonicalInventoryPreservesExternalResourcesAndNativeDefaultNames(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(compose, []byte(canonicalInventoryFixture), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(compose)
	actual, err := ExistingCanonicalProjectInventory("isolated", compose, canonicalInventoryFixture)
	expected := []ProjectResource{{Kind: "container", Name: "isolated-provider-1"}, {Kind: "network", Name: "exact-owned-network"}, {Kind: "volume", Name: "isolated_data"}}
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("canonical inventory adopted external resources or lost native defaults: %+v %v", actual, err)
	}
	after, _ := os.ReadFile(compose)
	if string(before) != string(after) {
		t.Fatal("read-only inventory rewrote owner")
	}
}
