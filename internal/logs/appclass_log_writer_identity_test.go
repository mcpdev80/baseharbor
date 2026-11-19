package logs_test

// Correction-round regression for card 0428-122 (CODEX-REVIEW REQUEST_CHANGES
// at 8956bf08e, recipe in the review comment on PR #1692): the established
// runtime log identity keeps the SERVICE dimension - a log source is
// identified semantically by application, environment, source class, provider
// AND service (docs/architecture/runtime-log-source-contract.md), and the
// registry ID format observability.ApplicationLogSourceID already carries that
// token. Two workload services of the same application/environment therefore
// are two distinct sources: both must stay distinguishable and visible across
// repeated render-time reconciliation and every Loki driver Bind, and a
// service that disappears must have exactly its own source removed by the next
// reconcile while the sibling survives untouched.

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// appClassWorkloadManifest is the two-service shape of the review: identical
// app class ("application"), different service ids.
func appClassWorkloadManifest(t *testing.T, services ...string) (application.Manifest, string) {
	t.Helper()
	m := application.WithLogsCollection(application.New("demo", "dev", false, false, false), "application")
	m = application.WithWorkloadComponents(m, services...)
	return m, application.WorkloadProjectName(m)
}

func wantApplicationLogSources(project string, services ...string) map[string]string {
	want := make(map[string]string, len(services))
	for _, service := range services {
		want[observability.ApplicationLogSourceID("demo", "dev", service)] = service
	}
	return want
}

// assertApplicationLogSources reads the application-class listing boundary its
// consumers read (logs.ListWorkloadLogSources) and requires exactly the wanted
// service set: one distinguishable ID per service, each targeting its own
// runtime service.
func mustWorkloadLogSources(t *testing.T, m application.Manifest) []observability.SignalSource {
	t.Helper()
	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	sources, err := logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatalf("workload log listing: %v", err)
	}
	return sources
}

func assertApplicationLogSources(t *testing.T, m application.Manifest, want map[string]string) {
	t.Helper()
	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	sources, err := logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatalf("workload log listing: %v", err)
	}
	if len(sources) != len(want) {
		t.Fatalf("workload log sources = %d (%#v), want %d", len(sources), sources, len(want))
	}
	for _, source := range sources {
		service, ok := want[source.ID]
		if !ok {
			t.Fatalf("unexpected workload log source %#v, want only %v", source, want)
		}
		if source.Class != observability.SourceApplication || source.Kind != observability.SignalLogs ||
			source.Provider != capability.ProviderLoki {
			t.Fatalf("workload log source %q has kind/class/provider %q/%q/%q", source.ID, source.Kind, source.Class, source.Provider)
		}
		if source.Protocol != "stdout-stderr" || source.Verification != capability.ObservabilityVerifyBackend {
			t.Fatalf("workload log source %q has protocol/verification %q/%q", source.ID, source.Protocol, source.Verification)
		}
		gotProject, gotService, ok := observability.ParseRuntimeTarget(source.Target)
		if !ok || gotProject != application.WorkloadProjectName(m) || gotService != service {
			t.Fatalf("workload log source %q targets %q, want runtime://%s/%s", source.ID, source.Target, application.WorkloadProjectName(m), service)
		}
	}
}

// TestApplicationClassLogSourcesStayDistinguishablePerService drives the real
// path with two services of identical app class and different service ids: the
// render-time reconciliation over the deployment's workload services, then the
// per-service Bind registration, both repeatedly. Both sources must survive
// every round, and the registry must hold exactly one entry per service.
func TestApplicationClassLogSourcesStayDistinguishablePerService(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassWorkloadManifest(t, "api", "worker")
	want := wantApplicationLogSources(project, "api", "worker")

	for round := 1; round <= 2; round++ {
		if err := application.ReconcileManagedRuntimeObservability(m, project, ""); err != nil {
			t.Fatalf("render-time reconcile round %d: %v", round, err)
		}
		assertApplicationLogSources(t, m, want)
		// The Loki driver Bind path registers one concrete service at a time
		// and must never overwrite a sibling service's source.
		for _, service := range []string{"api", "worker"} {
			if err := logs.ReconcileWorkloadLogSource(m, project, service); err != nil {
				t.Fatalf("bind reconcile round %d service %q: %v", round, service, err)
			}
		}
		assertApplicationLogSources(t, m, want)
	}
}

