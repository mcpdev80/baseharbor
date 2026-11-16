package externalprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsafeReferencesCannotEnterPublicRegistration(t *testing.T) {
	for _, reference := range []string{
		"file://user:sentinel-secret@host/credentials.json",
		"vault://team/app?token=sentinel-secret",
		"vault://team/app#sentinel-secret",
		"vault://team/app?",
		"-----BEGIN PRIVATE KEY-----sentinel-secret",
		"vault://team/app\nsentinel-secret",
		"{\"password\":\"sentinel-secret\"}",
	} {
		for _, field := range []string{"credential", "key"} {
			t.Run(field+"/"+reference, func(t *testing.T) {
				store := Store{Path: filepath.Join(t.TempDir(), "providers.json")}
				reg := registryIntegrityFixture()
				if err := store.Register(reg); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(store.Path)
				if err != nil {
					t.Fatal(err)
				}
				reg.ID = "unsafe-reference"
				if field == "credential" {
					reg.CredentialRef = reference
				} else {
					reg.Trust = Trust{Mode: TrustMTLS, ClientCertificate: "/protected/client.crt", ClientKey: reference}
				}
				err = store.Register(reg)
				if err == nil || strings.Contains(err.Error(), "sentinel-secret") {
					t.Fatal("unsafe reference accepted or exposed in validation error")
				}
				after, err := os.ReadFile(store.Path)
				if err != nil || string(before) != string(after) {
					t.Fatal("unsafe reference altered the existing public registration")
				}
			})
		}
	}
}
