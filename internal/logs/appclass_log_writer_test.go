package logs_test

// Regression for card 0428-122 (hunt-918 Befund 3, shape 1): the documented
// normalized log-source class "application" had NO production writer. Before
// the fix, a deploy with log collection enabled persisted a workload
// registration plus an Alloy listener labelled baseharbor_source_class=
// "application" while the shared signal registry stayed empty of
// application-class log sources, and every provider log listing silently
// dropped the class - the silent success these tests make fail visibly at the
// exact boundaries: the reconciliation boundary (ReconcileApplicationLogSource
// writes the source for both realization transports and reclaims it on a
// disabled policy), the listing boundary (class application reaches callers
// that collect application logs), and the write-key boundary (ReconcileMatching
// must not let a caller delete a same-ID signal of another provider).

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

func appClassManifest(t *testing.T) (application.Manifest, string) {
	t.Helper()
	m := application.New("demo", "dev", false, false, false)
	m = application.WithLogsCollection(m, "application")
	return m, application.WorkloadProjectName(m)
}

// TestApplicationClassLogSourceSurvivesManagedReconcile is the full matrix
// cell: the reference class (an application-provider log signal) is
// registered for the SAME application/environment/provider identity space as
// the workload signal, and every application-class write boundary - the
// render-time reconciliation and the collector-side converge entry - must
// leave it untouched while the workload signal stays exactly one entry.
// Before card 0428-122 the question was moot because no log writer existed;
// a naive ID-keyed or provider-keyed writer would fail it today.
func TestApplicationClassLogSourceSurvivesManagedReconcile(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassManifest(t)
	applicationLogID := observability.ApplicationLogSourceID(m.Name, m.Environment, "workload")

	if err := observability.UpdateSignal(observability.SignalSource{
		ID:                 "loki:" + project,
		Kind:               observability.SignalLogs,
		Provider:           capability.ProviderLoki,
		Class:              observability.SourceApplicationProvider,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   m.Name,
		Target:             observability.RuntimeTarget(project, "postgres"),
		Protocol:           "stdout-stderr",
		Mode:               capability.ObservabilityRuntime,
		SemanticConvention: "baseharbor.runtime.logs",
		Verification:       capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := application.ReconcileManagedRuntimeObservability(m, project); err != nil {
			t.Fatalf("managed logs converge round %d: %v", i+1, err)
		}
		if err := logs.ConvergeManagedLogsForReconcile(m, project); err != nil {
			t.Fatalf("collector-side reconcile round %d: %v", i+1, err)
		}
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	// The workload signal exists exactly once after repeated writes.
	sources, err := logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].ID != applicationLogID {
		t.Fatalf("workload log sources after repeated reconciles = %#v, want exactly %q", sources, applicationLogID)
	}
	// The reference class survived every application-class write.
	found, err := observability.RegistryHasSignal("loki:"+project, capability.ProviderLoki, observability.SignalLogs)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		providerSources, listErr := observability.ListLogs(placement, []string{m.Name}, true, true)
		t.Fatalf("application-class reconcile lost the reference-class source; listings after the round: %#v (err %v)", providerSources, listErr)
	}
}

func TestApplicationClassLogSourceHasProductionWriter(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassManifest(t)

	if err := logs.ReconcileApplicationLogSource(m, project, []string{"api"}); err != nil {
		t.Fatalf("reconcile application log source: %v", err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}
	// The reference-class matrix lives in TestApplicationClassLogSourceSurvivesManagedReconcile:
	// a platform-provider log source registered for the SAME application and
	// environment must survive every application-class write unchanged. Here
	// the listing boundary itself must see exactly one source - the class
	// application one - and nothing else.
	if err := observability.UpdateSignal(observability.SignalSource{
		ID:                 "loki:" + project,
		Kind:               observability.SignalLogs,
		Provider:           capability.ProviderLoki,
		Class:              observability.SourceApplicationProvider,
		Scope:              capability.ScopeApplication,
		OwnerApplication:   m.Name,
		Target:             observability.RuntimeTarget(project, "postgres"),
		Protocol:           "stdout-stderr",
		Mode:               capability.ObservabilityRuntime,
		SemanticConvention: "baseharbor.runtime.logs",
		Verification:       capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}
	sources, err := logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("application-class log sources = %d (%#v), want the workload source", len(sources), sources)
	}
	source := sources[0]
	if source.Class != observability.SourceApplication {
		t.Fatalf("listed log source %q has class %q, want %q", source.ID, source.Class, observability.SourceApplication)
	}
	if source.Kind != observability.SignalLogs || source.Provider != capability.ProviderLoki {
		t.Fatalf("listed log source %q has kind/provider %q/%q", source.ID, source.Kind, source.Provider)
	}
	if source.Scope != capability.ScopeApplication || source.OwnerApplication != m.Name {
		t.Fatalf("listed log source %q has scope/owner %q/%q", source.ID, source.Scope, source.OwnerApplication)
	}
	if source.Protocol != "stdout-stderr" {
		t.Fatalf("listed log source %q has protocol %q, want stdout-stderr", source.ID, source.Protocol)
	}
	if source.Verification != capability.ObservabilityVerifyBackend {
		t.Fatalf("listed log source %q has verification %q, want backend-query", source.ID, source.Verification)
	}
	gotProject, service, ok := observability.ParseRuntimeTarget(source.Target)
	if !ok || gotProject != project || service != "api" {
		t.Fatalf("listed log source %q has target %q, want runtime://%s/api", source.ID, source.Target, project)
	}

	// Reconciliation is idempotent: the same intent must not duplicate entries.
	if err := logs.ReconcileApplicationLogSource(m, project, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	sources, err = logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("after re-reconcile application-class log sources = %d, want 1 (idempotent)", len(sources))
	}
}

func TestApplicationClassLogSourceDisabledPolicyReclaims(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassManifest(t)
	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}

	if err := logs.ReconcileApplicationLogSource(m, project, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	// The ID is the workload identity: a second reconcile that resolves a
	// different service retargets the SAME signal instead of accumulating a
	// second entry.
	if err := logs.ReconcileApplicationLogSource(m, project, []string{"worker"}); err != nil {
		t.Fatal(err)
	}
	sources, err := logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("application-class log signals = %#v, want exactly one after a re-target", sources)
	}
	// The prune boundary equals the write boundary: a nil keep set on the
	// application/environment identity empties exactly this deployment's
	// application-class space.
	if err := observability.PruneApplicationLogs(m.Name, m.Environment, nil); err != nil {
		t.Fatal(err)
	}
	sources, err = logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("the prune left the workload log signal behind: %#v", sources)
	}
}

