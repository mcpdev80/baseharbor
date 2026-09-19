package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRegisterReferenceProvidersMapsCurrentOwnership(t *testing.T) {
	registry := capability.NewRegistry()
	alpha := New("alpha", "production", true, true, true)
	beta := New("beta", "production", false, false, true)

	if err := registerReferenceProviders(&registry, alpha); err != nil {
		t.Fatal(err)
	}
	if err := registerReferenceProviders(&registry, beta); err != nil {
		t.Fatal(err)
	}

	if len(registry.Instances) != 3 {
		t.Fatalf("instances=%#v", registry.Instances)
	}
	shared, err := registry.Resolve(capability.ProviderOpenBao, capability.ScopeShared, "beta", "")
	if err != nil {
		t.Fatal(err)
	}
	if shared.ID != "openbao/control-plane" {
		t.Fatalf("shared=%#v", shared)
	}
	pg, err := registry.Resolve(capability.ProviderPostgreSQL, capability.ScopeApplication, "alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if pg.OwnerApplication != "alpha" {
		t.Fatalf("postgres=%#v", pg)
	}
	if _, err := registry.Resolve(capability.ProviderPostgreSQL, capability.ScopeApplication, "beta", ""); err == nil {
		t.Fatal("beta unexpectedly resolved alpha dedicated PostgreSQL")
	}
}

func TestCheckReferenceProviderRegistryRejectsCorruptStateBeforeReconcile(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	if err := os.WriteFile(filepath.Join(stateDir, "provider-registry.json"), []byte(`{
  "version": 99,
  "instances": [],
  "bindings": []
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	err := CheckReferenceProviderRegistry(New("demo", "dev", true, false, false))
	if err == nil || !strings.Contains(err.Error(), "unsupported provider registry version 99") {
		t.Fatalf("expected corrupt registry rejection, got %v", err)
	}
}

func TestCheckControlPlaneDestroySafeRejectsApplicationBindings(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	registry := capability.NewRegistry()
	m := New("demo", "dev", true, false, false)
	if err := registerReferenceProviders(&registry, m); err != nil {
		t.Fatal(err)
	}
	store := capability.RegistryStore{Path: filepath.Join(stateDir, "provider-registry.json")}
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}

	err := CheckControlPlaneDestroySafe()
	if err == nil || !strings.Contains(err.Error(), "application binding") {
		t.Fatalf("expected application binding guard, got %v", err)
	}

	registry.ReleaseApplication("demo")
	if err := store.Save(registry); err != nil {
		t.Fatal(err)
	}
	if err := CheckControlPlaneDestroySafe(); err != nil {
		t.Fatalf("shared-only registry should be safe to destroy: %v", err)
	}
}

func TestRegisterReferenceProvidersTracksApplicationScopedCaddy(t *testing.T) {
	registry := capability.NewRegistry()
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "frontend",
		Environment: "production",
		Workload:    WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}},
		Exposures:   []HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http"}},
	}
	if err := registerReferenceProviders(&registry, m); err != nil {
		t.Fatal(err)
	}
	instance, err := registry.Resolve(capability.ProviderCaddy, capability.ScopeApplication, "frontend", "")
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID != "caddy/frontend/production" || instance.OwnerApplication != "frontend" {
		t.Fatalf("unexpected Caddy instance %#v", instance)
	}
	found := false
	for _, binding := range registry.Bindings {
		if binding.Resource.Kind == capability.ExposureHTTP && binding.Resource.Name == "public" && binding.ProviderInstanceID == instance.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("exposure provider binding missing: %#v", registry.Bindings)
	}
}
