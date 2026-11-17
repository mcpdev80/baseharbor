package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// workloadOnlySourceContract mirrors the source that reconcileApplicationMetricsSource
// emits in the deploy path (reconcileManagedRuntimeObservability, run during
// EnsureRuntime) for a workload-only app: an application-scoped, application-
// provider metrics source owned by the app against the runtime workload target.
func workloadOnlySourceContract(m application.Manifest, project string) observability.MetricsSource {
	return observability.MetricsSource{
		ID:               "prometheus:" + project,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           observability.RuntimeTarget(project, "workload"),
		Path:             "/metrics",
	}
}

// TestWorkloadOnlyRegistersSharedSourceOnDeploy reproduces and then fixes the
// round-1 gap on the real deploy path. It materializes the shared Prometheus
// provider via EnsureProviderFiles after the workload-only app has registered
// its own application-provider metrics source (as reconcileManagedRuntime-
// Observability does during EnsureRuntime). The shared runtime-sources.json
// snapshot must carry at least the app's own source, so metrics verification
// resolves the app's presence instead of an empty snapshot.
func TestWorkloadOnlyRegistersSharedSourceOnDeploy(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")

	m := application.New("workload-only", "dev", false, false, false)
	if application.HasMetricsSources(m) || application.HasRuntimeMetricsPermissions(m) {
		t.Fatal("fixture: workload-only app must have no metrics sources and no runtime metrics permission")
	}

	project := "baseharbor-local-workload-only-dev"
	if err := observability.Update(workloadOnlySourceContract(m, project)); err != nil {
		t.Fatalf("register workload-only metrics source: %v", err)
	}

	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), m)
	if err != nil {
		t.Fatalf("EnsureProviderFiles for workload-only app: %v", err)
	}

	snapshotPath := filepath.Join(files.Dir, "runtime-sources.json")
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read runtime-sources.json: %v", err)
	}

	var snapshot struct {
		Version string `json:"version"`
		Project string `json:"project"`
		Scope   string `json:"scope"`
		Sources []struct {
			ID               string `json:"ID"`
			Class            string `json:"Class"`
			Scope            string `json:"Scope"`
			OwnerApplication string `json:"OwnerApplication"`
			Target           string `json:"Target"`
			Path             string `json:"Path"`
			Scheme           string `json:"Scheme"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("unmarshal runtime-sources.json: %v", err)
	}
	t.Logf("runtime-sources.json = %s", string(data))

	if got := len(snapshot.Sources); got == 0 {
		t.Fatalf("workload-only deploy registers zero metrics sources; want >= 1; snapshot=%s", string(data))
	}
	for _, s := range snapshot.Sources {
		if s.OwnerApplication != m.Name {
			t.Errorf("snapshot source %q owner = %q, want %q", s.ID, s.OwnerApplication, m.Name)
		}
		if s.ID != "prometheus:"+project {
			t.Errorf("snapshot source id = %q, want %q", s.ID, "prometheus:"+project)
		}
	}
}

// TestServiceAppStillRegistersOnDeploy verifies that a service app still
// registers its application-provider source through EnsureProviderFiles and the
// shared snapshot, proving the workload-only registration fix does not regress
// the service-app deploy path.
func TestServiceAppStillRegistersOnDeploy(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")

	m := application.New("svc", "dev", true, true, false)
	m = application.WithMetricsSource(m, "default", "db", 5432, "/metrics")
	if !application.HasMetricsSources(m) {
		t.Fatal("fixture: service app must have metrics sources")
	}

	project := "baseharbor-local-svc-dev"
	if err := observability.Update(workloadOnlySourceContract(m, project)); err != nil {
		t.Fatalf("register service-app source: %v", err)
	}

	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), m)
	if err != nil {
		t.Fatalf("EnsureProviderFiles for service app: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(files.Dir, "runtime-sources.json"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	var snapshot struct {
		Sources []struct {
			ID               string `json:"ID"`
			OwnerApplication string `json:"OwnerApplication"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	t.Logf("service-app snapshot carries %d source(s)", len(snapshot.Sources))
	if got := len(snapshot.Sources); got == 0 {
		t.Fatalf("service app registers zero sources; snapshot=%s", string(data))
	}
	for _, s := range snapshot.Sources {
		if s.OwnerApplication != m.Name {
			t.Errorf("service-app snapshot source %q owner = %q, want %q", s.ID, s.OwnerApplication, m.Name)
		}
	}
}
