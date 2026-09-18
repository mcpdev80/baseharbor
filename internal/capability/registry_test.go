package capability

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRegistryReusesOneSharedProviderAcrossApplications(t *testing.T) {
	registry := NewRegistry()
	shared := ProviderInstance{ID: "openbao/control-plane", Provider: OpenBao, Scope: ScopeShared, Ownership: OwnershipBaseHarbor}
	if err := registry.Register(shared); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(shared); err != nil {
		t.Fatalf("idempotent register: %v", err)
	}
	for _, app := range []string{"alpha", "beta"} {
		resource, err := Resolve(app, Requirement{Kind: Secrets, Name: "default"}, OpenBao)
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Bind(resource, shared.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(registry.Instances) != 1 || len(registry.Bindings) != 2 {
		t.Fatalf("registry=%#v", registry)
	}
}

func TestRegistryRejectsDuplicateSharedProvider(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(ProviderInstance{ID: "openbao/a", Provider: OpenBao, Scope: ScopeShared, Ownership: OwnershipBaseHarbor}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ProviderInstance{ID: "openbao/b", Provider: OpenBao, Scope: ScopeShared, Ownership: OwnershipBaseHarbor}); err == nil {
		t.Fatal("duplicate shared provider accepted")
	}
}

func TestRegistryApplicationProviderCannotCrossApplicationBoundary(t *testing.T) {
	registry := NewRegistry()
	instance := ProviderInstance{ID: "postgresql/alpha/default", Provider: PostgreSQL, Scope: ScopeApplication, Ownership: OwnershipBaseHarbor, OwnerApplication: "alpha"}
	if err := registry.Register(instance); err != nil {
		t.Fatal(err)
	}
	resource, _ := Resolve("beta", Requirement{Kind: SQL, Name: "default"}, PostgreSQL)
	if err := registry.Bind(resource, instance.ID); err == nil {
		t.Fatal("cross-application binding accepted")
	}
}

func TestRegistryExternalProviderIsBindableButNeverLifecycleOwned(t *testing.T) {
	registry := NewRegistry()
	instance := ProviderInstance{ID: "postgresql/customer-prod", Provider: PostgreSQL, Scope: ScopeExternal, Ownership: OwnershipExternal, Reference: "postgres-primary"}
	if err := registry.Register(instance); err != nil {
		t.Fatal(err)
	}
	resource, _ := Resolve("alpha", Requirement{Kind: SQL, Name: "primary"}, PostgreSQL)
	if err := registry.Bind(resource, instance.ID); err != nil {
		t.Fatal(err)
	}
	actions, err := registry.ApplicationLifecycle("alpha", LifecycleDestroy)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].MutateProvider || !actions[0].RemoveBinding {
		t.Fatalf("actions=%#v", actions)
	}
}

func TestRegistryLifecycleOwnsOnlyDedicatedManagedProvider(t *testing.T) {
	registry := NewRegistry()
	dedicated := ProviderInstance{ID: "postgresql/alpha/default", Provider: PostgreSQL, Scope: ScopeApplication, Ownership: OwnershipBaseHarbor, OwnerApplication: "alpha"}
	shared := ProviderInstance{ID: "openbao/control-plane", Provider: OpenBao, Scope: ScopeShared, Ownership: OwnershipBaseHarbor}
	for _, instance := range []ProviderInstance{dedicated, shared} {
		if err := registry.Register(instance); err != nil {
			t.Fatal(err)
		}
	}
	sql, _ := Resolve("alpha", Requirement{Kind: SQL, Name: "default"}, PostgreSQL)
	secrets, _ := Resolve("alpha", Requirement{Kind: Secrets, Name: "default"}, OpenBao)
	if err := registry.Bind(sql, dedicated.ID); err != nil {
		t.Fatal(err)
	}
	if err := registry.Bind(secrets, shared.ID); err != nil {
		t.Fatal(err)
	}
	actions, err := registry.ApplicationLifecycle("alpha", LifecycleBackup)
	if err != nil {
		t.Fatal(err)
	}
	var dedicatedOwned, sharedOwned bool
	for _, action := range actions {
		if action.ProviderInstanceID == dedicated.ID {
			dedicatedOwned = action.MutateProvider
		}
		if action.ProviderInstanceID == shared.ID {
			sharedOwned = action.MutateProvider
		}
	}
	if !dedicatedOwned || sharedOwned {
		t.Fatalf("actions=%#v", actions)
	}
}

func TestRegistryStorePersistsOwnerOnlyValidatedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "provider-registry.json")
	store := RegistryStore{Path: path}
	registry := NewRegistry()
	if err := registry.Register(ProviderInstance{ID: "openbao/control-plane", Provider: OpenBao, Scope: ScopeShared, Ownership: OwnershipBaseHarbor}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("corrupt registry accepted")
	}
}

func TestRegistryStoreSerializesConcurrentUpdates(t *testing.T) {
	store := RegistryStore{Path: filepath.Join(t.TempDir(), "provider-registry.json")}
	instances := []ProviderInstance{
		{ID: "postgresql/alpha/default", Provider: PostgreSQL, Scope: ScopeApplication, Ownership: OwnershipBaseHarbor, OwnerApplication: "alpha"},
		{ID: "postgresql/beta/default", Provider: PostgreSQL, Scope: ScopeApplication, Ownership: OwnershipBaseHarbor, OwnerApplication: "beta"},
	}

	start := make(chan struct{})
	errs := make(chan error, len(instances))
	var wg sync.WaitGroup
	for _, instance := range instances {
		instance := instance
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- store.Update(func(registry *Registry) error {
				return registry.Register(instance)
			})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	registry, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Instances) != 2 {
		t.Fatalf("instances=%#v", registry.Instances)
	}
}

func TestReleaseManagedApplicationPreservesExternalBinding(t *testing.T) {
	registry := NewRegistry()
	managed := ProviderInstance{ID: "postgresql/alpha/default", Provider: PostgreSQL, Scope: ScopeApplication, Ownership: OwnershipBaseHarbor, OwnerApplication: "alpha"}
	external := ProviderInstance{ID: "postgresql/customer", Provider: PostgreSQL, Scope: ScopeExternal, Ownership: OwnershipExternal, Reference: "customer-postgres"}
	for _, instance := range []ProviderInstance{managed, external} {
		if err := registry.Register(instance); err != nil {
			t.Fatal(err)
		}
	}
	managedResource, _ := Resolve("alpha", Requirement{Kind: SQL, Name: "managed"}, PostgreSQL)
	externalResource, _ := Resolve("alpha", Requirement{Kind: SQL, Name: "external"}, PostgreSQL)
	if err := registry.Bind(managedResource, managed.ID); err != nil {
		t.Fatal(err)
	}
	if err := registry.Bind(externalResource, external.ID); err != nil {
		t.Fatal(err)
	}

	registry.ReleaseManagedApplication("alpha")

	if _, ok := registry.instance(managed.ID); ok {
		t.Fatal("managed application provider was retained")
	}
	if _, ok := registry.instance(external.ID); !ok {
		t.Fatal("external provider was removed")
	}
	if len(registry.Bindings) != 1 || registry.Bindings[0].ProviderInstanceID != external.ID {
		t.Fatalf("bindings=%#v", registry.Bindings)
	}

	registry.ReleaseApplication("alpha")
	if len(registry.Bindings) != 0 {
		t.Fatalf("destroy bindings=%#v", registry.Bindings)
	}
	if _, ok := registry.instance(external.ID); !ok {
		t.Fatal("destroy removed external provider instance")
	}
}
