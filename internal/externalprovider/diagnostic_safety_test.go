package externalprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectedRegistrationDiagnosticsPreserveSecretsAndRecovery(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		alter func(*Registration)
	}{
		{"trust-mode", func(r *Registration) { r.Trust.Mode = "sentinel-secret" }},
		{"sensitive-metadata-key", func(r *Registration) { r.Metadata = map[string]string{"token=sentinel-secret": "rejected"} }},
		{"newline-metadata", func(r *Registration) { r.Metadata = map[string]string{"sentinel-secret": "invalid\nvalue"} }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := Store{Path: filepath.Join(t.TempDir(), "providers.json")}
			reg := registryIntegrityFixture()
			if err := store.Register(reg); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.Path)
			if err != nil {
				t.Fatal(err)
			}
			scenario.alter(&reg)
			checkError := func(err error) {
				t.Helper()
				if err == nil || strings.Contains(err.Error(), "sentinel-secret") {
					t.Fatal("rejected registration accepted or exposed in diagnostic")
				}
			}
			checkError(store.Register(reg))
			after, err := os.ReadFile(store.Path)
			if err != nil || string(before) != string(after) {
				t.Fatal("rejection changed registry")
			}
			// Legacy damaged records must also fail public reads without rewriting
			// the bytes retained for recovery or reflecting their rejected input.
			stored, err := json.Marshal(Document{Version: StoreVersion, Registrations: []Registration{reg}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.Path, stored, 0600); err != nil {
				t.Fatal(err)
			}
			_, err = store.Inspect(reg.ID)
			checkError(err)
			_, err = store.List()
			checkError(err)
			after, err = os.ReadFile(store.Path)
			if err != nil || string(stored) != string(after) {
				t.Fatal("read changed recovery state")
			}
		})
	}
}
