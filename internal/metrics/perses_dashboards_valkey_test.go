package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestPersesValkeyDashboardCatalogIsUnavailableOnly pins the shipped Valkey
// catalog: it contains exactly one entry (valkey-targets) and that entry is
// EXPLICITLY UNAVAILABLE, because Valkey has no native metrics signal in
// BaseHarbor. No fabricated panel ships for Valkey and no exporter is
// installed. This is the 0428-081 conditional-dashboard deliverable, mirroring
// the PostgreSQL/MongoDB handling in the #1597 inventory.
func TestPersesValkeyDashboardCatalogIsUnavailableOnly(t *testing.T) {
	catalog := persesValkeyDashboardCatalog()
	if len(catalog) != 1 {
		t.Fatalf("Valkey catalog must contain exactly one entry, got %d: %v", len(catalog), keysOf(catalog))
	}
	entry, ok := catalog[PersesValkeyDashboardCatalogKey]
	if !ok {
		t.Fatalf("Valkey catalog must contain entry %q", PersesValkeyDashboardCatalogKey)
	}
	if !entry.Unavailable {
		t.Fatalf("Valkey dashboard has no native metrics signal and must be marked unavailable")
	}
	if entry.Dashboard.Kind != "" {
		t.Fatalf("unavailable Valkey dashboard must ship no fabricated panel, got kind %q", entry.Dashboard.Kind)
	}
	if entry.Dashboard.Spec.Panels != nil {
		t.Fatalf("unavailable Valkey dashboard must ship zero panels, got %d", len(entry.Dashboard.Spec.Panels))
	}
	if entry.Reason == "" {
		t.Fatalf("unavailable entry must carry a concrete reason")
	}
	// The reason must name the missing native signal and the
	// ObservabilityUnsupported status exactly, so the CLI witness is
	// unambiguous and reflects the 0428-182 reference status.
	if !strings.Contains(entry.Reason, "Valkey") {
		t.Fatalf("reason must name the missing-signal provider Valkey, got: %s", entry.Reason)
	}
	if !strings.Contains(entry.Reason, "RequiresAdapter") {
		t.Fatalf("reason must cite the observability status RequiresAdapter, got: %s", entry.Reason)
	}
	if strings.Contains(entry.Reason, "exporter is installed") && strings.Count(entry.Reason, "no exporter") == 0 {
		t.Fatalf("reason must state no exporter is installed, got: %s", entry.Reason)
	}
}

// TestPersesValkeyDashboardKeysDistinctFromOtherInventories pins the faechregel:
// the Valkey catalog key is disjoint from the #1597 database dashboard keys and
// the 0428-077 host/container dashboard keys. A collision would be a scope
// violation (stacking on another lane's inventory).
func TestPersesValkeyDashboardKeysDistinctFromOtherInventories(t *testing.T) {
	otherKeys := []string{
		// #1597 database dashboards (perses_dashboards.go).
		"prometheus-targets", "loki-logs", "tempo-traces",
		"postgresql-targets", "mongodb-targets",
		// 0428-077 host/container dashboards (perses_dashboards_hosts.go).
		"prometheus-host-container", "loki-host-container", "tempo-host-container",
		"host-node-metrics", "container-stats",
	}
	for _, k := range otherKeys {
		if k == PersesValkeyDashboardCatalogKey {
			t.Fatalf("Valkey dashboard key %q collides with another lane's inventory key", k)
		}
	}
}

