package metrics_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// deployRuntimeObservability exercises the same application runtime setup and provider
// materialization sequence used by deploy before checking the shared source snapshot.
func deployRuntimeObservability(t *testing.T, m application.Manifest) (string, application.RuntimeFiles, metrics.ProviderFiles) {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	store := application.Store{Root: filepath.Join(stateDir, "apps")}
	runtimeFiles, err := application.EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatalf("EnsureRuntime: %v", err)
	}
	providerFiles, err := metrics.EnsureProviderFiles(context.Background(), serviceissuer.New(t), m)
	if err != nil {
		t.Fatalf("EnsureProviderFiles: %v", err)
	}
	return stateDir, runtimeFiles, providerFiles
}

func providerSnapshot(t *testing.T, files metrics.ProviderFiles) []observability.MetricsSource {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(files.Dir, "runtime-sources.json"))
	if err != nil {
		t.Fatalf("read runtime-sources.json: %v", err)
	}
	var snapshot struct {
		Sources []observability.MetricsSource `json:"sources"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("unmarshal runtime-sources.json: %v", err)
	}
	return snapshot.Sources
}

func assertWorkloadSnapshot(t *testing.T, sources []observability.MetricsSource, m application.Manifest, project string) {
	t.Helper()
	id := "prometheus:" + project
	for _, source := range sources {
		if source.ID != id {
			continue
		}
		if source.OwnerApplication != m.Name {
			t.Fatalf("source owner = %q, want %q", source.OwnerApplication, m.Name)
		}
		if source.Target != observability.RuntimeTarget(project, "workload") {
			t.Fatalf("source target = %q, want symbolic workload target", source.Target)
		}
		return
	}
	t.Fatalf("runtime-sources.json has no workload source %q: %+v", id, sources)
}

// assertNoWorkloadComposeNetwork scans the real provider Compose file and fails
// if the snapshot-only workload provider network is attached to Prometheus.
func assertNoWorkloadComposeNetwork(t *testing.T, files metrics.ProviderFiles, network string) {
	t.Helper()
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatalf("read provider compose file: %v", err)
	}
	if strings.Contains(string(data), network) {
		t.Fatalf("snapshot-only workload network %q was attached to Prometheus Compose: %s", network, data)
	}
}

func assertNoWorkloadScrapeTarget(t *testing.T, files metrics.ProviderFiles, sourceID string) {
	t.Helper()
	entries, err := os.ReadDir(files.TargetsDir)
	if err != nil {
		t.Fatalf("read provider targets: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(files.TargetsDir, entry.Name()))
		if err != nil {
			t.Fatalf("read provider target %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(data), sourceID) {
			t.Fatalf("symbolic workload source %q was written as a Prometheus file_sd target: %s", sourceID, data)
		}
	}
}

func TestWorkloadOnlyRegistersSharedSnapshotOnDeploy(t *testing.T) {
	m := application.WithWorkloadComponents(application.New("workload-only", "dev", false, false, false), "api")
	if application.HasMetricsSources(m) || application.HasRuntimeMetricsPermissions(m) {
		t.Fatal("fixture must have no declared application metrics sources or runtime metrics permissions")
	}

	stateDir, runtimeFiles, providerFiles := deployRuntimeObservability(t, m)
	sourceID := "prometheus:" + runtimeFiles.Project
	sources := providerSnapshot(t, providerFiles)
	assertWorkloadSnapshot(t, sources, m, runtimeFiles.Project)
	assertNoWorkloadScrapeTarget(t, providerFiles, sourceID)
	assertNoWorkloadComposeNetwork(t, providerFiles, application.MetricsProviderNetworkNameForNamespace(m, ""))

	if err := metrics.VerifyProviderSourcesAt(context.Background(), m, stateDir, ""); err != nil {
		t.Fatalf("verify deployed workload-only provider snapshot: %v", err)
	}
}

func TestServiceAppKeepsRuntimeSourceRegistrationOnDeploy(t *testing.T) {
	m := application.WithWorkloadComponents(application.New("service-app", "dev", true, false, false), "api")
	m = application.WithMetricsSource(m, "default", "api", 5432, "/metrics")
	if !application.HasMetricsSources(m) {
		t.Fatal("fixture must retain its declared service metrics source")
	}

	stateDir, runtimeFiles, providerFiles := deployRuntimeObservability(t, m)
	sourceID := "prometheus:" + runtimeFiles.Project
	sources := providerSnapshot(t, providerFiles)
	assertWorkloadSnapshot(t, sources, m, runtimeFiles.Project)
	assertNoWorkloadScrapeTarget(t, providerFiles, sourceID)
	if data, err := os.ReadFile(providerFiles.Compose); err != nil {
		t.Fatalf("read provider compose file: %v", err)
	} else if !strings.Contains(string(data), application.MetricsProviderNetworkNameForNamespace(m, "")) {
		t.Fatal("service application metrics network was not attached to Prometheus Compose")
	}

	if err := metrics.VerifyProviderSourcesAt(context.Background(), m, stateDir, ""); err != nil {
		t.Fatalf("verify service app provider snapshot: %v", err)
	}
}
