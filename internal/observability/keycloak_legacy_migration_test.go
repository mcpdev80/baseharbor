package observability

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// legacyKeycloakSource names the pre-scoping "keycloak-metrics" registration the
// runtime wrote before 0428-1707 made the ID scoping-aware.
var legacyKeycloakSource = LegacySource{
	ID:        "keycloak-metrics",
	Providers: []capability.ProviderKind{capability.ProviderKeycloak},
}

// TestLegacyKeycloakMetricsSourceIsCorpseProvesTheBug pins the leiche exactly:
// a pre-existing legacy "keycloak-metrics" source of record is silently dropped
// by the new scoping-ID registration, so it is gone from List and unremovable by
// its own ID. This test PASSES against the buggy code (witnesses the corpse) and
// FAILS once the migration re-books it. The finding depends on it.
func TestLegacyKeycloakMetricsSourceIsCorpseProvesTheBug(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	const legacyID = "keycloak-metrics"

	// 1. The pre-deploy legacy source, written exactly as the OLD code did it:
	// an app-scoped Keycloak metrics source under the literal id.
	if err := Update(MetricsSource{
		ID: legacyID, Provider: capability.ProviderKeycloak, Class: SourceApplicationProvider,
		Scope: capability.ScopeApplication, OwnerApplication: "app-a",
		Network: "baseharbor-app-a-dev", Target: "keycloak:9000", Path: "/metrics", Scheme: "http",
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}
	if ok, err := RegistryHasSignal(legacyID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatalf("legacy source not present immediately after write")
	}

	// 2. The NEW code runs its first scoping-ID registration (a sibling
	// Keycloak project, different owner application). Without the migration the
	// legacy source of record is dropped.
	const newID = "keycloak:project-b"
	if err := RegisterProviderSignals(ProviderSignalRegistration{
		ID:               newID,
		Descriptor:       capability.KeycloakIntegration,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "app-b",
		Enabled:          map[SignalKind]bool{SignalMetrics: true},
		Signals:          map[string]ProviderSignalRuntime{"keycloak-metrics": {Network: "baseharbor-app-b-dev", Target: "keycloak:9000"}},
	}); err != nil {
		t.Fatalf("new scoping-ID registration: %v", err)
	}

	// 3. The corpse: the legacy source is gone from List for its own caller.
	list, err := ListMetrics(capability.ProviderPlacement{Scope: capability.ScopeApplication}, []string{"app-a"}, true, true, "dev")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, s := range list {
		if s.ID == legacyID {
			t.Fatalf("legacy %q still present after new registration -> corpse NOT reproduced: %#v", legacyID, list)
		}
	}

	// 4. Because it is gone, it is unremovable by its own ID.
	_ = Remove(legacyID)
	if ok, err := RegistryHasSignal(legacyID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("legacy source unexpectedly still present after its own Remove")
	}
}

// TestReconcileLegacySignalsRebooksTheCorpse proves the FIX: the first new
// scoping-ID registration re-reads the legacy source of record and re-books it
// under the new ID, so it stays visible and removable by its new ID and the
// legacy ID is gone from List.
func TestReconcileLegacySignalsRebooksTheCorpse(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	const legacyID = "keycloak-metrics"
	const newID = "keycloak:project-b"

	if err := Update(MetricsSource{
		ID: legacyID, Provider: capability.ProviderKeycloak, Class: SourceApplicationProvider,
		Scope: capability.ScopeApplication, OwnerApplication: "app-a",
		Network: "baseharbor-app-a-dev", Target: "keycloak:9000", Path: "/metrics", Scheme: "http",
	}); err != nil {
		t.Fatalf("write legacy source: %v", err)
	}

	// The NEW registration carries the reconciliation seam: it re-books the
	// legacy Keycloak source onto the scoping ID instead of dropping it.
	if err := ReconcileLegacySignals(legacyKeycloakSource, newID,
		map[capability.ProviderKind]struct{}{capability.ProviderKeycloak: {}}, nil); err != nil {
		t.Fatalf("ReconcileLegacySignals: %v", err)
	}
	if err := RegisterProviderSignals(ProviderSignalRegistration{
		ID:               newID,
		Descriptor:       capability.KeycloakIntegration,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "app-b",
		Enabled:          map[SignalKind]bool{SignalMetrics: true},
		Signals:          map[string]ProviderSignalRuntime{"keycloak-metrics": {Network: "baseharbor-app-b-dev", Target: "keycloak:9000"}},
	}); err != nil {
		t.Fatalf("new scoping-ID registration: %v", err)
	}

	// The legacy ID is gone from List (re-booked onto the new ID).
	if ok, err := RegistryHasSignal(legacyID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("legacy %q still present after reconciliation -> not re-booked", legacyID)
	}

	// The source is now visible and removable by its new scoping ID.
	if ok, err := RegistryHasSignal(newID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatalf("re-booked source %q not present", newID)
	}
	if err := Remove(newID); err != nil {
		t.Fatalf("Remove(%q): %v", newID, err)
	}
	if ok, err := RegistryHasSignal(newID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("re-booked source still present after its own Remove")
	}
}

// TestReconcileLegacySignalsIdempotentAndNoMutation proves the two migration
// regression guards: (a) a second reconciliation on an already-migrated
// registry changes nothing, and (b) a legacy source already carried under the
// new scoping ID is never rewritten.
func TestReconcileLegacySignalsIdempotentAndNoMutation(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	const legacyID = "keycloak-metrics"
	const newID = "keycloak:project-b"

	// Case (a): no legacy source at all, reconcile is a no-op that leaves the
	// registry empty (nothing to re-book, nothing written).
	if err := ReconcileLegacySignals(legacyKeycloakSource, newID,
		map[capability.ProviderKind]struct{}{capability.ProviderKeycloak: {}}, nil); err != nil {
		t.Fatalf("reconcile on empty registry: %v", err)
	}
	if ok, err := RegistryHasSignal(newID, capability.ProviderKeycloak, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("reconcile on empty registry wrote a source it did not own")
	}

	// Case (b): a source already carried under the new scoping ID is not
	// rewritten by a reconciliation.
	if err := Update(MetricsSource{
		ID: newID, Provider: capability.ProviderKeycloak, Class: SourceApplicationProvider,
		Scope: capability.ScopeApplication, OwnerApplication: "app-b",
		Network: "baseharbor-app-b-dev", Target: "keycloak:9000", Path: "/metrics", Scheme: "http",
	}); err != nil {
		t.Fatalf("write new-ID source: %v", err)
	}
	before, err := readSignalsForTest(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReconcileLegacySignals(legacyKeycloakSource, newID,
		map[capability.ProviderKind]struct{}{capability.ProviderKeycloak: {}}, nil); err != nil {
		t.Fatalf("reconcile on already-migrated registry: %v", err)
	}
	after, err := readSignalsForTest(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("reconcile changed the registry size %d -> %d (not idempotent)", len(before), len(after))
	}
	for i := range before {
		if before[i].ID != after[i].ID || before[i].OwnerApplication != after[i].OwnerApplication {
			t.Fatalf("reconcile rewrote an already-migrated source: before %#v after %#v", before, after)
		}
	}
}

// TestReconcileLegacySignalsForeignProviderUntouched proves a same-ID legacy
// registration under a provider the new identity does not own is never
// rewritten, so cross-provider identity spaces stay disjoint (boundary to the
// two-project separation owned by card 0428-166).
func TestReconcileLegacySignalsForeignProviderUntouched(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	const legacyID = "keycloak-metrics"

	// A legacy "keycloak-metrics" under a DIFFERENT provider (Prometheus) must
	// survive a reconciliation that only owns Keycloak.
	if err := Update(MetricsSource{
		ID: legacyID, Provider: capability.ProviderPrometheus, Class: SourceApplicationProvider,
		Scope: capability.ScopeApplication, OwnerApplication: "app-z",
		Network: "baseharbor-app-z-dev", Target: "prometheus:9090", Path: "/metrics", Scheme: "http",
	}); err != nil {
		t.Fatalf("write foreign-provider source: %v", err)
	}
	if err := ReconcileLegacySignals(legacyKeycloakSource, "keycloak:project-b",
		map[capability.ProviderKind]struct{}{capability.ProviderKeycloak: {}}, nil); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if ok, err := RegistryHasSignal(legacyID, capability.ProviderPrometheus, SignalMetrics); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatalf("foreign-provider legacy source was rewritten by a Keycloak-only reconciliation")
	}
}