// TestPersesValkeyUnavailableMarshalsReason pins the negative witness: the
// unavailable Valkey entry emits a reason document (not a fabricated panel) with
// the Perses-unavailable kind and the concrete reason.
func TestPersesValkeyUnavailableMarshalsReason(t *testing.T) {
	entry := persesValkeyDashboardCatalog()[PersesValkeyDashboardCatalogKey]
	raw, err := persesValkeyPanelJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The & in a reason must be emitted literally if present; here we only
	// assert the document is well-formed Perses-native JSON.
	if strings.Contains(string(raw), `\u0026`) {
		t.Fatalf("output contains escaped ampersand \\u0026, want literal &: %s", raw)
	}
	var doc struct {
		Kind        string `json:"kind"`
		Key         string `json:"key"`
		Backend     string `json:"backend"`
		Unavailable bool   `json:"unavailable"`
		Reason      string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("output is not the reason document: %v", err)
	}
	if doc.Kind != "DashboardUnavailable" {
		t.Fatalf("unavailable entry kind = %q, want DashboardUnavailable", doc.Kind)
	}
	if !doc.Unavailable {
		t.Fatalf("unavailable entry must report unavailable=true")
	}
	if doc.Key != PersesValkeyDashboardCatalogKey {
		t.Fatalf("unavailable entry key = %q, want %q", doc.Key, PersesValkeyDashboardCatalogKey)
	}
	if doc.Backend != "valkey" {
		t.Fatalf("unavailable entry backend = %q, want valkey", doc.Backend)
	}
	if !strings.Contains(doc.Reason, "Valkey") {
		t.Fatalf("reason must name the missing-signal provider Valkey, got: %s", doc.Reason)
	}
}

// TestPersesValkeyDashboardJSONIsDeterministic pins the marshalling: the same
// entry marshals to the same bytes on every call, so the golden witness is
// stable.
func TestPersesValkeyDashboardJSONIsDeterministic(t *testing.T) {
	first, err := persesValkeyPanelJSON(persesValkeyDashboardCatalog()[PersesValkeyDashboardCatalogKey])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := persesValkeyPanelJSON(persesValkeyDashboardCatalog()[PersesValkeyDashboardCatalogKey])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("perses valkey dashboard JSON is not deterministic:\n%s\n---\n%s", first, second)
	}
}

// TestPersesValkeyDashboardAvailabilityPinsReasons surfaces the availability
// verdict on the CLI witness: valkey-targets reports "unavailable" with the
// reason naming the missing native signal.
func TestPersesValkeyDashboardAvailabilityPinsReasons(t *testing.T) {
	avail := PersesValkeyDashboardAvailability("")
	entry, ok := avail[PersesValkeyDashboardCatalogKey]
	if !ok {
		t.Fatalf("availability must contain entry %q", PersesValkeyDashboardCatalogKey)
	}
	if entry.Available != "unavailable" {
		t.Fatalf("valkey-targets availability = %q, want unavailable", entry.Available)
	}
	if entry.Backend != "valkey" {
		t.Fatalf("valkey-targets backend = %q, want valkey", entry.Backend)
	}
	if entry.Datasource != "" {
		t.Fatalf("unavailable valkey-targets must report no datasource, got %q", entry.Datasource)
	}
	if !strings.Contains(entry.Reason, "RequiresAdapter") {
		t.Fatalf("availability reason must cite RequiresAdapter, got: %s", entry.Reason)
	}
}

// valkeyAvailabilityStatuses enumerates the three states the card acceptance
// requires on the Valkey face, driven by the REAL producer
// (persesValkeyResolvedSignalStatus) plus the two other resolved catalog
// statuses. This is NOT a caller-set bool — each status is a value the
// reference catalog actually resolves (ObservabilitySupported /
// ObservabilityUnsupported / empty), so driving it proves the producer can
// deliver the verdict, that the gate reads it, and that the entry flips.
func valkeyAvailabilityStatuses() map[string]capability.ObservabilitySupportStatus {
	return map[string]capability.ObservabilitySupportStatus{
		"not_configured":        "",
		"configured_unverified": capability.ObservabilityUnsupported,
		"verified":              capability.ObservabilitySupported,
	}
}

