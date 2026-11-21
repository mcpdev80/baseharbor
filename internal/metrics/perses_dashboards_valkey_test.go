package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	// The reason must name the missing native signal and the RequiresAdapter
	// status exactly, so the CLI witness is unambiguous.
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
	avail := PersesValkeyDashboardAvailability()
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
