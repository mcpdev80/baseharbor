package application

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

func TestExternalRemovalInterruptionRetainsRegistrationOwnerAndCanRetry(t *testing.T) {
	root := t.TempDir()
	reg := externalprovider.Registration{ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app", CredentialRef: "file:///protected/retained-credential", Provider: capability.Provider{Kind: capability.ProviderPostgreSQL, Capabilities: []capability.Kind{capability.SQL}}, Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem}}
	if err := RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatal(err)
	}
	store, err := externalProviderStoreAt(root)
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, "foreign-owner")
	if err := os.WriteFile(foreign, []byte("foreign material"), 0600); err != nil {
		t.Fatal(err)
	}
	// Simulate a final-store failure after its earlier inspection and the
	// adjacent registry release. The protected registration remains authoritative.
	if err := os.Remove(store.Path + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, store.Path+".lock"); err != nil {
		t.Fatal(err)
	}
	if err := removeExternalProviderWithAcknowledgedScopeForTest(root, reg.ID); err == nil {
		t.Fatal("interrupted registration change reported success")
	}
	retained, err := store.Inspect(reg.ID)
	if err != nil || retained.ID != reg.ID || retained.CredentialRef != reg.CredentialRef {
		t.Fatalf("original registration/recovery reference lost: %+v %v", retained, err)
	}
	if actual, err := os.ReadFile(foreign); err != nil || string(actual) != "foreign material" {
		t.Fatal("foreign resource changed")
	}
	if err := os.Remove(store.Path + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err := removeExternalProviderWithAcknowledgedScopeForTest(root, reg.ID); err != nil {
		t.Fatalf("retained owner could not complete retry: %v", err)
	}
	if _, err := store.Inspect(reg.ID); err == nil {
		t.Fatal("registration remains after verified retry")
	}
	registryStore, err := referenceProviderRegistryStoreAt(root)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := registryStore.Load()
	if err != nil || len(registry.Instances) != 0 {
		t.Fatalf("registry did not converge: %+v %v", registry, err)
	}
}
