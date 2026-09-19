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

func TestRegisterReferenceProvidersMetricsRespectsDeploymentPolicy(t *testing.T) {
	m := New("demo", "production", false, false, false)
	m.Services.Postgres = false
	m = WithWorkload(m, "compose.yaml", "api")
	m = WithMetricsSource(m, "application", "api", 8080, "/metrics")

	t.Setenv(MetricsEnabledEnv, "false")
	disabled := capability.NewRegistry()
	if err := registerReferenceProviders(&disabled, m); err != nil {
		t.Fatal(err)
	}
	if _, err := disabled.Resolve(capability.ProviderPrometheus, capability.ScopeShared, m.Name, ""); err == nil {
		t.Fatal("Prometheus registered while metrics collection policy is disabled")
	}

	t.Setenv(MetricsEnabledEnv, "true")
	enabled := capability.NewRegistry()
	if err := registerReferenceProviders(&enabled, m); err != nil {
		t.Fatal(err)
	}
	instance, err := enabled.Resolve(capability.ProviderPrometheus, capability.ScopeShared, m.Name, "")
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID != "prometheus/shared" {
		t.Fatalf("Prometheus instance = %#v", instance)
	}
}

func TestRegisterReferenceProvidersPersistsRuntimeOnlyMetricsPlacement(t *testing.T) {
	t.Setenv(MetricsEnabledEnv, "true")
	m := New("runtime-metrics", "dev", false, false, false)
	m.Services.Postgres = false
	m = WithWorkload(m, "compose.yaml", "api")
	m = WithRuntimePermission(m, string(capability.MetricsV1.ID), []string{"api"}, "runtime.create", "runtime.get", "runtime.delete")

	registry := capability.NewRegistry()
	if err := registerReferenceProviders(&registry, m); err != nil {
		t.Fatal(err)
	}

	if len(registry.Instances) != 1 {
		t.Fatalf("instances=%#v", registry.Instances)
	}
	if len(registry.Bindings) != 1 {
		t.Fatalf("bindings=%#v", registry.Bindings)
	}
	binding := registry.Bindings[0]
	if binding.Resource.Kind != capability.Metrics ||
		binding.Resource.Name != runtimeMetricsRegistryResource ||
		binding.ProviderInstanceID != "prometheus/shared" {
		t.Fatalf("runtime metrics binding=%#v", binding)
	}
}

func TestRegisteredProviderPlacementSurvivesDesiredOverrideChange(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(MetricsEnabledEnv, "true")

	m := New("demo", "dev", false, false, false)
	m.Services.Postgres = false
	m = WithWorkload(m, "compose.yaml", "api")
	m = WithMetricsSource(m, "application", "api", 8080, "/metrics")

	if err := ReconcileReferenceProviderRegistry(m); err != nil {
		t.Fatal(err)
	}

	t.Setenv(ProviderScopeEnv(capability.ProviderPrometheus), "application")
	desired, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		t.Fatal(err)
	}
	if desired.Scope != capability.ScopeApplication {
		t.Fatalf("desired placement=%#v", desired)
	}

	registered, found, err := RegisteredProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("registered Prometheus placement not found")
	}
	if registered.Scope != capability.ScopeShared {
		t.Fatalf("registered placement changed with desired override: %#v", registered)
	}
}

