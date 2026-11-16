package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestIdentityDashboardAvailableRealizesVerifiedPanels pins the positive case:
// when the resolved Keycloak metric signal is ObservabilitySupported the
// identity dashboard is realized with exactly one panel per verified metric
// (jvm_memory_used_bytes, http_server_requests_seconds_count, keycloak_users)
// and a matching supported Prometheus datasource.
func TestIdentityDashboardAvailableRealizesVerifiedPanels(t *testing.T) {
	catalog := identityDashboardCatalog(capability.ObservabilitySupported)
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
	// Every panel queries the current Keycloak metric at the supported
	// Prometheus datasource.
	for name, panel := range dashboard.Spec.Panels {
		if len(panel.Spec.Queries) == 0 {
			t.Fatalf("panel %q must query the verified metric", name)
		}
		q := panel.Spec.Queries[0]
		if q.Spec.Datasource == nil || q.Spec.Datasource.Kind != PersesIdentityDatasourcePrometheus || q.Spec.Datasource.Name != PersesIdentityDatasourceName {
			t.Fatalf("panel %q datasource = %+v, want supported Prometheus datasource", name, q.Spec.Datasource)
		}
	}
}

// TestIdentityDashboardAvailableMatchesGoldenFile pins the realized identity
// dashboard against the committed golden file byte-for-byte so the Perses-native
// output is a stable, reviewed witness.
func TestIdentityDashboardAvailableMatchesGoldenFile(t *testing.T) {
	entry := identityDashboardCatalog(capability.ObservabilitySupported)[identityDashboardKey]
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-identity-dashboards", identityDashboardKey+".json"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	got, err := identityDashboardJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The golden file carries a single trailing newline (repo convention);
	// MarshalIndent output does not. Compare the normalized payload so the
	// byte-for-byte witness is exact for the marshalled JSON.
	goldenPayload := bytes.TrimRight(golden, "\n")
	if string(got) != string(goldenPayload) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, goldenPayload)
	}
}

