package externalprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownRegistryFieldsDoNotExposeRejectedMaterialOrRewriteOwner(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "document", true: "trust"}[nested], func(t *testing.T) {
			registration := registryIntegrityFixture()
			encoded, err := json.Marshal(Document{Version: StoreVersion, Registrations: []Registration{registration}})
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(encoded, &document); err != nil {
				t.Fatal(err)
			}
			owner := document
			if nested {
				owner = document["registrations"].([]any)[0].(map[string]any)["trust"].(map[string]any)
			}
			owner["token=sentinel-secret"] = "rejected material"
			encoded, err = json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			store := Store{Path: filepath.Join(t.TempDir(), "providers.json")}
			if err := os.WriteFile(store.Path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			assertRejected := func(err error) {
				t.Helper()
				if err == nil || strings.Contains(err.Error(), "sentinel-secret") {
					t.Fatal("unknown field was accepted or reflected into ordinary diagnostics")
				}
			}
			_, err = store.Inspect(registration.ID)
			assertRejected(err)
			_, err = store.List()
			assertRejected(err)
			assertRejected(store.Register(registration))
			after, err := os.ReadFile(store.Path)
			if err != nil || string(after) != string(encoded) {
				t.Fatal("invalid registry recovery bytes were overwritten")
			}
		})
	}
}
