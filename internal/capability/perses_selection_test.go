package capability

import "testing"

// availableIdentityWitness is a test helper: an available identity-selection
// witness attests a real Keycloak metric signal for an authorized Target with
// the identity provider actually selected and pinned to the current release.
func availableIdentityWitness() IdentitySelectionWitness {
	return NewIdentitySelectionWitness(true, true, true, true)
}

// allBackendSignals is a helper: every real backend has a real signal.
func allBackendSignals() map[DashboardBackend]bool {
	return map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
		DashboardBackendLoki:       true,
		DashboardBackendTempo:      true,
	}
}

// TestDashboardCatalogBuildsWithRealBackends pins the shipped catalog: it
// has a template per real BaseHarbor observability backend (Prometheus,
// Loki, Tempo) and every template carries a backend and a datasource kind.
// Backends without a native signal (PostgreSQL/Valkey requires-adapter)
// are intentionally absent: they must never be provisioned.
func TestDashboardCatalogBuildsWithRealBackends(t *testing.T) {
	if err := VerifyPersesDashboardCatalogBuilds(); err != nil {
		t.Fatalf("shipped catalog must build: %v", err)
	}
	catalog := DefaultDashboardCatalog()
	want := map[DashboardBackend]string{
		DashboardBackendPrometheus: "prometheus-targets",
		DashboardBackendLoki:       "loki-logs",
		DashboardBackendTempo:      "tempo-traces",
	}
	for backend, key := range want {
		if !catalog.HasTemplate(key) {
			t.Fatalf("catalog must have template %q for backend %q", key, backend)
		}
	}
	// A backend with no native signal (PostgreSQL) must not appear.
	for key := range catalog.Templates() {
		if key == "postgresql-targets" {
			t.Fatalf("postgresql has requires-adapter (no native signal); no template may exist")
		}
	}
}

// TestDashboardEligibleWhenRealSignalAndAuthorizedTarget present is the
// positive case: the identity-selection witness attests a real Keycloak metric
// signal for an authorized Target with the provider selected and the pinned
// release current -> the dashboard is eligible.
func TestDashboardEligibleWhenRealSignalAndAuthorizedTarget(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	elig, err := catalog.EligibleFor("prometheus-targets", availableIdentityWitness(), allBackendSignals())
	if err != nil {
		t.Fatalf("eligibility must not error on a real signal: %v", err)
	}
	if !elig.Eligible {
		t.Fatalf("prometheus-targets must be eligible with a real signal: %+v", elig)
	}
	if elig.VersionPinned != AcceptedPersesVersion {
		t.Fatalf("eligibility version = %q, want %q", elig.VersionPinned, AcceptedPersesVersion)
	}
	if elig.Backend != string(DashboardBackendPrometheus) {
		t.Fatalf("eligibility backend = %q, want %q", elig.Backend, DashboardBackendPrometheus)
	}
}

// TestIdentitySelectionDerivedNotSupplied pins the core invariant: a witness
// is built from the four evidence flags, and Available is the AND of them. A
// caller cannot make availability true by asserting a single status boolean.
func TestIdentitySelectionDerivedNotSupplied(t *testing.T) {
	witness := NewIdentitySelectionWitness(true, true, true, true)
	if !witness.Available {
		t.Fatalf("a fully-evidenced witness must derive available")
	}
	// Every single missing flag fails closed.
	for name, w := range map[string]IdentitySelectionWitness{
		"not-selected":    NewIdentitySelectionWitness(false, true, true, true),
		"no-metric":       NewIdentitySelectionWitness(true, false, true, true),
		"metric-not-curr": NewIdentitySelectionWitness(true, true, false, true),
		"unauthorized":    NewIdentitySelectionWitness(true, true, true, false),
		"all-missing":     NewIdentitySelectionWitness(false, false, false, false),
	} {
		if w.Available {
			t.Fatalf("%s: a missing evidence flag must fail closed to unavailable", name)
		}
	}
}