func TestApplicationClassLogSourcePolicyDisableReclaims(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m, project := appClassManifest(t)
	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}

	if err := logs.ReconcileApplicationLogSource(m, project, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.LogsEnabledEnv, "false")
	if err := logs.ReconcileApplicationLogSource(m, project, []string{"api"}); err != nil {
		t.Fatalf("disabled policy reconcile: %v", err)
	}
	sources, err := logs.ListWorkloadLogSources(placement, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("disabled logs policy left application-class sources behind: %#v", sources)
	}
}

// TestReconcileMatchingFailsClosedOnForeignProviderSource pins the safety of
// the new registry primitive itself: a caller whose predicate matches by ID
// cannot use it to delete a same-ID signal of another provider.
func TestReconcileMatchingFailsClosedOnForeignProviderSource(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	if err := observability.UpdateSignal(observability.SignalSource{
		ID:           "loki:bh-local-shared",
		Kind:         observability.SignalLogs,
		Provider:     capability.ProviderLoki,
		Class:        observability.SourcePlatformProvider,
		Scope:        capability.ScopeShared,
		Target:       observability.RuntimeTarget("bh-local-shared", "loki"),
		Protocol:     "stdout-stderr",
		Mode:         capability.ObservabilityRuntime,
		Verification: capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}
	// A desired source outside the predicate (different provider under the
	// same ID) must be rejected without touching the registry.
	err := observability.ReconcileMatching(func(existing observability.SignalSource) bool {
		return existing.ID == "loki:bh-local-shared" && existing.Provider == capability.ProviderLoki
	}, []observability.SignalSource{{
		ID:           "loki:bh-local-shared",
		Kind:         observability.SignalLogs,
		Provider:     capability.ProviderTempo,
		Class:        observability.SourcePlatformProvider,
		Scope:        capability.ScopeShared,
		Target:       observability.RuntimeTarget("bh-local-shared", "loki"),
		Protocol:     "stdout-stderr",
		Mode:         capability.ObservabilityRuntime,
		Verification: capability.ObservabilityVerifyBackend,
	}})
	if err == nil {
		t.Fatal("ReconcileMatching accepted a desired source outside its own predicate")
	}
	found, err := observability.RegistryHasSignal("loki:bh-local-shared", capability.ProviderLoki, observability.SignalLogs)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("a rejected reconciliation must leave the registry untouched")
	}
}

// TestApplicationClassLogSourceReferenceClassCounterCheck is the reference
// class (platform-provider) probe of the matrix: a platform-provider log
// signal must appear in provider listings and must not be removable through
// the application-class boundary.
func TestApplicationClassLogSourceReferenceClassCounterCheck(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	if err := observability.UpdateSignal(observability.SignalSource{
		ID:                 "loki:bh-local-shared",
		Kind:               observability.SignalLogs,
		Provider:           capability.ProviderLoki,
		Class:              observability.SourcePlatformProvider,
		Scope:              capability.ScopeShared,
		Target:             observability.RuntimeTarget("bh-local-shared", "loki"),
		Protocol:           "stdout-stderr",
		Mode:               capability.ObservabilityRuntime,
		SemanticConvention: "baseharbor.runtime.logs",
		Verification:       capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	shared := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}
	sources, err := observability.ListLogs(shared, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Class != observability.SourcePlatformProvider {
		t.Fatalf("platform-provider reference listing = %#v, want exactly the provider source", sources)
	}

	m, project := appClassManifest(t)
	if err := logs.ReconcileApplicationLogSource(m, project, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	if err := logs.ReconcileApplicationLogSource(m, project, nil); err != nil {
		t.Fatal(err)
	}
	sources, err = observability.ListLogs(shared, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Class != observability.SourcePlatformProvider {
		t.Fatalf("the application-class reclaim disturbed the platform-provider reference source: %#v", sources)
	}
}
