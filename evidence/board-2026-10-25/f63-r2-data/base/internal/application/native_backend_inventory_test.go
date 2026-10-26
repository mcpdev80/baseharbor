package application

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestNativeBackendInventoryUsesExactNamedInstancesAndRetainsHAResources(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	m := WithWorkloadComponents(New("demo", "dev", false, false, false), "api")
	m = WithCacheInstances(m, "first", "second")
	ha := true
	m = WithAvailabilityOverride(m, "cache", &ha, 3)
	files := RuntimeFilesFor(Store{Root: t.TempDir(), Namespace: "isolated"}, m)
	all, err := NativeBackendResources(m, files)
	if err != nil {
		t.Fatal(err)
	}
	r := capability.NewRegistry()
	if err := registerReferenceProviders(&r, m); err != nil {
		t.Fatal(err)
	}
	if len(r.Instances) != 2 {
		t.Fatalf("named registry: %#v", r)
	}
	first, err := NativeBackendInstanceResources(m, files, r.Instances[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := NativeBackendInstanceResources(m, files, r.Instances[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || len(second) == 0 || reflect.DeepEqual(first, second) {
		t.Fatal("named instances are missing or share a fabricated inventory")
	}
	for _, one := range first {
		if one.Kind == "container" || one.Kind == "volume" {
			for _, two := range second {
				if one == two {
					t.Fatalf("peer inherits resource %#v", one)
				}
			}
		}
	}
	for _, resource := range append(first, second...) {
		if !ResourceNamedExists(all, resource) {
			t.Fatalf("instance invented resource %#v", resource)
		}
	}
	networks := 0
	for _, resource := range all {
		if resource.Kind == "network" {
			networks++
		}
	}
	if networks != 3 {
		t.Fatalf("HA networks missing: %#v", all)
	}
}

func TestNativeBackendInventoryChecksRetainedOrDesiredCanonicalDefinition(t *testing.T) {
	m := WithWorkloadComponents(New("demo", "dev", false, false, false), "api")
	m = WithDocumentDatabaseInstances(m, "records")
	files := RuntimeFilesFor(Store{Root: t.TempDir(), Namespace: "isolated"}, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("protected=unchanged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	desired := m
	desired.Services.DocumentDatabase, desired.Services.DocumentDatabaseInstances = false, nil
	for _, intent := range []Manifest{m, desired} {
		canonical, err := RuntimeComposeYAMLForProject(intent, files.ResourceProject)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
			t.Fatal(err)
		}
		if err := CheckExistingNativeBackendInventory(files, m, desired); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(files.Compose, []byte("services:\n  foreign:\n    image: other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckExistingNativeBackendInventory(files, m, desired); !errors.Is(err, ErrRuntimeDefinitionChanged) {
		t.Fatalf("modified owner accepted: %v", err)
	}
	if err := os.Remove(files.Compose); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign.yaml")
	canonical, _ := RuntimeComposeYAMLForProject(m, files.ResourceProject)
	if err := os.WriteFile(foreign, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, files.Compose); err != nil {
		t.Fatal(err)
	}
	if err := CheckExistingNativeBackendInventory(files, m); err == nil {
		t.Fatal("symlink owner accepted")
	}
}

func TestNativeBackendInventoryIncludesApplicationScopedDurableKeyValue(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	m := WithKeyValueInstances(WithWorkloadComponents(New("demo", "dev", false, false, false), "api"), "durable")
	if !HasApplicationScopedRuntimeServices(m) {
		t.Fatal("durable key-value native runtime omitted")
	}
	resources, err := NativeBackendResources(m, RuntimeFilesFor(Store{Root: t.TempDir()}, m))
	if err != nil || len(resources) < 3 {
		t.Fatalf("durable native inventory: %#v %v", resources, err)
	}
}
