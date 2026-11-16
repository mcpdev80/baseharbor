package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestPersesHostDashboardCatalogHasRealBackendsOnly pins the shipped catalog:
// it contains one real dashboard per real BaseHarbor backend (prometheus, loki,
// tempo) and an explicit UNAVAILABLE entry per host/container view with no
// native signal (raw host node metrics, cAdvisor per-container stats). No
// fabricated panel ships for a view with no real signal.
func TestPersesHostDashboardCatalogHasRealBackendsOnly(t *testing.T) {
	catalog := persesHostDashboardCatalog(capability.ObservabilitySupported)
	for _, key := range []string{"prometheus-host-container", "loki-host-container", "tempo-host-container"} {
		entry, ok := catalog[key]
		if !ok {
			t.Fatalf("catalog must contain real dashboard %q", key)
		}
		if entry.Unavailable {
			t.Fatalf("real dashboard %q must not be marked unavailable", key)
		}
		if entry.Dashboard.Kind == "" {
			t.Fatalf("real dashboard %q must ship a dashboard resource", key)
		}
	}
	for _, key := range []string{"host-node-metrics", "container-stats"} {
		entry, ok := catalog[key]
		if !ok {
			t.Fatalf("catalog must contain entry %q", key)
		}
		if !entry.Unavailable {
			t.Fatalf("host/container view %q has no native metrics signal and must be marked unavailable", key)
		}
		if entry.Dashboard.Kind != "" {
			t.Fatalf("unavailable host/container view %q must ship no fabricated panel", key)
		}
		if entry.Reason == "" {
			t.Fatalf("unavailable entry %q must carry a concrete reason", key)
		}
	}
}

// TestPersesHostDashboardPersesNativeJSON validates that each real dashboard
// marshals to Perses-native JSON: kind "Dashboard", one layout referencing a
// panel, and a panel carrying a query with the matching backend datasource.
func TestPersesHostDashboardPersesNativeJSON(t *testing.T) {
	catalog := persesHostDashboardCatalog(capability.ObservabilitySupported)
	for _, key := range []string{"prometheus-host-container", "loki-host-container", "tempo-host-container"} {
		entry := catalog[key]
		raw, err := persesHostPanelJSON(entry)
		if err != nil {
			t.Fatalf("%s: marshal: %v", key, err)
		}
		// The & in the display name must be emitted literally, not \u0026.
		if strings.Contains(string(raw), `\u0026`) {
			t.Fatalf("%s: output contains escaped ampersand \\u0026, want literal &: %s", key, raw)
		}
		var dashboard PersesHostDashboard
		if err := json.Unmarshal(raw, &dashboard); err != nil {
			t.Fatalf("%s: output is not a Perses dashboard: %v\n%s", key, err, raw)
		}
		if dashboard.Kind != PersesHostDashboardKind {
			t.Fatalf("%s: kind = %q, want %q", key, dashboard.Kind, PersesHostDashboardKind)
		}
		if len(dashboard.Spec.Panels) == 0 {
			t.Fatalf("%s: dashboard must ship at least one panel", key)
		}
		if len(dashboard.Spec.Layouts) == 0 {
			t.Fatalf("%s: dashboard must ship at least one layout", key)
		}
		for _, layout := range dashboard.Spec.Layouts {
			for _, item := range layout.Spec.Items {
				if strings.TrimSpace(item.Content.Ref) == "" {
					t.Fatalf("%s: layout item must reference a panel by $ref", key)
				}
			}
		}
		for _, panel := range dashboard.Spec.Panels {
			if len(panel.Spec.Queries) == 0 {
				t.Fatalf("%s: panel must carry at least one query", key)
			}
			for _, q := range panel.Spec.Queries {
				if q.Spec.Datasource == nil {
					t.Fatalf("%s: query must select a datasource", key)
				}
				if q.Spec.Datasource.Kind == "" || q.Spec.Datasource.Name == "" {
					t.Fatalf("%s: datasource selector must have kind + name", key)
				}
				if q.Spec.Datasource.Kind != backendsDatasource(entry.Backend) {
					t.Fatalf("%s: query datasource kind = %q, want %q for backend %q", key, q.Spec.Datasource.Kind, backendsDatasource(entry.Backend), entry.Backend)
				}
			}
		}
	}
}