// TestApplicationClassLogSourceReconcileRegistersActiveServiceSet proves the
// reconcile boundary registers the ACTUALLY active service set: a service that
// disappears loses exactly its own source at the next reconcile, a service
// that appears is registered instead of swallowed by the previous keys.
func TestApplicationClassLogSourceReconcileRegistersActiveServiceSet(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassWorkloadManifest(t, "api", "worker")
	if err := application.ReconcileManagedRuntimeObservability(m, project, ""); err != nil {
		t.Fatalf("render-time reconcile: %v", err)
	}
	assertApplicationLogSources(t, m, wantApplicationLogSources(project, "api", "worker"))

	// The deployment loses its second service: its source must be gone, the
	// surviving service's source must be intact.
	m = application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "api")
	m = application.WithLogsCollection(m, "application")
	if err := application.ReconcileManagedRuntimeObservability(m, project, ""); err != nil {
		t.Fatalf("render-time reconcile after the service was removed: %v", err)
	}
	assertApplicationLogSources(t, m, wantApplicationLogSources(project, "api"))
	if present, err := observability.RegistryHasSignal(observability.ApplicationLogSourceID("demo", "dev", "worker"), capability.ProviderLoki, observability.SignalLogs); err != nil || present {
		t.Fatalf("the removed service's source survived the reconcile (present=%v err=%v)", present, err)
	}

	// A service that appears later must be registered, not swallowed.
	m = application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "api", "cron")
	m = application.WithLogsCollection(m, "application")
	if err := application.ReconcileManagedRuntimeObservability(m, project, ""); err != nil {
		t.Fatalf("render-time reconcile after a new service appeared: %v", err)
	}
	assertApplicationLogSources(t, m, wantApplicationLogSources(project, "api", "cron"))
}

// TestApplicationClassLogSourceBindKeepsSiblings pins the per-registration
// form of the boundary: binding a service that was never registered adds its
// own source and keeps the sibling's; binding it again stays exactly one entry.
func TestApplicationClassLogSourceBindKeepsSiblings(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassWorkloadManifest(t, "api")
	if err := application.ReconcileManagedRuntimeObservability(m, project, ""); err != nil {
		t.Fatalf("render-time reconcile: %v", err)
	}
	assertApplicationLogSources(t, m, wantApplicationLogSources(project, "api"))

	for round := 1; round <= 2; round++ {
		if err := logs.ReconcileWorkloadLogSource(m, project, "worker"); err != nil {
			t.Fatalf("bind the second service round %d: %v", round, err)
		}
		assertApplicationLogSources(t, m, wantApplicationLogSources(project, "api", "worker"))
	}
}

// TestApplicationClassLogSourceDisabledPolicyReclaimsEveryService keeps the
// existing reclaim contract on the new, service-carrying identity: a disabled
// logs policy reclaims the whole application/environment space. The
// application-class listing must be empty while the application-provider
// reference class of the same deployment is still collected: reclaim is
// bounded by the service-carrying class-application identity, never by the
// application/environment as a whole.
func TestApplicationClassLogSourceDisabledPolicyReclaimsEveryService(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassWorkloadManifest(t, "api", "worker")
	// The reference class of the matrix: an application-provider log source of
	// the SAME application/environment that the class-application reclaim must
	// never touch. The generic reconciliation registers the managed PostgreSQL
	// instance only when the app is private and does not share the backend.
	m.Services.SQL = true
	m.Private = true
	if err := application.ReconcileManagedRuntimeObservability(m, project, ""); err != nil {
		t.Fatalf("render-time reconcile: %v", err)
	}
	assertApplicationLogSources(t, m, wantApplicationLogSources(project, "api", "worker"))
	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	providerBefore, err := observability.ListLogs(placement, []string{m.Name}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(providerBefore) == 0 {
		t.Fatalf("the application-provider reference class is absent before the policy disable: application-class sources %#v", mustWorkloadLogSources(t, m))
	}

	t.Setenv(application.LogsCollectSourcesEnv, "application-provider")
	if err := application.ReconcileManagedRuntimeObservability(m, project, ""); err != nil {
		t.Fatalf("disabled-policy reconcile: %v", err)
	}
	assertApplicationLogSources(t, m, wantApplicationLogSources(project))
	providerAfter, err := observability.ListLogs(placement, []string{m.Name}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(providerAfter) != len(providerBefore) {
		t.Fatalf("the class-application reclaim disturbed the application-provider class: %d before, %d after", len(providerBefore), len(providerAfter))
	}
}