// TestIdentitySelectionDefaultUnpublishedIsUnavailable pins fail-closed: the
// zero-value witness (an unconstructed / not-yet-resolved fan-in) is
// unavailable and yields no eligible dashboard, even when Perses is selected.
func TestIdentitySelectionDefaultUnpublishedIsUnavailable(t *testing.T) {
	var zero IdentitySelectionWitness
	if zero.Available {
		t.Fatalf("the zero-value witness must be unavailable (fail-closed)")
	}
	catalog := DefaultDashboardCatalog()
	elig, err := catalog.EligibleFor("prometheus-targets", zero, allBackendSignals())
	if err != nil {
		t.Fatalf("eligibility must not error on an unconstructed witness: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("dashboard must not be eligible without an available witness: %+v", elig)
	}
	if elig.Reason == "" {
		t.Fatalf("an unavailable verdict must carry a concrete fail-closed reason")
	}
	if containsString(elig.Reason, "caller-supplied status") == false {
		t.Fatalf("the fail-closed reason must name the caller-supplied-status prohibition, got %q", elig.Reason)
	}
}

// TestIdentitySelectionNeverForgeableDirectAvailable is the non-forgeability
// regression: a caller cannot obtain a positive verdict by constructing the
// struct literal with only Available: true and no evidence. The production
// gate uses NonForgeableAvailable(), which requires the four real-evidence
// flags, so the forged literal fails closed.
func TestIdentitySelectionNeverForgeableDirectAvailable(t *testing.T) {
	// The exact Luna request-changes case: a caller sets only the exported
	// Available result, supplying no evidence at all.
	forged := IdentitySelectionWitness{Available: true}
	if forged.NonForgeableAvailable() {
		t.Fatalf("a forged literal with only Available: true must be non-forgeable")
	}
	catalog := DefaultDashboardCatalog()
	elig, err := catalog.EligibleFor("prometheus-targets", forged, allBackendSignals())
	if err != nil {
		t.Fatalf("eligibility must not error on a forged witness: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("a forged Available:true literal must not make a dashboard available: %+v", elig)
	}
	if elig.Reason == "" {
		t.Fatalf("the forged witness must fail closed with a concrete reason")
	}
}

// TestIdentitySelectionUnavailableWhenNoKeycloakMetricSignal fails closed when
// no real Keycloak metric signal is present for this Target.
func TestIdentitySelectionUnavailableWhenNoKeycloakMetricSignal(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	elig, err := catalog.EligibleFor("prometheus-targets", NewIdentitySelectionWitness(true, false, true, true), allBackendSignals())
	if err != nil {
		t.Fatalf("eligibility must not error on a missing signal: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("dashboard must not be eligible without a real Keycloak metric signal")
	}
	if !containsString(elig.Reason, "Keycloak") {
		t.Fatalf("reason must name the missing Keycloak signal, got %q", elig.Reason)
	}
}

// TestIdentitySelectionUnavailableWhenMetricNotCurrentVersion fails closed when
// the metric is not pinned to the current accepted release.
func TestIdentitySelectionUnavailableWhenMetricNotCurrentVersion(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	elig, err := catalog.EligibleFor("prometheus-targets", NewIdentitySelectionWitness(true, true, false, true), allBackendSignals())
	if err != nil {
		t.Fatalf("eligibility must not error on an obsolete metric: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("dashboard must not be eligible when the metric is not the current version")
	}
}

// TestIdentitySelectionUnavailableWhenTargetNotAuthorized fails closed when the
// provider is not authorized for this Target, even with a real signal present.
func TestIdentitySelectionUnavailableWhenTargetNotAuthorized(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	elig, err := catalog.EligibleFor("prometheus-targets", NewIdentitySelectionWitness(true, true, true, false), allBackendSignals())
	if err != nil {
		t.Fatalf("eligibility must not error on an unauthorized target: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("dashboard must not be eligible when the target is not authorized")
	}
}

// TestEligibleForPerBackendGate proves the per-backend gating (no
// cross-dashboard aggregate): the identity witness being available enables a
// backend only when that backend itself has a real signal, and an absent
// witness does not suppress a backend that has its own real signal unless
// identity is a required prerequisite (it is: NonForgeableAvailable gates
// first). Each backend is evaluated on its own backendSignals[backend] entry.
func TestEligibleForPerBackendGate(t *testing.T) {
	catalog := DefaultDashboardCatalog()

	// A single backend signal present, all others absent: only that
	// backend's dashboard is eligible, never the others.
	witness := availableIdentityWitness()
	for key, have := range map[string]DashboardBackend{
		"prometheus-targets": DashboardBackendPrometheus,
		"loki-logs":          DashboardBackendLoki,
		"tempo-traces":       DashboardBackendTempo,
	} {
		signals := map[DashboardBackend]bool{have: true}
		elig, err := catalog.EligibleFor(key, witness, signals)
		if err != nil {
			t.Fatalf("%s: eligibility must not error: %v", key, err)
		}
		if !elig.Eligible {
			t.Fatalf("%s must be eligible when its own backend has a real signal: %+v", key, elig)
		}
	}

	// The same backend, requested against a different key, still fails closed
	// because that key's backend has no signal — not because of a shared
	// aggregate.
	signals := map[DashboardBackend]bool{DashboardBackendPrometheus: true}
	loki, err := catalog.EligibleFor("loki-logs", witness, signals)
	if err != nil {
		t.Fatalf("loki-eligibility must not error: %v", err)
	}
	if loki.Eligible {
		t.Fatalf("loki-logs must not be eligible when only prometheus has a signal (per-backend, not aggregate)")
	}
	if !containsString(loki.Reason, "loki") {
		t.Fatalf("the reason must name the missing loki backend, got %q", loki.Reason)
	}
}

// TestEligibleForZeroValueWitnessFailsClosedPerBackend proves that the
// zero-value witness (no evidence) fails closed for every backend independently
// of any backend signal that may be present.
func TestEligibleForZeroValueWitnessFailsClosedPerBackend(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	elig, err := catalog.EligibleFor("prometheus-targets", IdentitySelectionWitness{}, allBackendSignals())
	if err != nil {
		t.Fatalf("eligibility must not error on a zero-value witness: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("a zero-value witness must fail closed even with all backend signals present")
	}
}

// TestFilterEligibleDashboardsUnknownOwnerFailClosed guards the
// authorization boundary: an empty/zero witness (no owned backend signal)
// yields no eligible dashboard even when Perses is selected.
func TestFilterEligibleDashboardsUnknownOwnerFailClosed(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	eligible, err := FilterEligibleDashboards(catalog, IdentitySelectionWitness{}, map[DashboardBackend]bool{})
	if err != nil {
		t.Fatalf("filter must not error on an unconstructed witness: %v", err)
	}
	if len(eligible) != 0 {
		t.Fatalf("an unconstructed witness must yield no eligible dashboard, got %v", eligible)
	}
}

// TestFilterEligibleDashboardsAvailableYieldsExpected pins the positive filter:
// an available witness and all backend signals yield the expected eligible
// dashboards.
func TestFilterEligibleDashboardsAvailableYieldsExpected(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	eligible, err := FilterEligibleDashboards(catalog, availableIdentityWitness(), allBackendSignals())
	if err != nil {
		t.Fatalf("filter must not error: %v", err)
	}
	if len(eligible) != 3 {
		t.Fatalf("expected 3 eligible dashboards with an available witness, got %v", eligible)
	}
	for i := 1; i < len(eligible); i++ {
		if eligible[i-1] > eligible[i] {
			t.Fatalf("eligible dashboards are not ordered: %v", eligible)
		}
	}
}

// TestFilterEligibleDashboardsPerBackendOnly pins the per-backend filter:
// only the dashboards whose backend actually has a signal are eligible; an
// available identity witness alone does not enable every backend.
func TestFilterEligibleDashboardsPerBackendOnly(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	eligible, err := FilterEligibleDashboards(catalog, availableIdentityWitness(), map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
	})
	if err != nil {
		t.Fatalf("filter must not error: %v", err)
	}
	if len(eligible) != 1 {
		t.Fatalf("only prometheus-targets must be eligible when only prometheus has a signal, got %v", eligible)
	}
	if eligible[0] != "prometheus-targets" {
		t.Fatalf("the single eligible dashboard must be prometheus-targets, got %q", eligible[0])
	}
}

// TestDashboardCatalogVersionMatchesSeamPin keeps the catalog release key
// in lock-step with the seam pin so a drift cannot provision a dashboard
// against an unpinned release.
func TestDashboardCatalogVersionMatchesSeamPin(t *testing.T) {
	if AcceptedPersesVersion != PersesDefaultRelease.Version {
		t.Fatalf("catalog version %q must match the seam pin %q", AcceptedPersesVersion, PersesDefaultRelease.Version)
	}
	if err := ValidatePersesRelease(PersesDefaultRelease); err != nil {
		t.Fatalf("the pinned release must validate: %v", err)
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
