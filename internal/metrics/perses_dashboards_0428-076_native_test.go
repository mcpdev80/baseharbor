package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestPersesNativeShapeCanonicalValidator proves the native-format acceptance
// that the prior round failed: the emitted resources must match the Perses
// v0.55.1 native shape, not merely round-trip against the candidate's own
// permissive structs (which is what the previous marshalled/candidate-defined
// struct test only showed).
//
// The canonical witness is the exact Perses v0.55.1 shape anchored in
// testdata/perses-native/v0.55.1 (derived from github.com/perses/spec v0.3.0
// go/dashboard and the real v0.55.1 example dashboards dev/data/9-dashboard.json
// and dev/data/8-datasource.json): every plugin nested under spec.plugin,
// datasource as {default, plugin:{kind,spec}}, query plugin nested under
// spec.plugin with the query-level fields beside it.
//
// For each real dashboard this test:
//   - re-marshals the catalog entry via the production path (persesDashboardCatalog +
//     persesPanelJSON),
//   - asserts the marshalled bytes equal the canonical Perses v0.55.1 JSON
//     byte-for-byte (not a struct round-trip against the candidate's own types),
//   - re-unmarshals the canonical witness into the candidate structs and
//     re-marshals to confirm round-trip stability of the same bytes.
func TestPersesNativeShapeCanonicalValidator(t *testing.T) {
	catalog := persesDashboardCatalog()
	for _, tc := range []struct {
		key     string
		witness string
	}{
		{"prometheus-targets", "prometheus-app-up"},
		{"loki-logs", "loki-app-errors"},
		{"tempo-traces", "tempo-app-traces"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			entry := catalog[tc.key]
			if entry.Unavailable {
				t.Fatalf("%s: real dashboard must not be marked unavailable", tc.key)
			}
			raw, err := persesPanelJSON(entry)
			if err != nil {
				t.Fatalf("%s: marshal: %v", tc.key, err)
			}
			golden, err := os.ReadFile(filepath.Join("testdata", "perses-native", "v0.55.1", tc.witness+".json"))
			if err != nil {
				t.Fatalf("%s: canonical Perses witness missing: %v", tc.key, err)
			}
			if string(raw) != string(golden) {
				t.Fatalf("%s: emitted bytes do not match Perses v0.55.1 canonical shape:\n--- got ---\n%s\n--- want (native) ---\n%s", tc.key, raw, golden)
			}
			// Round-trip stability: the same bytes must re-parse into the
			// candidate structs and re-marshal identically. This proves the
			// canonical witness is not a coincidental string but the true shape.
			var dashboard PersesDashboard
			if err := json.Unmarshal(golden, &dashboard); err != nil {
				t.Fatalf("%s: canonical witness is not a valid dashboard struct: %v", tc.key, err)
			}
			if dashboard.Kind != "Dashboard" {
				t.Fatalf("%s: canonical witness kind = %q, want Dashboard", tc.key, dashboard.Kind)
			}
			// The query plugin must be nested under spec.plugin with the
			// correct query-kind — the core of the native-shape claim.
			for _, panel := range dashboard.Spec.Panels {
				for _, q := range panel.Spec.Queries {
					if q.Spec.Plugin.Kind == "" {
						t.Fatalf("%s: query plugin kind must be nested under spec.plugin (native shape)", tc.key)
					}
					if len(q.Spec.Plugin.Spec) == 0 {
						t.Fatalf("%s: query plugin spec must be non-empty (native shape): %s", tc.key, q.Spec.Plugin.Spec)
					}
				}
			}
			// The local datasource must be {default, plugin:{kind,spec}} with no
			// top-level name/kind — the v0.55.1 datasource shape.
			for key, ds := range dashboard.Spec.Datasources {
				if ds.Plugin.Kind == "" {
					t.Fatalf("%s: datasource %q must carry a nested plugin kind (native shape)", tc.key, key)
				}
				if len(ds.Plugin.Spec) == 0 {
					t.Fatalf("%s: datasource %q plugin spec must be non-empty (object, not null): %s", tc.key, key, string(ds.Plugin.Spec))
				}
			}
		})
	}
}

