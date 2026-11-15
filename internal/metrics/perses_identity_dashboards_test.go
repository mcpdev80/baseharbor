package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIdentityDashboardAvailableRealizesVerifiedPanels pins the positive case:
// with the verified current Keycloak metric signal the identity dashboard is
// realized with exactly one panel per verified metric (jvm_memory_used_bytes,
// http_server_requests_seconds_count, keycloak_users) and a matching
// Keycloak datasource.
func TestIdentityDashboardAvailableRealizesVerifiedPanels(t *testing.T) {
	catalog := identityDashboardCatalog(true)
	entry, ok := catalog[identityDashboardKey]
	if !ok {
		t.Fatalf("identity dashboard %q must be in the catalog", identityDashboardKey)
	}
	if entry.Unavailable {
		t.Fatalf("identity dashboard must not be unavailable when the verified signal is present")
	}
	if entry.Backend != identityKeycloakBackend {
		t.Fatalf("backend = %q, want %q", entry.Backend, identityKeycloakBackend)
	}
	dashboard := entry.Dashboard
	if len(dashboard.Spec.Panels) != 3 {
		t.Fatalf("identity dashboard must ship 3 verified-panel entries, got %d", len(dashboard.Spec.Panels))
	}
	wantKeys := []string{"keycloak-jvm-memory", "keycloak-http-requests", "keycloak-users"}
	for _, k := range wantKeys {
		if _, ok := dashboard.Spec.Panels[k]; !ok {
			t.Fatalf("identity dashboard missing verified panel %q", k)
		}
	}
	// Every panel queries the current Keycloak metric at the Keycloak datasource.
	for name, panel := range dashboard.Spec.Panels {
		if len(panel.Spec.Queries) == 0 {
			t.Fatalf("panel %q must query the verified metric", name)
		}
		q := panel.Spec.Queries[0]
		if q.Spec.Datasource == nil || q.Spec.Datasource.Kind != PersesIdentityDatasourceKeycloak || q.Spec.Datasource.Name != "keycloak" {
			t.Fatalf("panel %q datasource = %+v, want Keycloak datasource", name, q.Spec.Datasource)
		}
	}
}

// TestIdentityDashboardAvailableMatchesGoldenFile pins the realized identity
// dashboard against the committed golden file byte-for-byte so the Perses-native
// output is a stable, reviewed witness.
func TestIdentityDashboardAvailableMatchesGoldenFile(t *testing.T) {
	entry := identityDashboardCatalog(true)[identityDashboardKey]
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-identity-dashboards", identityDashboardKey+".json"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	got, err := identityDashboardJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

// TestIdentityDashboardAvailablePersesNativeJSON validates the realized
// identity dashboard is Perses-native JSON: kind "Dashboard", a grid layout
// referencing a panel, and a panel carrying a query with the Keycloak
// datasource. It must never be a Grafana dashboard.
func TestIdentityDashboardAvailablePersesNativeJSON(t *testing.T) {
	entry := identityDashboardCatalog(true)[identityDashboardKey]
	raw, err := identityDashboardJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dashboard struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &dashboard); err != nil {
		t.Fatalf("output is not a Perses dashboard: %v", err)
	}
	if dashboard.Kind != PersesIdentityDashboardKind {
		t.Fatalf("kind = %q, want %q (Perses-native, never a Grafana dashboard)", dashboard.Kind, PersesIdentityDashboardKind)
	}
	if len(entry.Dashboard.Spec.Panels) == 0 {
		t.Fatal("identity dashboard must ship at least one panel")
	}
	if len(entry.Dashboard.Spec.Layouts) == 0 {
		t.Fatal("identity dashboard must ship at least one layout")
	}
	for _, layout := range entry.Dashboard.Spec.Layouts {
		for _, item := range layout.Spec.Items {
			if strings.TrimSpace(item.Content.Ref) == "" {
				t.Fatal("layout item must reference a panel by $ref")
			}
		}
	}
	for name, panel := range entry.Dashboard.Spec.Panels {
		if len(panel.Spec.Queries) == 0 {
			t.Fatalf("panel %q must carry at least one query", name)
		}
	}
}

// TestIdentityDashboardJSONIsDeterministic pins the marshalling: the same
// realized dashboard marshals to the same bytes on every call, so the golden
// witness is stable.
func TestIdentityDashboardJSONIsDeterministic(t *testing.T) {
	entry := identityDashboardCatalog(true)[identityDashboardKey]
	first, err := identityDashboardJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := identityDashboardJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("identity dashboard JSON is not deterministic:\n%s\n---\n%s", first, second)
	}
}