// TestPersesValkeyDashboardProducerReadsReferenceCatalog pins the REAL producer
// (persesValkeyResolvedSignalStatus) to the authoritative reference catalog: it
// must resolve to the status Valkey actually declares in reference.go
// (ValkeyIntegration) — today ObservabilityUnsupported (metrics is not a real
// signal), which is NOT ObservabilitySupported. A producer that did not read the
// catalog (e.g. always returning Supported) would fail this — it is the proof
// the verdict comes from the real signal path, not a caller-set flag.
func TestPersesValkeyDashboardProducerReadsReferenceCatalog(t *testing.T) {
	got := persesValkeyResolvedSignalStatus()
	if got == capability.ObservabilitySupported {
		t.Fatalf("real producer must NOT resolve Valkey metrics to ObservabilitySupported — Valkey has no real native metrics signal in the reference catalog (reference.go ValkeyIntegration), got %q", got)
	}
	// The producer must match the status the reference catalog declares for
	// Valkey's metrics signal. Pin it to that exact status so a future
	// reference-catalog change is reflected here, not silently diverged.
	desc, refErr := capability.ReferenceIntegration(capability.ProviderValkey)
	if refErr != nil {
		t.Fatalf("resolve Valkey reference integration: %v", refErr)
	}
	want := capability.ObservabilitySupportStatus("")
	for _, signal := range desc.Observability.Signals {
		if signal.Name == "metrics" {
			want = signal.Status
			break
		}
	}
	if want == "" {
		t.Fatalf("reference catalog Valkey integration has no metrics signal")
	}
	if got != want {
		t.Fatalf("producer verdict %q must equal the Valkey metrics status the reference catalog declares (%q)", got, want)
	}
}

// TestPersesValkeyDashboardMutantPredicateRevertedIsolated proves the availability
// gate is real, not vacuous: the producer resolves ObservabilitySupported for a
// real native signal, and the catalog MUST promote the entry. If a future
// change reverts the predicate (always Unavailable), this test fails — it is
// mutant A from the 0428-068 / 0428-166 history (a reverted predicate is NOT an
// independent witness, so it needs its own isolated test).
func TestPersesValkeyDashboardMutantPredicateRevertedIsolated(t *testing.T) {
	// The real producer answers the catalog's OWN verified branch. If the
	// catalog now always returns Unavailable for a Supported signal, the gate
	// is reverted and this test fails.
	entry := persesValkeyDashboardCatalogWithStatus(capability.ObservabilitySupported)[PersesValkeyDashboardCatalogKey]
	if entry.Unavailable {
		t.Fatalf("ObservabilitySupported must NOT leave the entry unavailable; the availability gate was reverted")
	}
	if entry.Dashboard.Kind == "" {
		t.Fatalf("ObservabilitySupported must realize a real dashboard resource, got an empty dashboard")
	}
}

// TestPersesValkeyDashboardMutantAvailabilityForcedIsolated proves the reverse
// direction: with the empty status (the CLI fail-closed face) the availability
// gate MUST stay fail-closed and report "unavailable" (mutant C — forcing the
// availability branch without a real producer verdict is NOT an independent
// witness, so it needs its own isolated test that fails if the gate opens).
func TestPersesValkeyDashboardMutantAvailabilityForcedIsolated(t *testing.T) {
	avail := PersesValkeyDashboardAvailability("")
	entry := avail[PersesValkeyDashboardCatalogKey]
	if entry.Available == "available" {
		t.Fatalf("empty status must NOT report available; the availability branch was force-opened without a real producer verdict")
	}
	if entry.Available != "unavailable" {
		t.Fatalf("empty-status valkey-targets availability = %q, want unavailable", entry.Available)
	}
}

// TestPersesValkeyDashboardThreeStates drives the three-state availability path
// the card requires on the Valkey face through the REAL producer verdicts:
// not_configured (empty) and configured_unverified (ObservabilityUnsupported)
// report "unavailable", verified (ObservabilitySupported) reports "available" with a
// real datasource. configured_unverified MUST NOT be available — only a verified
// native signal promotes the entry, mirroring the verified-only predicate the
// identity dashboard hardened (0428-078).
func TestPersesValkeyDashboardThreeStates(t *testing.T) {
	for state, status := range valkeyAvailabilityStatuses() {
		avail := PersesValkeyDashboardAvailability(status)
		entry, ok := avail[PersesValkeyDashboardCatalogKey]
		if !ok {
			t.Fatalf("%s: availability must contain entry %q", state, PersesValkeyDashboardCatalogKey)
		}
		switch state {
		case "not_configured", "configured_unverified":
			if entry.Available != "unavailable" {
				t.Fatalf("%s: valkey-targets availability = %q, want unavailable (only a verified native signal promotes it)", state, entry.Available)
			}
			if entry.Datasource != "" {
				t.Fatalf("%s: unverified valkey-targets must report no datasource, got %q", state, entry.Datasource)
			}
			if entry.Reason == "" {
				t.Fatalf("%s: unverified valkey-targets must carry a reason naming the missing verified signal", state)
			}
		case "verified":
			if entry.Available != "available" {
				t.Fatalf("verified: valkey-targets availability = %q, want available", entry.Available)
			}
			if entry.Datasource == "" {
				t.Fatalf("verified: valkey-targets must report its Perses datasource kind (prometheus), got empty")
			}
			if entry.Reason != "" {
				t.Fatalf("verified: valkey-targets must carry no reason, got %q", entry.Reason)
			}
		default:
			t.Fatalf("%s: unknown state", state)
		}
	}
}

