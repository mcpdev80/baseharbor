package application

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRuntimeRetirementReleasesOnlySelectedReferencesPreservingEveryOwner(t *testing.T) {
	root := t.TempDir()
	app := New("selected", "dev", true, false, false)
	sibling := New("sibling", "dev", true, false, false)
	registry := capability.NewRegistry()
	for _, instance := range []capability.ProviderInstance{
		{ID: "sql/owned", Provider: capability.PostgreSQL, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: app.ApplicationID, OwnerApplication: app.Name, OwnerEnvironment: app.Environment},
		{ID: "sql/shared", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor},
		{ID: "sql/external", Provider: capability.PostgreSQL, Scope: capability.ScopeExternal, Ownership: capability.OwnershipExternal, Reference: "team-database"},
	} {
		if err := registry.Register(instance); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []struct{ name, id, env, provider string }{
		{app.Name, app.ApplicationID, "dev", "sql/owned"},
		{app.Name, app.ApplicationID, "dev", "sql/external"},
		{app.Name, app.ApplicationID, "prod", "sql/shared"},
		{sibling.Name, sibling.ApplicationID, "dev", "sql/shared"},
	} {
		name := "default"
		if binding.provider == "sql/external" {
			name = "external"
		}
		resource := capability.Resource{Application: binding.name, Kind: capability.SQL, Name: name, Provider: capability.ProviderPostgreSQL}
		if err := registry.BindDeployment(resource, binding.id, binding.env, binding.provider); err != nil {
			t.Fatal(err)
		}
	}
	store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
	if err := store.Update(func(actual *capability.Registry) error { *actual = registry; return nil }); err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := ReleaseApplicationProviderReferencesAt(root, app); err != nil {
			t.Fatal(err)
		}
		actual, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual.Instances, before.Instances) || len(actual.Bindings) != 2 {
			t.Fatal("runtime retirement removed provider ownership or another consumer")
		}
		for _, binding := range actual.Bindings {
			if binding.ApplicationID == app.ApplicationID && binding.Environment == "dev" {
				t.Fatal("selected consumer was not released")
			}
		}
	}
}
