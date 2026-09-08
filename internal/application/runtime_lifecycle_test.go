package application

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExpectedPostgresRuntimeResourcesAreProjectScoped(t *testing.T) {
	m := New("mailflow", "prod", true, false, false)
	resources := ExpectedPostgresRuntimeResources(m)
	wantNames := []string{
		"baseharbor-mailflow-prod-postgres-1",
		"baseharbor-mailflow-prod_default",
		"baseharbor-mailflow-prod_postgres-data",
	}
	if len(resources) != len(wantNames) {
		t.Fatalf("expected %d resources, got %d", len(wantNames), len(resources))
	}
	for i, resource := range resources {
		if resource.Name != wantNames[i] {
			t.Fatalf("unexpected %s resource %q", resource.Kind, resource.Name)
		}
	}
}

func TestCheckManagedRuntimeDefinitionRejectsModifiedCompose(t *testing.T) {
	dir := t.TempDir()
	files := RuntimeFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env")}
	if err := os.WriteFile(files.Compose, []byte(postgresComposeYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckManagedRuntimeDefinition(files); err != nil {
		t.Fatalf("expected generated definition to pass: %v", err)
	}
	if err := os.WriteFile(files.Compose, []byte(postgresComposeYAML+"# modified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckManagedRuntimeDefinition(files); !errors.Is(err, ErrRuntimeDefinitionChanged) {
		t.Fatalf("expected ErrRuntimeDefinitionChanged, got %v", err)
	}
}

func TestStoreDeleteOnlyRemovesRequestedApplication(t *testing.T) {
	store := Store{Root: filepath.Join(t.TempDir(), "apps")}
	for _, name := range []string{"one", "two"} {
		if _, err := store.Create(New(name, "dev", true, false, false)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Delete("one"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load("one"); err == nil {
		t.Fatal("deleted application still loads")
	}
	if _, _, err := store.Load("two"); err != nil {
		t.Fatalf("unrelated application was affected: %v", err)
	}
}