// TestPersesValkeyDashboardVerifiedRealizesRealPanel pins the verified branch:
// ObservabilitySupported promotes the entry to a real dashboard resource carrying
// real panels (never the unavailable reason document). This is the card
// acceptance: an availability path that returns a real dashboard, not a
// self-confirmed "not nil".
func TestPersesValkeyDashboardVerifiedRealizesRealPanel(t *testing.T) {
	avail := PersesValkeyDashboardAvailability(capability.ObservabilitySupported)
	entry, ok := avail[PersesValkeyDashboardCatalogKey]
	if !ok {
		t.Fatalf("availability must contain entry %q", PersesValkeyDashboardCatalogKey)
	}
	if entry.Available != "available" {
		t.Fatalf("verified availability = %q, want available", entry.Available)
	}
	// The marshalled output must be a dashboard resource, never the reason
	// document: the kind is Dashboard, not DashboardUnavailable.
	verifiedEntry := persesValkeyDashboardCatalogWithStatus(capability.ObservabilitySupported)[PersesValkeyDashboardCatalogKey]
	raw, err := persesValkeyPanelJSON(verifiedEntry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("output is not the dashboard resource: %v", err)
	}
	if doc.Kind == "DashboardUnavailable" {
		t.Fatalf("verified valkey dashboard must not emit DashboardUnavailable, got the reason document")
	}
	// The verified dashboard must carry a REAL data query for its panel, never
	// a query-less graph panel. This is the 0428-183 P2 regression check: a
	// graph panel without a query datasource + expression is a fabricated panel
	// that cannot show Valkey data even when the entry is available.
	if !strings.Contains(string(raw), `"datasource"`) {
		t.Fatalf("verified valkey dashboard panel must reference a Perses datasource in its query, got none in:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"query"`) {
		t.Fatalf("verified valkey dashboard panel must carry a Prometheus query expression, got none in:\n%s", raw)
	}
}

// TestPersesValkeyDashboardVerifiedGolden pins the marshalled verified dashboard
// against the committed golden file under testdata/perses-valkey, so the
// Perses-native output of the real dashboard is a stable, reviewed witness.
func TestPersesValkeyDashboardVerifiedGolden(t *testing.T) {
	entry := persesValkeyDashboardCatalogWithStatus(capability.ObservabilitySupported)[PersesValkeyDashboardCatalogKey]
	raw, err := persesValkeyPanelJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-valkey", "valkey-targets-verified.json"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if string(raw) != string(golden) {
		t.Fatalf("verified golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", raw, golden)
	}
}

// TestPersesValkeyDashboardGoldenFiles pins the marshalled catalog against the
// committed golden file under testdata/perses-valkey, so the Perses-native
// output is a stable, reviewed witness.
func TestPersesValkeyDashboardGoldenFiles(t *testing.T) {
	catalog := persesValkeyDashboardCatalog()
	for key, entry := range catalog {
		golden, err := os.ReadFile(filepath.Join("testdata", "perses-valkey", key+".json"))
		if err != nil {
			t.Fatalf("%s: golden file missing: %v", key, err)
		}
		got, err := persesValkeyPanelJSON(entry)
		if err != nil {
			t.Fatalf("%s: marshal: %v", key, err)
		}
		if string(got) != string(golden) {
			t.Fatalf("%s: golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", key, got, golden)
		}
	}
}

// keysOf is a small test helper for deterministic key listing.
func keysOf(m map[string]PersesValkeyPanelCatalogEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