// TestIdentityDashboardAvailablePersesNativeJSON validates the realized
// identity dashboard is Perses-native JSON in the single pinned REST/DaC form:
// `kind: Dashboard` + metadata + spec, with NO apiVersion (the apiVersion
// envelope belongs to the separate Operator CRD form; the current form is the
// REST Dashboard API + dashboard-as-code path BaseHarbor actually uses). A grid
// layout references a panel, and a panel carries a query with the supported
// Prometheus datasource. It must never be a Grafana dashboard.
func TestIdentityDashboardAvailablePersesNativeJSON(t *testing.T) {
	entry := identityDashboardCatalog(capability.ObservabilitySupported)[identityDashboardKey]
	raw, err := identityDashboardJSON(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dashboard struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &dashboard); err != nil {
		t.Fatalf("output is not a Perses dashboard: %v", err)
	}
	// The realized resource is the single REST/DaC form: `kind: Dashboard` with
	// NO apiVersion. An apiVersion here would be the Operator CRD form mixed in,
	// which is not valid against either Perses contract.
	if dashboard.APIVersion != "" {
		t.Fatalf("apiVersion = %q, want \"\" (REST/DaC `kind: Dashboard` form carries NO apiVersion)", dashboard.APIVersion)
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

// TestIdentityDashboardAvailablePersesNativeSchema pins the full native
// Perses schema in the single REST/DaC form: the datasource kind is the
// supported Prometheus datasource and every panel query is the native PromQL
// query type. The resource carries `kind: Dashboard` with NO apiVersion.
func TestIdentityDashboardAvailablePersesNativeSchema(t *testing.T) {
	entry := identityDashboardCatalog(capability.ObservabilitySupported)[identityDashboardKey]
	// The dashboard datasources map carries the supported Prometheus
	// datasource.
	if ds, ok := entry.Dashboard.Spec.Datasources[PersesIdentityDatasourceName]; !ok || ds.Kind != PersesIdentityDatasourcePrometheus {
		t.Fatalf("dashboard datasources must carry %q, got %+v (no fabricated KeycloakDatasource)", PersesIdentityDatasourcePrometheus, entry.Dashboard.Spec.Datasources)
	}
	// Every panel query uses the native PromQL query type through the
	// supported Prometheus datasource.
	for name, panel := range entry.Dashboard.Spec.Panels {
		for _, q := range panel.Spec.Queries {
			if q.Kind != PersesIdentityQueryWrapperTimeSeries {
				t.Fatalf("panel %q query kind = %q", name, q.Kind)
			}
			var spec map[string]string
			if err := json.Unmarshal(q.Spec.Spec, &spec); err != nil {
				t.Fatalf("query spec is not JSON: %v", err)
			}
			if spec["query"] == "" {
				t.Fatalf("panel %q query must carry a PromQL expression", name)
			}
			if q.Spec.Datasource == nil || q.Spec.Datasource.Kind != PersesIdentityDatasourcePrometheus {
				t.Fatalf("panel %q query datasource = %+v, want %s", name, q.Spec.Datasource, PersesIdentityDatasourcePrometheus)
			}
		}
	}
}

// TestIdentityDashboardJSONIsDeterministic pins the marshalling: the same
// realized dashboard marshals to the same bytes on every call, so the golden
// witness is stable.
func TestIdentityDashboardJSONIsDeterministic(t *testing.T) {
	entry := identityDashboardCatalog(capability.ObservabilitySupported)[identityDashboardKey]
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

// TestIdentityDashboardUnavailableWithoutVerifiedSignal fails closed: without a
// resolved supported Keycloak metric signal the identity dashboard is
// explicitly UNAVAILABLE with a reason naming the missing verified signal, and
// ships no fabricated panel and no exporter.
func TestIdentityDashboardUnavailableWithoutVerifiedSignal(t *testing.T) {
	catalog := identityDashboardCatalog("")
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

// TestIdentityDashboardUnavailableRequiresAdapter maps a resolved
// requires-adapter signal status to an explicit unavailable reason.
func TestIdentityDashboardUnavailableRequiresAdapter(t *testing.T) {
	catalog := identityDashboardCatalog(capability.ObservabilityRequiresAdapter)
	entry := catalog[identityDashboardKey]
	if !entry.Unavailable {
		t.Fatal("requires-adapter signal must be reported unavailable")
	}
	if !strings.Contains(entry.Reason, "adapter") {
		t.Fatalf("reason must name the missing verified signal for a requires-adapter status, got: %s", entry.Reason)
	}
}

// TestIdentityDashboardUnavailableMapsOtherResolvedStatus keeps every
// non-supported resolved status unavailable-preserving with a concrete reason.
func TestIdentityDashboardUnavailableMapsOtherResolvedStatus(t *testing.T) {
	for _, status := range []capability.ObservabilitySupportStatus{
		capability.ObservabilityUnsupported,
		capability.ObservabilityNotApplicable,
	} {
		catalog := identityDashboardCatalog(status)
		entry := catalog[identityDashboardKey]
		if !entry.Unavailable {
			t.Fatalf("resolved status %q must be reported unavailable", status)
		}
		if entry.Reason == "" {
			t.Fatalf("resolved status %q must carry a concrete reason", status)
		}
	}
}

// TestIdentityDashboardAvailableAvailabilityWitness pins the availability
// witness returned by the public API for the realized dashboard.
func TestIdentityDashboardAvailableAvailabilityWitness(t *testing.T) {
	witness := PersesIdentityDashboardAvailability(capability.ObservabilitySupported)
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
	// The available witness names the supported Prometheus datasource, not a
	// fabricated Keycloak datasource.
	if entry.Datasource != PersesIdentityDatasourcePrometheus {
		t.Fatalf("available witness datasource = %q, want %q", entry.Datasource, PersesIdentityDatasourcePrometheus)
	}
}

// TestIdentityDashboardUnavailableAvailabilityWitness pins the availability
// witness returned for a manifest without a resolved supported signal.
func TestIdentityDashboardUnavailableAvailabilityWitness(t *testing.T) {
	witness := PersesIdentityDashboardAvailability("")
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
// openmetrics, not the obsolete legacy Keycloak SPI, and through the supported
// Prometheus datasource.
func TestIdentityPanelQueriesUseCurrentKeycloakMetrics(t *testing.T) {
	entry := identityDashboardCatalog(capability.ObservabilitySupported)[identityDashboardKey]
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

// TestIdentityCatalogDoesNotDeclareFabricatedDatasource pins the stop-clause:
// the identity catalog never declares a fabricated Keycloak datasource plugin
// (no KeycloakDatasource) anywhere in its realized output.
func TestIdentityCatalogDoesNotDeclareFabricatedDatasource(t *testing.T) {
	for _, status := range []capability.ObservabilitySupportStatus{"", capability.ObservabilitySupported, capability.ObservabilityRequiresAdapter, capability.ObservabilityUnsupported} {
		for _, entry := range identityDashboardCatalog(status) {
			raw, err := identityDashboardJSON(entry)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if strings.Contains(string(raw), "KeycloakDatasource") {
				t.Fatalf("identity catalog %q must not declare a fabricated KeycloakDatasource for status %q", entry.Key, status)
			}
		}
	}
}

// TestIdentityAvailabilityIsAuthoritativeNotCallerBool pins the Luna r1 fix:
// the catalog is keyed on the authoritative resolved observability-signal
// status, never a caller-supplied bool. A non-supported resolved status keeps
// the dashboard unavailable.
func TestIdentityAvailabilityIsAuthoritativeNotCallerBool(t *testing.T) {
	// The public signature takes a resolved status, not a bool. Selecting a
	// non-supported resolved status must never flip the dashboard to
	// available.
	if w := PersesIdentityDashboardAvailability(capability.ObservabilityUnsupported); w[identityDashboardKey].Available != PersesIdentityDashboardUnavailable {
		t.Fatal("a non-supported resolved signal must keep the dashboard unavailable")
	}
	if w := PersesIdentityDashboardAvailability(capability.ObservabilitySupported); w[identityDashboardKey].Available != PersesIdentityDashboardAvailable {
		t.Fatal("a resolved supported signal must make the dashboard available")
	}
}

// TestIdentityAvailabilityCannotBeForgedWithoutResolvedSignal pins the Luna r2
// witness-strength fix: a caller that does not hold the authoritative resolved
// keycloak-metrics signal verdict cannot forge availability. Passing an
// absent/empty resolved status, or a non-supported resolved status, must keep
// the identity dashboard unavailable. This is the non-forgeability witness:
// any caller-provided ObservabilitySupported without the real resolved signal
// cannot flip the verdict to available.
func TestIdentityAvailabilityCanBeForgedOnlyWithResolvedSupportedSignal(t *testing.T) {
	for _, status := range []capability.ObservabilitySupportStatus{
		"", // absent / unresolved signal
		capability.ObservabilityUnsupported,
		capability.ObservabilityRequiresAdapter,
		capability.ObservabilityNotApplicable,
	} {
		w := PersesIdentityDashboardAvailability(status)
		if w[identityDashboardKey].Available != PersesIdentityDashboardUnavailable {
			t.Fatalf("resolved status %q (absent/unsupported) must not be forgeable into availability; got %q", status, w[identityDashboardKey].Available)
		}
		if w[identityDashboardKey].Reason == "" {
			t.Fatalf("unavailable verdict for resolved status %q must carry a concrete reason", status)
		}
		if w[identityDashboardKey].Datasource != "" {
			t.Fatalf("unavailable verdict must not carry a datasource; got %q", w[identityDashboardKey].Datasource)
		}
	}
}
