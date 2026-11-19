package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// persesHostCatalogAllProven is the test witness of a deployment whose three
// backends have EACH proven their own signal: Prometheus via the resolved
// manifest host/provider-metrics verdict, Loki via the Alloy provider-log
// pipeline, Tempo via registered provider OTLP trace sources. Availability
// here is per-backend proof, never one backend inheriting another's health.
func persesHostCatalogAllProven() map[string]PersesHostPanelCatalogEntry {
	return persesHostDashboardCatalogWithWitness(capability.ObservabilitySupported, PersesBackendSignalWitness{
		PrometheusProven: true,
		LokiProven:       true,
		TempoProven:      true,
	})
}

// TestPersesHostDashboardCatalogHasRealBackendsOnly pins the shipped catalog:
// with every backend's OWN signal proven it contains one real dashboard per
// real BaseHarbor backend (prometheus, loki, tempo) and an explicit
// UNAVAILABLE entry per host/container view with no native signal (raw host
// node metrics, cAdvisor per-container stats). No fabricated panel ships for a
// view with no real signal.
func TestPersesHostDashboardCatalogHasRealBackendsOnly(t *testing.T) {
	catalog := persesHostCatalogAllProven()
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
	catalog := persesHostCatalogAllProven()
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

// TestPersesHostDashboardQueriesAreRealSignals replaces the former shared
// "baseharbor-providers" substring check (it blessed invalid Loki/Tempo
// selectors — CODEX-REVIEW act 1602-0428-077.md). Every real panel query must
// now select ONLY labels/attributes its OWN backend actually emits, proven
// line-by-line against the collector sources.
func TestPersesHostDashboardQueriesAreRealSignals(t *testing.T) {
	catalog := persesHostCatalogAllProven()
	queries := map[string]string{}
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
				queries[key] = spec.Query
			}
		}
	}
	// Prometheus: the job label is real because Prometheus scrape config
	// literally declares the provider job (internal/metrics/
	// prometheus_render.go:221 "- job_name: baseharbor-providers") and the
	// provider target files carry it (internal/metrics/prometheus_files.go:71
	// "job": "baseharbor-providers").
	if !strings.Contains(queries["prometheus-host-container"], `job="baseharbor-providers"`) {
		t.Fatalf("prometheus panel query %q must select the real provider scrape job job=\"baseharbor-providers\" (prometheus_files.go:71, prometheus_render.go:221)", queries["prometheus-host-container"])
	}
	// Loki: the selector must use the label the Alloy pipeline actually
	// relabels onto every provider stream (internal/logs/render.go:140-172
	// syslog rules target_label "baseharbor_provider", :292-306 journald
	// rules) — never a Prometheus job label, which Loki streams do not carry.
	loki := queries["loki-host-container"]
	if strings.Contains(loki, "job=") {
		t.Fatalf("loki panel query %q must not select a Prometheus job label: Loki streams carry baseharbor_provider/baseharbor_service from Alloy relabel (logs/render.go:145,152,296,300), never job", loki)
	}
	if !strings.Contains(loki, "baseharbor_provider") {
		t.Fatalf("loki panel query %q must select the emitted Alloy label baseharbor_provider (logs/render.go:145,152,296,300)", loki)
	}
	// Tempo: the selector must use an attribute BaseHarbor actually emits.
	// The provider verification span puts baseharbor.provider on the SPAN
	// (internal/telemetry/otlp_payload.go:28 keyValue("baseharbor.provider",
	// string(source.Provider))) while service.name is a RESOURCE attribute
	// with values "baseharbor" or the application name (otlp_payload.go:38,66;
	// internal/runtimeobservability/observer.go:400) — so the former
	// service.name="baseharbor-providers" selector could never resolve.
	tempo := queries["tempo-host-container"]
	if strings.Contains(tempo, `service.name="baseharbor-providers"`) {
		t.Fatalf("tempo panel query %q must not select service.name=\"baseharbor-providers\": no emitted OTLP resource attribute carries that value (telemetry/otlp_payload.go:38,66)", tempo)
	}
	if !strings.Contains(tempo, "baseharbor.provider") {
		t.Fatalf("tempo panel query %q must select the emitted span attribute baseharbor.provider (telemetry/otlp_payload.go:28)", tempo)
	}
}

