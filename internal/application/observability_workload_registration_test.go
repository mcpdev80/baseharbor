package application

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// TestReconcileManagedRuntimeObservabilityWorkloadOnlyRegistersSource verifies
// the deploy-path registration: a workload-only app (no SQL/Valkey backend, no
// runtime broker, no runtime metrics permission) declares an application-scoped
// metrics source through reconcileManagedRuntimeObservability, the real hook
// that EnsureRuntime runs during apply.
func TestReconcileManagedRuntimeObservabilityWorkloadOnlyRegistersSource(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_METRICS_ENABLED", "true")

	m := New("workload-only", "dev", false, false, false)
	if HasMetricsSources(m) || HasRuntimeMetricsPermissions(m) {
		t.Fatal("fixture: workload-only app must have no metrics sources and no runtime metrics permission")
	}

	project := "baseharbor-local-workload-only-dev"
	if err := reconcileManagedRuntimeObservability(m, project); err != nil {
		t.Fatalf("reconcile runtime observability: %v", err)
	}

	sources, err := observability.ListMetrics(
		capability.ProviderPlacement{Scope: capability.ScopeShared, SharingBoundary: "shared"},
		[]string{m.Name},
		true,
		true,
	)
	if err != nil {
		t.Fatalf("list metrics: %v", err)
	}
	if len(sources) == 0 {
		t.Fatalf("workload-only app registered zero metrics sources; snapshot stays empty")
	}
	for _, s := range sources {
		if s.OwnerApplication != m.Name {
			t.Errorf("registered source %q owner = %q, want %q", s.ID, s.OwnerApplication, m.Name)
		}
		if s.ID != "prometheus:"+project {
			t.Errorf("registered source id = %q, want %q", s.ID, "prometheus:"+project)
		}
	}
}
