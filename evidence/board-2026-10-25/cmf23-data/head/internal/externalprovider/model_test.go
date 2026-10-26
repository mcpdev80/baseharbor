package externalprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRegistrationRejectsCredentialsInEndpoint(t *testing.T) {
	r := Registration{
		ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://user:secret@db.example:5432/app",
		Provider: capability.Provider{Kind: capability.ProviderKind("company-postgresql"), Capabilities: []capability.Kind{capability.SQL}},
		Trust:    Trust{Mode: TrustSystem},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected embedded credentials to fail")
	}
}

func TestEndpointSecretsCannotEnterRegistrationOrPublicOutput(t *testing.T) {
	for _, endpoint := range []string{
		"postgres://db.example:5432/app?password=sentinel-secret",
		"https://identity.example?access_token=sentinel-secret",
		"https://identity.example?%61ccess_token=sentinel-secret",
		"https://identity.example#sentinel-secret",
		"https://identity.example?",
		"https://sentinel-secret@identity.example",
	} {
		t.Run(endpoint, func(t *testing.T) {
			store := Store{Path: filepath.Join(t.TempDir(), "providers.json")}
			reg := registryIntegrityFixture()
			if err := store.Register(reg); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.Path)
			if err != nil {
				t.Fatal(err)
			}
			reg.ID = "rejected-endpoint"
			reg.Endpoint = endpoint
			err = store.Register(reg)
			if err == nil || strings.Contains(err.Error(), "sentinel-secret") {
				t.Fatal("unsafe endpoint must be rejected without exposing its contents")
			}
			after, err := os.ReadFile(store.Path)
			if err != nil || string(before) != string(after) {
				t.Fatal("rejected endpoint altered the existing registry")
			}
			items, err := store.List()
			if err != nil || len(items) != 1 || items[0].ID != "owned-reference" {
				t.Fatal("rejected endpoint became a public registration")
			}
			// A previously stored unsafe endpoint must not be exposed by reads
			// or silently rewritten while its owner needs recovery.
			stored, err := json.Marshal(Document{Version: StoreVersion, Registrations: []Registration{reg}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.Path, stored, 0600); err != nil {
				t.Fatal(err)
			}
			_, err = store.Inspect(reg.ID)
			if err == nil || strings.Contains(err.Error(), "sentinel-secret") {
				t.Fatal("read exposed an unsafe stored endpoint")
			}
			after, err = os.ReadFile(store.Path)
			if err != nil || string(stored) != string(after) {
				t.Fatal("read rewrote unsafe recovery state")
			}
		})
	}
}

func TestRegistrationAcceptsReferenceOnlyTrust(t *testing.T) {
	r := Registration{
		ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app",
		CredentialRef: "vault://team/app/db",
		Provider:      capability.Provider{Kind: capability.ProviderKind("company-postgresql"), Capabilities: []capability.Kind{capability.SQL}},
		Trust:         Trust{Mode: TrustMTLS, CAReference: "corp-root", ClientCertificate: "cert://db-client", ClientKey: "key://db-client"},
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}