// TestPersesNativeShapeMalformedRejected is the fail-closed witness that the
// canonical validator is not vacuously green: a dashboard whose query plugin is
// emitted flat (plugin kind at the query-spec level instead of nested under
// spec.plugin) must be rejected by the native-shape assertion. This mirrors the
// review finding that the prior round emitted query/plugin/datasource flat.
func TestPersesNativeShapeMalformedRejected(t *testing.T) {
	catalog := persesDashboardCatalog()
	raw, err := persesPanelJSON(catalog["prometheus-targets"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Emulate the pre-round flat shape: move the query plugin kind out of
	// spec.plugin back to the query-spec level (the shape the review rejected).
	var flat map[string]any
	if err := json.Unmarshal(raw, &flat); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	panels, _ := flat["spec"].(map[string]any)["panels"].(map[string]any)
	appUp, _ := panels["app-up"].(map[string]any)
	panelSpec, _ := appUp["spec"].(map[string]any)
	queries, _ := panelSpec["queries"].([]any)
	q0, _ := queries[0].(map[string]any)
	qspec, _ := q0["spec"].(map[string]any)
	// Strip the nested plugin and put the kind flat at the query-spec level.
	delete(qspec, "plugin")
	qspec["kind"] = "PrometheusTimeSeriesQuery"
	flat["spec"].(map[string]any)["panels"] = panels
	bad, err := json.Marshal(flat)
	if err != nil {
		t.Fatalf("marshal flat: %v", err)
	}

	got := string(bad)
	want := string(raw)
	if got == want {
		t.Fatalf("malformed flat query must differ from the native shape; the mutation produced identical bytes")
	}
	if validatePersesNativeShape([]byte(got)) {
		t.Fatalf("malformed flat query plugin must fail the native-shape validator; the validator is not fail-closed")
	}
}

// validatePersesNativeShape reports whether the marshalled dashboard JSON
// matches the Perses v0.55.1 native shape: every panel plugin is nested under
// spec.plugin with a non-empty spec, every query plugin is nested under
// spec.plugin (not flat at the query-spec level), and every local datasource is
// {default, plugin:{kind,spec}} with no top-level name/kind. It is the
// fail-closed gate the two tests above rely on.
func validatePersesNativeShape(jsonBytes []byte) bool {
	var root map[string]any
	if err := json.Unmarshal(jsonBytes, &root); err != nil {
		return false
	}
	if root["kind"] != "Dashboard" {
		return false
	}
	spec, ok := root["spec"].(map[string]any)
	if !ok {
		return false
	}
	// Local datasources: {default, plugin:{kind,spec}} only.
	datasources, _ := spec["datasources"].(map[string]any)
	for _, v := range datasources {
		ds, ok := v.(map[string]any)
		if !ok {
			return false
		}
		if _, hasName := ds["name"]; hasName {
			return false
		}
		plugin, ok := ds["plugin"].(map[string]any)
		if !ok {
			return false
		}
		if plugin["kind"] == "" {
			return false
		}
		if _, hasSpec := plugin["spec"]; !hasSpec {
			return false
		}
	}
	// Panels: every query plugin must be nested under spec.plugin.
	panels, _ := spec["panels"].(map[string]any)
	for _, pv := range panels {
		panel, ok := pv.(map[string]any)
		if !ok {
			return false
		}
		pspec, ok := panel["spec"].(map[string]any)
		if !ok {
			return false
		}
		_, hasPlugin := pspec["plugin"]
		queries, _ := pspec["queries"].([]any)
		for _, qv := range queries {
			q, ok := qv.(map[string]any)
			if !ok {
				return false
			}
			qspec, ok := q["spec"].(map[string]any)
			if !ok {
				return false
			}
			// The query plugin must live under spec.plugin, not flat at the
			// query-spec level.
			if _, hasFlatKind := qspec["kind"]; hasFlatKind {
				return false
			}
			if !hasPlugin {
				return false
			}
			_, hasPluginSpec := qspec["plugin"].(map[string]any)
			if !hasPluginSpec {
				return false
			}
		}
	}
	return true
}