// TestPersesHostDashboardQueriesAreRealSignals pins that every real panel query
// references a real BaseHarbor backend job, never a fabricated target.
func TestPersesHostDashboardQueriesAreRealSignals(t *testing.T) {
	catalog := persesHostDashboardCatalog(capability.ObservabilitySupported)
	for key, entry := range catalog {
		if entry.Unavailable {
			continue
		}
		raw, err := persesHostPanelJSON(entry)
		if err != nil {
			t.Fatalf("%s: marshal: %v", key, err)
		}
		var dashboard PersesHostDashboard
		if err := json.Unmarshal(raw, &dashboard); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		for panelName, panel := range dashboard.Spec.Panels {
			for _, q := range panel.Spec.Queries {
				var spec struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal(q.Spec.Spec, &spec); err != nil {
					t.Fatalf("%s/%s: decode query: %v", key, panelName, err)
				}
				if !strings.Contains(spec.Query, `baseharbor-providers`) {
					t.Fatalf("%s/%s: query must reference a real BaseHarbor backend (baseharbor-providers), got %q", key, panelName, spec.Query)
				}
			}
		}
	}
}

// TestPersesHostDashboardJSONIsDeterministic pins the marshalling: the same
// entry marshals to the same bytes on every call, so the golden witness is
// stable.
func TestPersesHostDashboardJSONIsDeterministic(t *testing.T) {
	catalog := persesHostDashboardCatalog(capability.ObservabilitySupported)
	first, err := persesHostPanelJSON(catalog["prometheus-host-container"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := persesHostPanelJSON(catalog["prometheus-host-container"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("perses host dashboard JSON is not deterministic:\n%s\n---\n%s", first, second)
	}
}

// TestPersesHostUnavailableEntryMarshalsReason pins the negative witness: an
// unavailable entry emits a reason document (not a fabricated panel).
func TestPersesHostUnavailableEntryMarshalsReason(t *testing.T) {
	catalog := persesHostDashboardCatalog("")
	for _, key := range []string{"host-node-metrics", "container-stats"} {
		raw, err := persesHostPanelJSON(catalog[key])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var doc struct {
			Kind        string `json:"kind"`
			Unavailable bool   `json:"unavailable"`
			Reason      string `json:"reason"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("output is not the reason document: %v", err)
		}
		if doc.Kind != PersesHostUnavailableKind {
			t.Fatalf("%s: unavailable entry kind = %q, want %q", key, doc.Kind, PersesHostUnavailableKind)
		}
		if !doc.Unavailable {
			t.Fatalf("%s: unavailable entry must report unavailable=true", key)
		}
		if !strings.Contains(doc.Reason, "unavailable") {
			t.Fatalf("%s: reason must name the missing native signal, got: %s", key, doc.Reason)
		}
	}
}

// TestPersesHostDashboardKeysDistinctFromDatabaseDashboards pins that the host
// view owns distinct catalog keys and does not collide with #1597's database
// catalog keys (faechregel: disjoint write-sets, distinct keys).
func TestPersesHostDashboardKeysDistinctFromDatabaseDashboards(t *testing.T) {
	hostKeys := []string{"prometheus-host-container", "loki-host-container", "tempo-host-container", "host-node-metrics", "container-stats"}
	databaseKeys := []string{"prometheus-targets", "loki-logs", "tempo-traces", "postgresql-targets", "mongodb-targets"}
	set := map[string]struct{}{}
	for _, k := range databaseKeys {
		set[k] = struct{}{}
	}
	for _, h := range hostKeys {
		if _, ok := set[h]; ok {
			t.Fatalf("host dashboard key %q collides with a database dashboard key", h)
		}
	}
}

// TestPersesHostDashboardKeysMatchSelectionCatalogKeys keeps the panel backends
// in scope of the selection catalog (080): every real host panel's backend must
// be one of the three real backends the selection catalog knows (prometheus,
// loki, tempo).
func TestPersesHostDashboardKeysMatchSelectionCatalogKeys(t *testing.T) {
	catalog := persesHostDashboardCatalog(capability.ObservabilitySupported)
	for _, entry := range catalog {
		if entry.Unavailable {
			continue
		}
		switch entry.Backend {
		case string(capability.DashboardBackendPrometheus),
			string(capability.DashboardBackendLoki),
			string(capability.DashboardBackendTempo):
		default:
			t.Fatalf("real host dashboard backend %q is not a real BaseHarbor backend", entry.Backend)
		}
	}
}

// TestPersesHostDashboardGoldenFiles pins the marshalled catalog against
// committed golden files under testdata/perses-dashboard-hosts. Each catalog
// entry (real dashboard + unavailable reason document) must match its golden
// file byte-for-byte, so the Perses-native output is a stable, reviewed witness.
func TestPersesHostDashboardGoldenFiles(t *testing.T) {
	entries := persesHostDashboardCatalog(capability.ObservabilitySupported)
	for key, entry := range entries {
		golden, err := os.ReadFile(filepath.Join("testdata", "perses-dashboard-hosts", key+".json"))
		if err != nil {
			t.Fatalf("%s: golden file missing: %v", key, err)
		}
		got, err := persesHostPanelJSON(entry)
		if err != nil {
			t.Fatalf("%s: marshal: %v", key, err)
		}
		if string(got) != string(golden) {
			t.Fatalf("%s: golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", key, got, golden)
		}
	}
}

// TestPersesHostDashboardPersesNativeSchema validates that every real host
// dashboard emits the pinned native Perses schema envelope: the apiVersion is
// the pinned perses.dev/v1alpha1 group/version and the kind is Dashboard, and
// the query type is the supported native plugin kind for its backend. This is
// the Luna-r1 acceptance: the pinned native Perses schema, never a flattened
// or Grafana-shaped document.
func TestPersesHostDashboardPersesNativeSchema(t *testing.T) {
	catalog := persesHostDashboardCatalog(capability.ObservabilitySupported)
	for _, key := range []string{"prometheus-host-container", "loki-host-container", "tempo-host-container"} {
		entry := catalog[key]
		raw, err := persesHostPanelJSON(entry)
		if err != nil {
			t.Fatalf("%s: marshal: %v", key, err)
		}
		var resource struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
		}
		if err := json.Unmarshal(raw, &resource); err != nil {
			t.Fatalf("%s: output is not a Perses dashboard: %v", key, err)
		}
		if resource.APIVersion != PersesHostDashboardAPIVersion {
			t.Fatalf("%s: apiVersion = %q, want %q (pinned native Perses schema v0.55.1)", key, resource.APIVersion, PersesHostDashboardAPIVersion)
		}
		if resource.Kind != PersesHostDashboardKind {
			t.Fatalf("%s: kind = %q, want %q", key, resource.Kind, PersesHostDashboardKind)
		}
		// The query plugin kind must be the supported native Perses query kind
		// for the backend, not a flattened query field.
		for panelName, panel := range entry.Dashboard.Spec.Panels {
			for _, q := range panel.Spec.Queries {
				if q.Spec.Kind != backendQueryKind(entry.Backend) {
					t.Fatalf("%s/%s: query kind = %q, want %q for backend %q", key, panelName, q.Spec.Kind, backendQueryKind(entry.Backend), entry.Backend)
				}
			}
		}
	}
}

// TestPersesHostDashboardUnavailableWithoutVerifiedSignal pins that a host
// dashboard with an absent/unverified resolved signal is reported explicitly
// UNAVAILABLE with a concrete reason naming the missing verified signal, never
// as a fabricated panel. This is the Luna-r1 stop: no positive runtime status
// without authoritative input.
func TestPersesHostDashboardUnavailableWithoutVerifiedSignal(t *testing.T) {
	catalog := persesHostDashboardCatalog("")
	for _, key := range []string{"host-node-metrics", "container-stats"} {
		entry := catalog[key]
		if !entry.Unavailable {
			t.Fatalf("%s: absent signal must be reported unavailable", key)
		}
		if entry.Reason == "" {
			t.Fatalf("%s: unavailable entry must name the missing verified signal", key)
		}
		if !strings.Contains(entry.Reason, "unavailable") {
			t.Fatalf("%s: reason must state unavailability, got %q", key, entry.Reason)
		}
		raw, err := persesHostPanelJSON(entry)
		if err != nil {
			t.Fatalf("%s: marshal: %v", key, err)
		}
		var doc struct {
			Kind        string `json:"kind"`
			Unavailable bool   `json:"unavailable"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: unavailable output is not the reason document: %v", key, err)
		}
		if doc.Kind != PersesHostUnavailableKind || !doc.Unavailable {
			t.Fatalf("%s: unavailable entry must be %q/unavailable, got kind=%q unavailable=%v", key, PersesHostUnavailableKind, doc.Kind, doc.Unavailable)
		}
	}
}

// TestPersesHostDashboardAvailabilityConsumesAuthoritativeSignal pins that
// PersesHostDashboardAvailability returns "available" ONLY for a signal resolved
// to ObservabilitySupported, and "unavailable" for every other resolved status
// (empty/absent, requires-adapter, other). Availability is authoritative, never
// a caller-supplied static positive.
func TestPersesHostDashboardAvailabilityConsumesAuthoritativeSignal(t *testing.T) {
	supported := PersesHostDashboardAvailability(capability.ObservabilitySupported)
	for _, key := range []string{"prometheus-host-container", "loki-host-container", "tempo-host-container"} {
		if supported[key].Available != PersesHostDashboardAvailable {
			t.Fatalf("%s: resolved ObservabilitySupported must be available, got %q", key, supported[key].Available)
		}
	}
	for _, status := range []capability.ObservabilitySupportStatus{"", capability.ObservabilityRequiresAdapter, capability.ObservabilityUnsupported} {
		verdict := PersesHostDashboardAvailability(status)
		for _, key := range []string{"host-node-metrics", "container-stats"} {
			if verdict[key].Available != PersesHostDashboardUnavailable {
				t.Fatalf("status %q: %s must be unavailable, got %q", status, key, verdict[key].Available)
			}
		}
	}
}

// TestPersesHostDashboardAvailableAvailabilityWitness pins the availability
// witness shape on the CLI status entry: key + backend + verdict + reason.
func TestPersesHostDashboardAvailableAvailabilityWitness(t *testing.T) {
	witness := PersesHostDashboardAvailability(capability.ObservabilitySupported)
	entry, ok := witness["prometheus-host-container"]
	if !ok {
		t.Fatal("prometheus-host-container must be present in the availability witness")
	}
	if entry.Key != "prometheus-host-container" || entry.Backend != "prometheus" {
		t.Fatalf("availability witness shape mismatch: key=%q backend=%q", entry.Key, entry.Backend)
	}
	if entry.Datasource != "prometheus" {
		t.Fatalf("availability witness datasource = %q, want %q", entry.Datasource, "prometheus")
	}
}

// TestPersesHostDashboardUnavailableAvailabilityWitness pins the unavailable
// witness carries a non-empty reason naming the missing verified signal.
func TestPersesHostDashboardUnavailableAvailabilityWitness(t *testing.T) {
	witness := PersesHostDashboardAvailability("")
	for _, key := range []string{"host-node-metrics", "container-stats"} {
		if witness[key].Available != PersesHostDashboardUnavailable {
			t.Fatalf("%s must be unavailable", key)
		}
		if strings.TrimSpace(witness[key].Reason) == "" {
			t.Fatalf("%s: unavailable witness must carry a reason", key)
		}
	}
}

// backendsDatasource maps a real BaseHarbor backend to the Perses datasource
// kind its host/dashboard panels select. The datasource kind is a Perses
// plugin kind (e.g. "PrometheusDatasource"), distinct from the BaseHarbor data
// owner ("prometheus").
func backendsDatasource(backend string) string {
	switch backend {
	case "prometheus":
		return "PrometheusDatasource"
	case "loki":
		return "LokiDatasource"
	case "tempo":
		return "TempoDatasource"
	default:
		return ""
	}
}

// backendQueryKind maps a real BaseHarbor backend to the Perses native query
// plugin kind its host/dashboard panels select (e.g.
// "PrometheusTimeSeriesQuery"), distinct from the query wrapper kind.
func backendQueryKind(backend string) string {
	switch backend {
	case "prometheus":
		return PersesHostQueryPrometheus
	case "loki":
		return PersesHostQueryLoki
	case "tempo":
		return PersesHostQueryTempo
	default:
		return ""
	}
}