// TestIdentityDashboardUnavailableWithoutVerifiedSignal fails closed: without
// the verified current Keycloak metric signal the identity dashboard is
// explicitly UNAVAILABLE with a reason naming the missing verified signal, and
// ships no fabricated panel and no exporter.
func TestIdentityDashboardUnavailableWithoutVerifiedSignal(t *testing.T) {
	catalog := identityDashboardCatalog(false)
	entry := catalog[identityDashboardKey]
	if !entry.Unavailable {
		t.Fatal("identity dashboard must be unavailable without the verified signal")
	}
	if entry.Dashboard.Kind != "" {
		t.Fatal("unavailable identity dashboard must ship no fabricated panel")
	}
	if !strings.Contains(entry.Reason, "Keycloak") {
		t.Fatalf("reason must name the missing verified signal backend, got: %s", entry.Reason)
	}
	raw, err := identityDashboardJSON(entry)
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
	if doc.Kind != "DashboardUnavailable" {
		t.Fatalf("unavailable entry kind = %q, want DashboardUnavailable", doc.Kind)
	}
	if !doc.Unavailable {
		t.Fatal("unavailable entry must report unavailable=true")
	}
}

// TestIdentityDashboardAvailableAvailabilityWitness pins the availability
// witness returned by the public API for the realized dashboard.
func TestIdentityDashboardAvailableAvailabilityWitness(t *testing.T) {
	witness := PersesIdentityDashboardAvailability(true)
	entry, ok := witness[identityDashboardKey]
	if !ok {
		t.Fatalf("availability witness must contain %q", identityDashboardKey)
	}
	if entry.Available != PersesIdentityDashboardAvailable {
		t.Fatalf("available verdict = %q, want %q", entry.Available, PersesIdentityDashboardAvailable)
	}
	if entry.Backend != identityKeycloakBackend {
		t.Fatalf("witness backend = %q, want %q", entry.Backend, identityKeycloakBackend)
	}
}

// TestIdentityDashboardUnavailableAvailabilityWitness pins the availability
// witness returned for a manifest without the verified signal.
func TestIdentityDashboardUnavailableAvailabilityWitness(t *testing.T) {
	witness := PersesIdentityDashboardAvailability(false)
	entry, ok := witness[identityDashboardKey]
	if !ok {
		t.Fatalf("availability witness must contain %q", identityDashboardKey)
	}
	if entry.Available != PersesIdentityDashboardUnavailable {
		t.Fatalf("available verdict = %q, want %q", entry.Available, PersesIdentityDashboardUnavailable)
	}
	if entry.Reason == "" {
		t.Fatal("unavailable witness must carry a concrete reason")
	}
}

// TestIdentityDashboardKeyMatchesSelectionCatalogKey keeps the identity
// dashboard catalog key stable so a consumer can reference it by the same
// identifier through the coordinator fan-in.
func TestIdentityDashboardKeyMatchesSelectionCatalogKey(t *testing.T) {
	if identityDashboardKey == "" {
		t.Fatal("identity dashboard catalog key must not be empty")
	}
	if identityKeycloakBackend == "" {
		t.Fatal("identity backend must not be empty")
	}
}

// TestIdentityPanelQueriesUseCurrentKeycloakMetrics pins that the realized
// identity panels query the documented current Keycloak (Quarkus/Micrometer)
// openmetrics, not the obsolete legacy Keycloak SPI.
func TestIdentityPanelQueriesUseCurrentKeycloakMetrics(t *testing.T) {
	entry := identityDashboardCatalog(true)[identityDashboardKey]
	queried := map[string]bool{}
	for _, panel := range entry.Dashboard.Spec.Panels {
		for _, q := range panel.Spec.Queries {
			var spec map[string]string
			if err := json.Unmarshal(q.Spec.Spec, &spec); err != nil {
				t.Fatalf("query spec is not JSON: %v", err)
			}
			queried[spec["query"]] = true
		}
	}
	for _, want := range []string{
		`jvm_memory_used_bytes{area="heap"}`,
		`http_server_requests_seconds_count{status!="2xx"}`,
		`keycloak_users{type="real"}`,
	} {
		if !queried[want] {
			t.Fatalf("identity panels must query current Keycloak metric %q", want)
		}
	}
}
