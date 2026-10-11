package externalprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialMetadataCannotEnterPublicRegistration(t *testing.T) {
	for _, key := range []string{"api_key", "API-Key", "api.key", "access key", "Authorization", "Proxy-Authorization", "credentials", "client_credentials", "passphrase", "private-key", "pass_word", "access.to.ken"} {
		t.Run(key, func(t *testing.T) {
			store := Store{Path: filepath.Join(t.TempDir(), "providers.json")}
			reg := registryIntegrityFixture()
			if err := store.Register(reg); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.Path)
			if err != nil {
				t.Fatal(err)
			}
			reg.Metadata = map[string]string{key: "sentinel-credential-value"}
			check := func(err error) {
				t.Helper()
				if err == nil || strings.Contains(err.Error(), "sentinel-credential-value") || strings.Contains(err.Error(), key) {
					t.Fatal("credential metadata accepted or reflected in diagnostic")
				}
			}
			check(store.Register(reg))
			after, err := os.ReadFile(store.Path)
			if err != nil || string(after) != string(before) {
				t.Fatal("rejection changed existing registration")
			}
			legacy, err := json.Marshal(Document{Version: StoreVersion, Registrations: []Registration{reg}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.Path, legacy, 0600); err != nil {
				t.Fatal(err)
			}
			_, err = store.Inspect(reg.ID)
			check(err)
			_, err = store.List()
			check(err)
			after, err = os.ReadFile(store.Path)
			if err != nil || string(after) != string(legacy) {
				t.Fatal("public read altered protected recovery bytes")
			}
		})
	}
}

func TestPublicDescriptiveMetadataRetainsExactValues(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "providers.json")}
	reg := registryIntegrityFixture()
	reg.Metadata = map[string]string{"auth_mode": "mtls", "region": "eu-west", "public_key_algorithm": "RSA", "team": "runtime"}
	if err := store.Register(reg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Inspect(reg.ID)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range reg.Metadata {
		if got.Metadata[key] != value {
			t.Fatal("descriptive public metadata changed")
		}
	}
}