// TestPersesHostDashboardJSONIsDeterministic pins the marshalling: the same
// entry marshals to the same bytes on every call, so the golden witness is
// stable.
func TestPersesHostDashboardJSONIsDeterministic(t *testing.T) {
	catalog := persesHostCatalogAllProven()
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
	catalog := persesHostCatalogAllProven()
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
	entries := persesHostCatalogAllProven()
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
	catalog := persesHostCatalogAllProven()
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
// PersesHostDashboardAvailability returns "available" for the PROMETHEUS
// backend ONLY for a signal resolved to ObservabilitySupported, and
// "unavailable" for every other resolved status (empty/absent,
// requires-adapter, other). Availability is authoritative, never a
// caller-supplied static positive. Loki and Tempo stay unavailable under this
// one-argument face — the resolved host/provider-metrics status is a
// PROMETHEUS verdict and must never color them (CODEX-REVIEW act
// 1602-0428-077.md).
func TestPersesHostDashboardAvailabilityConsumesAuthoritativeSignal(t *testing.T) {
	supported := PersesHostDashboardAvailability(capability.ObservabilitySupported)
	prom := supported["prometheus-host-container"]
	if prom.Available != PersesHostDashboardAvailable {
		t.Fatalf("prometheus-host-container: resolved ObservabilitySupported must be available, got %q", prom.Available)
	}
	if prom.Backend != "prometheus" || prom.Datasource != "prometheus" {
		t.Fatalf("prometheus-host-container: prometheus availability must carry its own backend/datasource, got backend=%q datasource=%q", prom.Backend, prom.Datasource)
	}
	for _, key := range []string{"loki-host-container", "tempo-host-container"} {
		if supported[key].Available != PersesHostDashboardUnavailable {
			t.Fatalf("%s: a Prometheus health verdict must NEVER make another backend available, got %q", key, supported[key].Available)
		}
		if !strings.Contains(supported[key].Reason, keyPart(key)) {
			t.Fatalf("%s: reason must name the backend's OWN missing signal, got %q", key, supported[key].Reason)
		}
	}
	for _, status := range []capability.ObservabilitySupportStatus{"", capability.ObservabilityRequiresAdapter, capability.ObservabilityUnsupported} {
		verdict := PersesHostDashboardAvailability(status)
		for _, key := range []string{"host-node-metrics", "container-stats", "prometheus-host-container", "loki-host-container", "tempo-host-container"} {
			if verdict[key].Available != PersesHostDashboardUnavailable {
				t.Fatalf("status %q: %s must be unavailable, got %q", status, key, verdict[key].Available)
			}
		}
	}
}

// TestPersesHostDashboardAvailabilityPerBackendNoInference pins the
// per-backend gates: each backend answers ONLY from its own signal witness.
// In particular the named correction case: Prometheus healthy, Loki WITHOUT
// its own signal -> Loki unavailable; and each backend available exactly when
// its own signal is proven regardless of the others.
func TestPersesHostDashboardAvailabilityPerBackendNoInference(t *testing.T) {
	cases := []struct {
		name      string
		status    capability.ObservabilitySupportStatus
		witness   PersesBackendSignalWitness
		wantProm  string
		wantLoki  string
		wantTempo string
	}{
		{"prometheus healthy, loki without signal -> loki unavailable",
			capability.ObservabilitySupported,
			PersesBackendSignalWitness{PrometheusProven: true},
			PersesHostDashboardAvailable, PersesHostDashboardUnavailable, PersesHostDashboardUnavailable},
		{"prometheus healthy, loki signal proven -> loki available",
			capability.ObservabilitySupported,
			PersesBackendSignalWitness{PrometheusProven: true, LokiProven: true},
			PersesHostDashboardAvailable, PersesHostDashboardAvailable, PersesHostDashboardUnavailable},
		{"prometheus unhealthy, loki signal proven -> loki stays available",
			"",
			PersesBackendSignalWitness{LokiProven: true},
			PersesHostDashboardUnavailable, PersesHostDashboardAvailable, PersesHostDashboardUnavailable},
		{"prometheus unhealthy, tempo signal proven -> tempo stays available",
			capability.ObservabilityRequiresAdapter,
			PersesBackendSignalWitness{TempoProven: true},
			PersesHostDashboardUnavailable, PersesHostDashboardUnavailable, PersesHostDashboardAvailable},
		{"all signals proven -> all available",
			capability.ObservabilitySupported,
			PersesBackendSignalWitness{PrometheusProven: true, LokiProven: true, TempoProven: true},
			PersesHostDashboardAvailable, PersesHostDashboardAvailable, PersesHostDashboardAvailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict := PersesHostDashboardAvailabilityWithWitness(tc.status, tc.witness)
			for key, want := range map[string]string{
				"prometheus-host-container": tc.wantProm,
				"loki-host-container":       tc.wantLoki,
				"tempo-host-container":      tc.wantTempo,
			} {
				if verdict[key].Available != want {
					t.Fatalf("%s: verdict = %q, want %q (status %q witness %+v)", key, verdict[key].Available, want, tc.status, tc.witness)
				}
				if want == PersesHostDashboardUnavailable && strings.TrimSpace(verdict[key].Reason) == "" {
					t.Fatalf("%s: unavailable verdict must carry a concrete reason", key)
				}
			}
		})
	}
}

// keyPart extracts the backend word of a host catalog key (loki / tempo).
func keyPart(key string) string {
	return strings.TrimSuffix(key, "-host-container")
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
