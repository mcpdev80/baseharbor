package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workloadOnlyManifest() Manifest {
	return Manifest{
		Version:     CurrentVersion,
		Name:        "awc",
		Environment: "production",
		Services: Services{
			Postgres: false,
			Redis:    false,
			Secrets:  false,
		},
		Workload: WorkloadConfig{
			Compose:  "docker-compose.yml",
			Services: []string{"coordinator", "docker-engine", "web"},
		},
	}
}

func TestWorkloadOnlyManifestIsValid(t *testing.T) {
	m := workloadOnlyManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := CheckSupportedRuntimeServices(m); err != nil {
		t.Fatalf("CheckSupportedRuntimeServices() error = %v", err)
	}
	if HasManagedRuntimeServices(m) {
		t.Fatal("workload-only application must not report managed runtime services")
	}
	if !HasExplicitWorkload(m) {
		t.Fatal("workload-only application must report its explicit workload")
	}
}

func TestManifestWithoutBackendOrExplicitWorkloadStillFails(t *testing.T) {
	m := Manifest{Version: CurrentVersion, Name: "empty", Environment: "dev"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected empty application contract to be rejected")
	}
}

func TestSecretsOnlyApplicationStillFailsClosed(t *testing.T) {
	m := Manifest{
		Version:     CurrentVersion,
		Name:        "secret-only",
		Environment: "dev",
		Services:    Services{Secrets: true},
		Workload:    WorkloadConfig{Compose: "compose.yaml", Services: []string{"api"}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest syntax should be valid before runtime capability check: %v", err)
	}
	if err := CheckSupportedRuntimeServices(m); err == nil {
		t.Fatal("managed secrets without PostgreSQL or Valkey must remain unsupported")
	}
}

func TestWorkloadOnlyRuntimeMaterializationDoesNotInventBackend(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	m := workloadOnlyManifest()

	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatalf("EnsureRuntime() error = %v", err)
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(compose); got != "services: {}\n" {
		t.Fatalf("unexpected workload-only managed compose:\n%s", got)
	}
	env, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 0 {
		t.Fatalf("workload-only runtime env must be empty, got %q", string(env))
	}
	appEnv, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(appEnv) != 0 {
		t.Fatalf("workload-only application env must be empty, got %q", string(appEnv))
	}
	if got := ExpectedRuntimeResources(m); len(got) != 0 {
		t.Fatalf("workload-only app must not claim managed runtime resources: %#v", got)
	}
}

func TestWorkloadOnlyOverridePreservesApplicationNetworksWithoutFakeBackend(t *testing.T) {
	m := workloadOnlyManifest()
	override, err := workloadOverrideYAML(m, []string{"coordinator", "web"}, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	for _, unexpected := range []string{"baseharbor-backend", "DATABASE_URL", "REDIS_URL", "VALKEY_URL"} {
		if strings.Contains(override, unexpected) {
			t.Fatalf("workload-only override unexpectedly contains %q:\n%s", unexpected, override)
		}
	}
	for _, service := range []string{"coordinator:", "web:"} {
		if !strings.Contains(override, service) {
			t.Fatalf("workload-only override missing service %q:\n%s", service, override)
		}
	}
}

func TestWorkloadOnlyPlanContainsOnlyRepositoryWorkload(t *testing.T) {
	m := workloadOnlyManifest()
	plan, err := BuildPlan(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected one workload action, got %#v", plan.Actions)
	}
	if plan.Actions[0].Resource != "workload" {
		t.Fatalf("unexpected workload-only plan action %#v", plan.Actions[0])
	}
}
