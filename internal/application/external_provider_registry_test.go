package application

import (
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

func TestExternalProviderRegistrationAndRemoval(t *testing.T) {
	dataDir := t.TempDir()
	reg := externalprovider.Registration{
		ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app",
		CredentialRef: "vault://team/app/db",
		Provider:      capability.Provider{Kind: capability.ProviderKind("company-postgresql"), Capabilities: []capability.Kind{capability.SQL}},
		Trust:         externalprovider.Trust{Mode: externalprovider.TrustSystem},
	}
	if err := RegisterExternalProviderAt(dataDir, reg); err != nil {
		t.Fatal(err)
	}
	store, err := externalProviderStoreAt(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Inspect("company-db")
	if err != nil || got.ID != "company-db" {
		t.Fatalf("inspect=%#v err=%v", got, err)
	}
	registryStore, err := referenceProviderRegistryStoreAt(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := registryStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Instances) != 1 || registry.Instances[0].Ownership != capability.OwnershipExternal {
		t.Fatalf("registry=%#v", registry)
	}
	if err := RemoveExternalProviderAt(dataDir, "company-db"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect("company-db"); err == nil {
		t.Fatal("expected registration to be removed")
	}
	if _, err := filepath.Abs(dataDir); err != nil {
		t.Fatal(err)
	}
}

func TestExternalProviderBindingAndReleasePreserveForeignInstance(t *testing.T) {
	dataDir := t.TempDir()
	reg := externalprovider.Registration{
		ID:            "company-db",
		ProviderID:    "company/postgresql",
		Endpoint:      "postgres://db.example:5432/app",
		CredentialRef: "file:///run/secrets/company-db.json",
		Provider: capability.Provider{
			Kind:         capability.ProviderPostgreSQL,
			Capabilities: []capability.Kind{capability.SQL},
		},
		Trust: externalprovider.Trust{Mode: externalprovider.TrustAuto},
	}
	if err := RegisterExternalProviderAt(dataDir, reg); err != nil {
		t.Fatal(err)
	}

	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "external")
	t.Setenv(ProviderExternalReferenceEnv(capability.ProviderPostgreSQL), "company-db")
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "external-sql-demo",
		Environment:   "dev",
		Services:      Services{SQL: true},
	}
	if err := ReconcileReferenceProviderRegistryAt(dataDir, m); err != nil {
		t.Fatal(err)
	}

	registryStore, err := referenceProviderRegistryStoreAt(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := registryStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 1 {
		t.Fatalf("bindings=%#v", registry.Bindings)
	}
	instanceID := registry.Bindings[0].ProviderInstanceID
	if len(registry.BindingsForProviderInstance(instanceID)) != 1 {
		t.Fatalf("provider bindings=%#v", registry.BindingsForProviderInstance(instanceID))
	}
	if err := RemoveExternalProviderAt(dataDir, "company-db"); err == nil {
		t.Fatal("expected removal to fail while application binding exists")
	}

	if err := ReleaseApplicationProviderRegistryAt(dataDir, m); err != nil {
		t.Fatal(err)
	}
	registry, err = registryStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 0 {
		t.Fatalf("bindings after release=%#v", registry.Bindings)
	}
	found := false
	for _, instance := range registry.Instances {
		if instance.ID == instanceID {
			found = true
			if instance.Ownership != capability.OwnershipExternal {
				t.Fatalf("external instance ownership=%q", instance.Ownership)
			}
		}
	}
	if !found {
		t.Fatal("external provider instance was destroyed by application release")
	}
	if err := RemoveExternalProviderAt(dataDir, "company-db"); err != nil {
		t.Fatal(err)
	}
}
