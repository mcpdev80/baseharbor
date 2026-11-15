package capability

import "testing"

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

// TestDashboardEligibleWhenSelectedAndRealSignal present is the positive
// case: Perses selected, the pinned release valid, and the real backend
// signal present -> the corresponding dashboard is eligible.
func TestDashboardEligibleWhenSelectedAndRealSignalPresent(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	signals := map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
		DashboardBackendLoki:       true,
		DashboardBackendTempo:      true,
	}
	elig, err := catalog.EligibleFor("prometheus-targets", true, signals)
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

// TestDashboardNotEligibleWithoutPersesSelection fails closed: a selected
// backend signal alone is not enough; the Perses surface must be selected.
func TestDashboardNotEligibleWithoutPersesSelection(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	signals := map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
	}
	elig, err := catalog.EligibleFor("prometheus-targets", false, signals)
	if err != nil {
		t.Fatalf("eligibility must not error when unselected: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("dashboard must not be eligible when Perses is unselected")
	}
	if elig.Status() != DashboardNotEligible {
		t.Fatalf("status = %q, want %q", elig.Status(), DashboardNotEligible)
	}
}

// TestDashboardFailClosedWithoutPinnedRelease fails closed when the pinned
// release is invalid/empty, even with a selected surface and a signal.
func TestDashboardFailClosedWithoutPinnedRelease(t *testing.T) {
	signals := map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
	}
	// A release whose validation fails maps the catalog to fail closed.
	failCatalog := PersesDashboardCatalog{
		release: "v0.1.0",
		templates: map[string]DashboardTemplate{
			"prometheus-targets": {
				Key:               "prometheus-targets",
				Backend:           DashboardBackendPrometheus,
				Datasource:        "prometheus",
				SupportedVersions: []string{"v0.55.1"},
			},
		},
	}
	elig, err := failCatalog.EligibleFor("prometheus-targets", true, signals)
	if err != nil {
		t.Fatalf("eligibility must not error on an unsupported version: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("dashboard must not be eligible for an unsupported perses release")
	}
	if elig.Reason == "" {
		t.Fatal("an ineligible verdict must carry a concrete reason")
	}
}

// TestDashboardNotEligibleWithoutRealSignal fails closed when no real
// backend signal exists for the dashboard's backend (missing metric/label
// coverage): selection + pinned release alone must not provision.
func TestDashboardNotEligibleWithoutRealSignal(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	// Prometheus selected + pinned, but Loki has no real signal.
	signals := map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
		DashboardBackendLoki:       false,
	}
	elig, err := catalog.EligibleFor("loki-logs", true, signals)
	if err != nil {
		t.Fatalf("eligibility must not error on a missing signal: %v", err)
	}
	if elig.Eligible {
		t.Fatalf("loki-logs must not be eligible without a real Loki signal")
	}
	if !containsString(elig.Reason, "loki") {
		t.Fatalf("reason must name the missing backend, got %q", elig.Reason)
	}
}

// TestDashboardNotEligibleForUnknownKey rejects a key that is not part of
// the shipped catalog (stale/unknown dashboard reference).
func TestDashboardNotEligibleForUnknownKey(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	signals := map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
	}
	if _, err := catalog.EligibleFor("grafana-overview", true, signals); err == nil {
		t.Fatal("an unknown dashboard key must not be eligible")
	}
}

// TestDashboardUnselectedNoSignalYieldsNoEligible enumerates the whole
// catalog for an unselected surface: every dashboard is ineligible.
func TestDashboardUnselectedNoEligible(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	n, err := catalog.EligibleCount(false, map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
		DashboardBackendLoki:       true,
		DashboardBackendTempo:      true,
	})
	if err != nil {
		t.Fatalf("eligibility count must not error: %v", err)
	}
	if n != 0 {
		t.Fatalf("unselected Perses surface must yield 0 eligible dashboards, got %d", n)
	}
}

// TestDashboardAllEligibleWhenAllSignalsPresent verifies the full-catalog
// positive: selection + all real signals -> three eligible dashboards.
func TestDashboardAllEligibleWhenAllSignalsPresent(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	n, err := catalog.EligibleCount(true, map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
		DashboardBackendLoki:       true,
		DashboardBackendTempo:      true,
	})
	if err != nil {
		t.Fatalf("eligibility count must not error: %v", err)
	}
	if n != 3 {
		t.Fatalf("all three real backends must yield 3 eligible dashboards, got %d", n)
	}
}

// TestFilterEligibleDashboardsUnknownOwnerFailClosed guards the
// authorization boundary: an empty signal inventory (no owned backend)
// yields no eligible dashboard even when Perses is selected.
func TestFilterEligibleDashboardsUnknownOwnerFailClosed(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	eligible, err := FilterEligibleDashboards(catalog, true, map[DashboardBackend]bool{})
	if err != nil {
		t.Fatalf("filter must not error on an empty signal inventory: %v", err)
	}
	if len(eligible) != 0 {
		t.Fatalf("an empty signal inventory must yield no eligible dashboard, got %v", eligible)
	}
}

// TestFilterEligibleDashboardsDeterministic orders the eligible keys so a
// consumer can render them stably.
func TestFilterEligibleDashboardsDeterministic(t *testing.T) {
	catalog := DefaultDashboardCatalog()
	eligible, err := FilterEligibleDashboards(catalog, true, map[DashboardBackend]bool{
		DashboardBackendPrometheus: true,
		DashboardBackendLoki:       true,
		DashboardBackendTempo:      true,
	})
	if err != nil {
		t.Fatalf("filter must not error: %v", err)
	}
	if len(eligible) != 3 {
		t.Fatalf("expected 3 eligible dashboards, got %v", eligible)
	}
	for i := 1; i < len(eligible); i++ {
		if eligible[i-1] > eligible[i] {
			t.Fatalf("eligible dashboards are not ordered: %v", eligible)
		}
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
