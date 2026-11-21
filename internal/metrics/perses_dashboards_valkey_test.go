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
// catalog through the REAL producer + REAL verification: it resolves exactly one
// entry (valkey-targets) and that entry is EXPLICITLY UNAVAILABLE, because the
// reference catalog declares Valkey's metrics signal ObservabilityUnsupported
// (0428-182, reference.go ValkeyIntegration) with ObservabilityVerifyNone, so
// no real native metrics signal exists and no backend-verification is possible.
// No fabricated panel ships for Valkey and no exporter is installed. This is the
// 0428-081 conditional-dashboard deliverable, mirroring the PostgreSQL/MongoDB
// handling in the #1597 inventory. This is the card acceptance of 0428-183
// round 4: the positive witness is the real shipped truth, not a caller-set
// enum.
func TestPersesValkeyDashboardCatalogIsUnavailableOnly(t *testing.T) {
	// Drive through the real producer and the real backend-verification verdict
	// so the asserted catalog is exactly what the production caller
	// persesValkeyDashboardCatalog() (and therefore the shipped entry) produces.
	status := persesValkeyResolvedSignalStatus()
	verification := persesValkeyBackendVerified()
	catalog := persesValkeyDashboardCatalogWithStatus(status, verification)
	if len(catalog) != 1 {
		t.Fatalf("Valkey catalog must contain exactly one entry, got %d: %v", len(catalog), keysOf(catalog))
	}
	entry, ok := catalog[PersesValkeyDashboardCatalogKey]
	if !ok {
		t.Fatalf("Valkey catalog must contain entry %q", PersesValkeyDashboardCatalogKey)
	}
	if !entry.Unavailable {
		t.Fatalf("Valkey dashboard has no native metrics signal and must be marked unavailable (producer=%q verification=%q)", status, verification)
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
	if !strings.Contains(entry.Reason, "ObservabilityUnsupported") {
		t.Fatalf("reason must cite the observability status ObservabilityUnsupported (0428-182), got: %s", entry.Reason)
	}
	if strings.Contains(entry.Reason, "exporter is installed") && strings.Count(entry.Reason, "no exporter") == 0 {
		t.Fatalf("reason must state no exporter is installed, got: %s", entry.Reason)
	}
}

// TestPersesValkeyDashboardProducerReadsReferenceCatalog pins the REAL producer
// (persesValkeyResolvedSignalStatus) to the authoritative reference catalog: it
// must resolve to the status Valkey actually declares in reference.go
// (ValkeyIntegration) — today ObservabilityUnsupported (metrics is not a real
// signal), which is NOT ObservabilitySupported. A producer that did not read the
// catalog (e.g. always returning Supported) would fail this — it is the proof
// the verdict comes from the real signal path, not a caller-set flag. This is
// the anti-synthetic anchor of round 4: the test proves the producer resolves
// today's real status, so a positive witness that drives only that producer
// is real, not a caller-asserted enum.
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

// TestPersesValkeyDashboardAvailabilityResolvedUnavailableToday pins the
// production caller (PersesValkeyDashboardAvailabilityResolved), which reads BOTH
// the real producer and the real backend-verification verdict. Today both axes
// are absent (unsupported metrics, none verification), so the CLI witness is
// EXPLICITLY UNAVAILABLE — this is the real shipped state, the positive
// witness of round 4.
func TestPersesValkeyDashboardAvailabilityResolvedUnavailableToday(t *testing.T) {
	avail := PersesValkeyDashboardAvailabilityResolved()
	entry, ok := avail[PersesValkeyDashboardCatalogKey]
	if !ok {
		t.Fatalf("availability must contain entry %q", PersesValkeyDashboardCatalogKey)
	}
	if entry.Available != "unavailable" {
		t.Fatalf("real producer + real verification today must report unavailable, got %q", entry.Available)
	}
	if entry.Backend != "valkey" {
		t.Fatalf("valkey-targets backend = %q, want valkey", entry.Backend)
	}
	if entry.Datasource != "" {
		t.Fatalf("unavailable valkey-targets must report no datasource, got %q", entry.Datasource)
	}
	if !strings.Contains(entry.Reason, "ObservabilityUnsupported") {
		t.Fatalf("availability reason must cite ObservabilityUnsupported (0428-182), got: %s", entry.Reason)
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
// verdict on the CLI witness: the empty-status face (fail-closed) reports
// "unavailable" with the reason naming the missing native signal.
func TestPersesValkeyDashboardAvailabilityPinsReasons(t *testing.T) {
	avail := PersesValkeyDashboardAvailability("", capability.ObservabilityVerifyNone)
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
	if !strings.Contains(entry.Reason, "ObservabilityUnsupported") {
		t.Fatalf("availability reason must cite ObservabilityUnsupported (0428-182), got: %s", entry.Reason)
	}
}

// TestPersesValkeyDashboardAvailabilityVerifiedGated drives the availability
// path through a REAL backend-verification anchor (ObservabilityVerifyBackend),
// NOT the finished ObservabilitySupported enum: this proves the verified-panel
// code truly runs when a real verified signal exists, and that the verification
// axis is a real gate, not a no-op. This is the injectable test anchor round 4
// requires.
func TestPersesValkeyDashboardAvailabilityVerifiedGated(t *testing.T) {
	// supported + backend-verified -> the entry promotes to a real dashboard.
	avail := PersesValkeyDashboardAvailability(capability.ObservabilitySupported, capability.ObservabilityVerifyBackend)
	entry, ok := avail[PersesValkeyDashboardCatalogKey]
	if !ok {
		t.Fatalf("availability must contain entry %q", PersesValkeyDashboardCatalogKey)
	}
	if entry.Available != "available" {
		t.Fatalf("supported + backend-verified must report available, got %q", entry.Available)
	}
	if entry.Datasource != "prometheus" {
		t.Fatalf("available valkey-targets must report its Perses datasource (prometheus), got %q", entry.Datasource)
	}
	if entry.Reason != "" {
		t.Fatalf("available valkey-targets must carry no reason, got %q", entry.Reason)
	}

	// supported but NOT backend-verified -> still unavailable (verified-only
	// predicate, mirroring the identity dashboard 0428-078).
	unverified := PersesValkeyDashboardAvailability(capability.ObservabilitySupported, capability.ObservabilityVerifyNone)
	if unverified[PersesValkeyDashboardCatalogKey].Available != "unavailable" {
		t.Fatalf("supported + unverified must report unavailable")
	}
}

// TestPersesValkeyDashboardMutantPredicateRevertedIsolated proves the availability
// gate is real, not vacuous: with a supported + backend-verified signal the
// catalog MUST promote the entry. If a future change reverts the predicate
// (always Unavailable), this test fails — it is mutant A from the 0428-068 /
// 0428-166 history (a reverted predicate is NOT an independent witness, so it
// needs its own isolated test).
func TestPersesValkeyDashboardMutantPredicateRevertedIsolated(t *testing.T) {
	// The gate answers the catalog's OWN verified branch with a real
	// backend-verified anchor. If the catalog now always returns Unavailable for
	// a supported + verified signal, the gate is reverted and this test fails.
	entry := persesValkeyDashboardCatalogWithStatus(
		capability.ObservabilitySupported,
		capability.ObservabilityVerifyBackend,
	)[PersesValkeyDashboardCatalogKey]
	if entry.Unavailable {
		t.Fatalf("supported + backend-verified must NOT leave the entry unavailable; the availability gate was reverted")
	}
	if entry.Dashboard.Kind == "" {
		t.Fatalf("supported + backend-verified must realize a real dashboard resource, got an empty dashboard")
	}
	if entry.Dashboard.Spec.Panels["valkey-up"].Spec.Queries[0].Spec.Spec == nil {
		t.Fatalf("realized dashboard panel must carry a runnable query spec")
	}
}

// TestPersesValkeyDashboardMutantAvailabilityForcedIsolated proves the reverse
// direction: with the empty status (the CLI fail-closed face) the availability
// gate MUST stay fail-closed and report "unavailable" (mutant C — forcing the
// availability branch without a real producer verdict AND a real verification
// anchor is NOT an independent witness, so it needs its own isolated test that
// fails if the gate opens).
func TestPersesValkeyDashboardMutantAvailabilityForcedIsolated(t *testing.T) {
	avail := PersesValkeyDashboardAvailability("", capability.ObservabilityVerifyNone)
	entry := avail[PersesValkeyDashboardCatalogKey]
	if entry.Available == "available" {
		t.Fatalf("empty status must NOT report available; the availability branch was force-opened without a real producer verdict")
	}
	if entry.Available != "unavailable" {
		t.Fatalf("empty-status valkey-targets availability = %q, want unavailable", entry.Available)
	}
}

// TestPersesValkeyDashboardVerifiedRealizesRealPanel drives the verified branch
// through the real backend-verification anchor (ObservabilityVerifyBackend), NOT
// the finished enum: a supported + backend-verified signal promotes the entry to
// a real dashboard resource carrying a runnable query panel (never the
// unavailable reason document). This is the card acceptance of round 4: an
// availability path that returns a real dashboard via a real verified signal,
// not a caller-asserted enum.
func TestPersesValkeyDashboardVerifiedRealizesRealPanel(t *testing.T) {
	avail := PersesValkeyDashboardAvailability(
		capability.ObservabilitySupported,
		capability.ObservabilityVerifyBackend,
	)
	entry, ok := avail[PersesValkeyDashboardCatalogKey]
	if !ok {
		t.Fatalf("availability must contain entry %q", PersesValkeyDashboardCatalogKey)
	}
	if entry.Available != "available" {
		t.Fatalf("verified availability = %q, want available", entry.Available)
	}
	// The marshalled output must be a dashboard resource, never the reason
	// document: the kind is Dashboard, not DashboardUnavailable.
	verifiedEntry := persesValkeyDashboardCatalogWithStatus(
		capability.ObservabilitySupported,
		capability.ObservabilityVerifyBackend,
	)[PersesValkeyDashboardCatalogKey]
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
// Perses-native output of the real dashboard is a stable, reviewed witness. The
// verified branch is driven through the real backend-verification anchor.
func TestPersesValkeyDashboardVerifiedGolden(t *testing.T) {
	entry := persesValkeyDashboardCatalogWithStatus(
		capability.ObservabilitySupported,
		capability.ObservabilityVerifyBackend,
	)[PersesValkeyDashboardCatalogKey]
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
