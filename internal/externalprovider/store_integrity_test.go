package externalprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func registryIntegrityFixture() Registration {
	return Registration{ID: "owned-reference", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Trust: Trust{Mode: TrustSystem}}
}

func TestStoreRejectsDamagedOwnerAndPreservesItForRecovery(t *testing.T) {
	reg := registryIntegrityFixture()
	duplicate, _ := json.Marshal(Document{Version: StoreVersion, Registrations: []Registration{reg, reg}})
	for _, data := range []string{`{"version":1,"unknown":true}`, `{"version":1} {"version":1}`, string(duplicate), `{"version":1,"registrations":[`} {
		store := Store{Path: filepath.Join(t.TempDir(), "external-providers.json")}
		if err := os.WriteFile(store.Path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatalf("damaged registry accepted: %q", data)
		}
		if err := store.Register(reg); err == nil {
			t.Fatal("registration overwrote damaged owner")
		}
		actual, err := os.ReadFile(store.Path)
		if err != nil || string(actual) != data {
			t.Fatal("damaged ownership lost before recovery")
		}
	}
}

func TestStoreRejectsSymlinkOwnerParentAndLockWithoutMutatingForeignFiles(t *testing.T) {
	for _, mode := range []string{"registry", "parent", "lock", "permissions"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			foreign := filepath.Join(root, "foreign")
			if err := os.Mkdir(foreign, 0700); err != nil {
				t.Fatal(err)
			}
			foreignFile := filepath.Join(foreign, "owner.json")
			original := []byte(`{"version":1}`)
			if err := os.WriteFile(foreignFile, original, 0600); err != nil {
				t.Fatal(err)
			}
			store := Store{Path: filepath.Join(root, "external-providers.json")}
			switch mode {
			case "registry":
				if err := os.Symlink(foreignFile, store.Path); err != nil {
					t.Fatal(err)
				}
			case "parent":
				parent := filepath.Join(root, "redirect")
				if err := os.Symlink(foreign, parent); err != nil {
					t.Fatal(err)
				}
				store.Path = filepath.Join(parent, "external-providers.json")
			case "lock":
				if err := os.Symlink(foreignFile, store.Path+".lock"); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.WriteFile(store.Path, original, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Register(registryIntegrityFixture()); err == nil {
				t.Fatal("unprotected owner mutation accepted")
			}
			actual, err := os.ReadFile(foreignFile)
			if err != nil || string(actual) != string(original) {
				t.Fatal("foreign file was modified")
			}
		})
	}
}

func TestStoreBoundsReadsAndAvoidsPredictableTemporaryFile(t *testing.T) {
	root := t.TempDir()
	store := Store{Path: filepath.Join(root, "external-providers.json")}
	foreign := filepath.Join(root, "foreign")
	if err := os.WriteFile(foreign, []byte("retain foreign data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, store.Path+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(registryIntegrityFixture()); err != nil {
		t.Fatal(err)
	}
	if actual, err := os.ReadFile(foreign); err != nil || string(actual) != "retain foreign data" {
		t.Fatal("predictable temporary path modified foreign data")
	}
	file, err := os.OpenFile(store.Path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxRegistryBytes + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := store.Load(); err == nil {
		t.Fatal("oversized registry accepted")
	}
}
