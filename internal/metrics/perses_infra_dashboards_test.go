package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// seriesScriptProbe answers the per-panel series query seam with a SCRIPTED
// per-query verdict: each query string maps to present/absent. It also records
// every (endpoint, query) pair the builder asked about, so a test can assert
// that each panel's state travelled through the panel's OWN query — the exact
// path production takes via queryUp over the management endpoint.
type seriesScriptProbe struct {
	mu       sync.Mutex
	answers  map[string]bool
	endpoint string
	asked    []string
	err      error
}

func (p *seriesScriptProbe) querySeries(_ context.Context, endpoint, query string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endpoint = endpoint
	p.asked = append(p.asked, query)
	if p.err != nil {
		return false, p.err
	}
	return p.answers[query], nil
}

func (p *seriesScriptProbe) askedQueries() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.asked...)
}

// persesInfraAllPresent scripts every one of the four panel series present.
func persesInfraAllPresent(queries []string) *seriesScriptProbe {
	answers := map[string]bool{}
	for _, q := range queries {
		answers[q] = true
	}
	return &seriesScriptProbe{answers: answers}
}

// persesInfraQueriesOf returns the four panel queries a materialized source
// set produces, in deterministic key order.
func persesInfraQueriesOf(t *testing.T, m application.Manifest, sources []observability.MetricsSource) map[string]string {
	t.Helper()
	queries := map[string]string{}
	for _, key := range infraPanelKeys() {
		queries[key.Key] = infraQueryFor(key, sources, m)
		if queries[key.Key] == "" {
			t.Fatalf("panel %q must have a scoped query for a materialized source", key.Key)
		}
	}
	return queries
}

// TestPersesInfraDashboardPositiveFilledSignal pins the positive regression:
// a selected, pinned surface with a materialized application source and a
// probe that answers EVERY panel series through its own query builds the
// native Perses Dashboard resource with the four panels, all FILLED
// (MissingSignal=false) and each panel's own query seen on the probe path.
func TestPersesInfraDashboardPositiveFilledSignal(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	queries := persesInfraQueriesOf(t, m, []observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)})
	probe := persesInfraAllPresent([]string{queries["cpu"], queries["memory"], queries["disk"], queries["network"]})

	res, err := BuildPersesInfraDashboardWithProbe(m, probe)
	if err != nil {
		t.Fatalf("selected+pinned+verified datasource must build a dashboard: %v", err)
	}
	if res.MissingSignalCount() != 0 {
		t.Fatalf("every panel answered present, want 0 missing, got %d (signals %+v)", res.MissingSignalCount(), res.Signals)
	}
	d := res.Dashboard
	if d.Kind != "Dashboard" {
		t.Fatalf("dashboard kind = %q, want %q (pinned native Perses resource kind)", d.Kind, PersesInfraResourceKind)
	}
	if d.Metadata.Name != PersesInfraDashboardName || d.Metadata.Project != PersesInfraProject {
		t.Fatalf("dashboard metadata = %+v, want name %q in project %q", d.Metadata, PersesInfraDashboardName, PersesInfraProject)
	}
	if d.Owner != string(capability.ProviderPrometheus) {
		t.Fatalf("dashboard owner = %q, want %q (the real observability owner, as an owner reference)", d.Owner, string(capability.ProviderPrometheus))
	}
	if want := 4; len(d.Spec.Panels) != want {
		t.Fatalf("dashboard panels = %d, want %d", len(d.Spec.Panels), want)
	}
	// The probe must have been asked exactly one query per panel (per-panel
	// path, Luna batch17 finding 2), and every panel query must be distinct.
	asked := probe.askedQueries()
	if len(asked) != 4 {
		t.Fatalf("probe was asked %d queries, want 4 (one per panel)", len(asked))
	}
	seen := map[string]bool{}
	for _, q := range asked {
		if seen[q] {
			t.Fatalf("panel queries are not distinct: %q asked twice", q)
		}
		seen[q] = true
	}
	for _, key := range infraPanelKeys() {
		panel, ok := d.Spec.Panels[key.Key]
		if !ok {
			t.Fatalf("panel %q missing from spec.panels", key.Key)
		}
		if panel.MissingSignal {
			t.Fatalf("panel %q reports MissingSignal with its own series answered present", key.Key)
		}
		if panel.Kind != PersesInfraPanelKind {
			t.Fatalf("panel %q kind = %q, want %q", key.Key, panel.Kind, PersesInfraPanelKind)
		}
		if panel.Spec.Plugin.Kind != PersesInfraPanelPluginKind {
			t.Fatalf("panel %q plugin = %q, want the pinned native %q", key.Key, panel.Spec.Plugin.Kind, PersesInfraPanelPluginKind)
		}
		if want := 1; len(panel.Spec.Queries) != want {
			t.Fatalf("panel %q queries = %d, want %d", key.Key, len(panel.Spec.Queries), want)
		}
		wrapper := panel.Spec.Queries[0]
		if wrapper.Kind != PersesInfraQueryWrapperKind || wrapper.Spec.Plugin.Kind != PersesInfraQueryPluginKind {
			t.Fatalf("panel %q query kinds = (%q, %q), want (%q, %q)", key.Key, wrapper.Kind, wrapper.Spec.Plugin.Kind, PersesInfraQueryWrapperKind, PersesInfraQueryPluginKind)
		}
		var qspec PersesInfraPrometheusQuery
		if err := json.Unmarshal(wrapper.Spec.Plugin.Spec, &qspec); err != nil {
			t.Fatalf("panel %q query spec: %v", key.Key, err)
		}
		if qspec.Query != queries[key.Key] {
			t.Fatalf("panel %q query = %q, want the panel's own scoped query %q", key.Key, qspec.Query, queries[key.Key])
		}
		if qspec.Datasource == nil || qspec.Datasource.Kind != PersesInfraDatasourceSelectorKind || qspec.Datasource.Name != PersesInfraDatasourceName {
			t.Fatalf("panel %q datasource selector = %+v, want the installed %s/%s", key.Key, qspec.Datasource, PersesInfraDatasourceSelectorKind, PersesInfraDatasourceName)
		}
	}
}

// TestPersesInfraDashboardNegativeMissingSignalPerPanel pins the negative
// regression THROUGH THE PANEL'S OWN PATH: the probe answers every panel
// series present EXCEPT the memory family. Exactly the memory panel carries
// the visible missing state; the other three stay filled. A generic up{}
// verdict cannot produce this shape, which is the point of the witness.
func TestPersesInfraDashboardNegativeMissingSignalPerPanel(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	queries := persesInfraQueriesOf(t, m, []observability.MetricsSource{persesProviderSource("tempo:runtime", capability.ProviderTempo)})
	probe := persesInfraAllPresent([]string{queries["cpu"], queries["disk"], queries["network"]})

	res, err := BuildPersesInfraDashboardWithProbe(m, probe)
	if err != nil {
		t.Fatalf("selected+pinned datasource still builds with one missing panel series: %v", err)
	}
	if res.MissingSignalCount() != 1 {
		t.Fatalf("missing panels = %d, want 1 (only the memory panel's series was unanswered): %+v", res.MissingSignalCount(), res.Signals)
	}
	for _, key := range infraPanelKeys() {
		panel := res.Dashboard.Spec.Panels[key.Key]
		want := key.Key == "memory"
		if panel.MissingSignal != want {
			t.Fatalf("panel %q MissingSignal = %v, want %v", key.Key, panel.MissingSignal, want)
		}
	}
	// The gap stays VISIBLE: the panel itself (title, query, datasource) is
	// still part of the resource — the missing state never deletes a panel or
	// fabricates a value.
	mem := res.Dashboard.Spec.Panels["memory"]
	if mem.Spec.Display == nil || mem.Spec.Display.Name == "" {
		t.Fatal("the missing panel must keep its display name (visible gap, not a hidden panel)")
	}
	if len(mem.Spec.Queries) != 1 {
		t.Fatalf("the missing panel must keep exactly one query, got %d", len(mem.Spec.Queries))
	}
	var memQuery PersesInfraPrometheusQuery
	if err := json.Unmarshal(mem.Spec.Queries[0].Spec.Plugin.Spec, &memQuery); err != nil {
		t.Fatalf("missing panel query spec: %v", err)
	}
	if strings.TrimSpace(memQuery.Query) != queries["memory"] {
		t.Fatalf("the missing panel must keep its own scoped query, got %q", memQuery.Query)
	}
	// The serialized resource must NOT carry a private per-panel status field.
	raw, err := json.Marshal(res.Dashboard)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "MissingSignal") || strings.Contains(string(raw), "missingSignal") {
		t.Fatalf("the serialized native resource must not carry a private missing-signal field: %s", raw)
	}
}

// TestPersesInfraDashboardProbeErrorStaysMissing pins that a probe transport
// failure keeps the affected panel MISSING (fail toward showing the gap),
// never a silent filled panel: the builder still produces the dashboard, with
// every panel visible-but-missing.
func TestPersesInfraDashboardProbeErrorStaysMissing(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesInfraDashboardWithProbe(m, &seriesScriptProbe{answers: map[string]bool{}, err: errors.New("connection refused")})
	if err != nil {
		t.Fatalf("a failing series query keeps the dashboard buildable with visible gaps: %v", err)
	}
	if res.MissingSignalCount() != 4 {
		t.Fatalf("missing panels = %d, want 4 (a transport failure never fills a panel): %+v", res.MissingSignalCount(), res.Signals)
	}
}

// TestPersesInfraDashboardNoGenericProbeFillsPanels pins that the datasource
// health probe (recordingProbe, the generic up{} path) can NEVER decide a
// panel: wiring it into the old BuildPersesInfraDashboardWithProbe shape must
// not exist anymore, and the unobserved build keeps all panels missing.
func TestPersesInfraDashboardNoGenericProbeFillsPanels(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesInfraDashboard(m)
	if err != nil {
		t.Fatalf("selected+pinned datasource must build the resource: %v", err)
	}
	if res.MissingSignalCount() != 4 {
		t.Fatalf("the probe-less build must keep every panel explicitly missing, got %+v", res.Signals)
	}
	// The result signals must name the real panel keys; there is no generic
	// single-verdict field on the result anymore.
	if len(res.Signals) != 4 {
		t.Fatalf("signals = %d, want 4 (per-panel inventory)", len(res.Signals))
	}
	for _, s := range res.Signals {
		if s.Query == "" {
			t.Fatalf("signal %q carries no query of its own", s.Panel)
		}
	}
}

// TestPersesInfraDashboardQueriesUseRealScrapedSeries pins Luna batch17
// finding 3: no fabricated node_exporter series, no invented job, and the
// real scrape labels in every query.
func TestPersesInfraDashboardQueriesUseRealScrapedSeries(t *testing.T) {
	m := persesPrometheusDatasourceManifest()
	sources := []observability.MetricsSource{
		persesProviderSource("tempo:runtime", capability.ProviderTempo),
	}
	for _, key := range infraPanelKeys() {
		q := infraQueryFor(key, sources, m)
		if q == "" {
			t.Fatalf("panel %q has no query for a materialized source", key.Key)
		}
		for _, forbidden := range []string{"node_", "process_", `job="prometheus"`} {
			if strings.Contains(q, forbidden) {
				t.Fatalf("panel %q query %q carries the fabricated fragment %q (Core provisions no node_exporter and the job is not baseharbor-*)", key.Key, q, forbidden)
			}
		}
		if !strings.Contains(q, `baseharbor_provider="tempo"`) || !strings.Contains(q, `baseharbor_source="tempo:runtime"`) {
			t.Fatalf("panel %q query %q must scope through the real source labels the scrape config attaches", key.Key, q)
		}
	}
	// An application-class source foreign to this app must be excluded.
	foreign := observability.MetricsSource{ID: "app:metrics", Provider: capability.ProviderPostgreSQL, Class: observability.SourceApplication, Scope: capability.ScopeApplication, OwnerApplication: "someone-else"}
	if q := infraQueryFor(infraPanelKeys()[0], []observability.MetricsSource{foreign}, m); strings.Contains(q, "someone-else") {
		t.Fatalf("foreign-owned application source must not scope a panel query, got %q", q)
	}
}

// TestPersesInfraDashboardSchemaMatchesPinnedUpstream witnesses that the
// produced resource shape comes from the pinned upstream sources, not from
// this file's own serialization.
func TestPersesInfraDashboardSchemaMatchesPinnedUpstream(t *testing.T) {
	witnesses := map[string][]string{
		"perses-dashboard-model.go.txt": {
			`Kind     Kind`,
			`Metadata ProjectMetadata`,
			`Spec     dashboardSpec.Spec`,
		},
		"perses-metadata-model.go.txt": {
			"type Metadata struct",
			"Name string",
			"type ProjectMetadataWrapper struct",
			"Project string",
		},
		"spec-dashboard-model.go.txt": {
			"Panels      map[string]*Panel",
			"Layouts     []Layout",
			`json:"duration"`,
		},
		"spec-plugin-model.go.txt": {
			"type Plugin struct",
			"Kind string",
		},
		"perses-prometheus-time-series-query.cue.txt": {
			`kind: "PrometheusTimeSeriesQuery"`,
			"spec: close(",
			"query:             strings.MinRunes(1)",
			"ds.#selector",
		},
		"shared-datasource-selector.cue.txt": {
			"#datasourceSelector: {",
			"kind:  _kind",
		},
		"perses-datasource-model.go.txt": {
			"type Datasource struct",
			"type GlobalDatasource struct",
		},
		"perses-prometheus-datasource-schema.cue.txt": {
			"#kind: \"PrometheusDatasource\"",
			"#selector: common.#datasourceSelector",
		},
		"dev-data-dashboard.json": {
			`"kind": "Dashboard"`,
			`"kind": "Panel"`,
			`"kind": "TimeSeriesQuery"`,
			`"kind": "PrometheusTimeSeriesQuery"`,
			`"kind": "Grid"`,
			`"$ref": "#/spec/panels/`,
		},
	}
	for file, fragments := range witnesses {
		raw, err := os.ReadFile(filepath.Join("testdata", "perses-infra-dashboard", "upstream", file))
		if err != nil {
			t.Fatalf("upstream witness %s missing: %v", file, err)
		}
		// Provenance: the pinned upstream REST models declare NO apiVersion
		// anywhere (w091 witness pattern); a witness that suddenly carries
		// one means the mirror drifted from the pinned release.
		if strings.Contains(string(raw), "apiVersion") {
			t.Fatalf("upstream witness %s unexpectedly mentions apiVersion — re-check the mirror against the pinned release", file)
		}
		for _, frag := range fragments {
			if !strings.Contains(string(raw), frag) {
				t.Fatalf("upstream witness %s does not declare %q — the pinned schema evidence is gone", file, frag)
			}
		}
	}
	// The produced document's own shape must stay inside the witness: the
	// exact kinds the upstream dev-data dashboard carries.
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	res, err := BuildPersesInfraDashboard(m)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := PersesInfraDashboardJSON(res.Dashboard)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, frag := range []string{`"kind": "Dashboard"`, `"kind": "Panel"`, `"kind": "TimeSeriesQuery"`, `"kind": "PrometheusTimeSeriesQuery"`, `"$ref": "#/spec/panels/`} {
		if !strings.Contains(string(raw), frag) {
			t.Fatalf("produced resource must carry the native fragment %q", frag)
		}
	}
	if strings.Contains(string(raw), "apiVersion") {
		t.Fatal("the produced resource must carry no apiVersion field (the pinned model has none)")
	}
}

// TestPersesInfraDashboardFailClosedWhenUnselected pins that an unselected
// Perses surface builds no dashboard (fail-closed).
func TestPersesInfraDashboardFailClosedWhenUnselected(t *testing.T) {
	res, err := BuildPersesInfraDashboard(persesInfraUnselectedManifest())
	if err == nil {
		t.Fatal("unselected Perses surface must fail closed (no dashboard), an unselected surface has nothing to bind")
	}
	var notBuilt *PersesInfraDashboardError
	if !errors.As(err, &notBuilt) {
		t.Fatalf("fail-closed must return a *PersesInfraDashboardError, got %v", err)
	}
	if res.Dashboard.Kind != "" || res.Dashboard.Metadata.Name != "" {
		t.Fatal("a fail-closed unselected surface must not carry a dashboard resource")
	}
}

// TestPersesInfraDashboardFailClosedWhenNoProvider pins that a missing
// Prometheus provider builds no dashboard (fail-closed): the dashboard only
// ever sits on top of a verified datasource, never a fabricated document.
func TestPersesInfraDashboardFailClosedWhenNoProvider(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()

	res, err := BuildPersesInfraDashboard(m)
	if err == nil {
		t.Fatal("missing Prometheus provider must fail closed (no dashboard), a dashboard is never fabricated without a verified datasource")
	}
	var notBuilt *PersesInfraDashboardError
	if !errors.As(err, &notBuilt) {
		t.Fatalf("fail-closed must return a *PersesInfraDashboardError, got %v", err)
	}
	if res.Dashboard.Kind != "" || res.Dashboard.Metadata.Name != "" {
		t.Fatal("a fail-closed missing-provider surface must not carry a dashboard resource")
	}
}

// TestPersesInfraDashboardFailClosedWhenNoAuthorizedSource pins that a
// datasource without a single authorized materialized source fails closed
// BEFORE any panel logic runs: no empty-panel dashboard either way.
func TestPersesInfraDashboardFailClosedWhenNoAuthorizedSource(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.MetricsCollectSourcesEnv, "")
	m := persesPrometheusDatasourceManifest()
	// Materialize the provider but deny every source class: the datasource
	// build itself fails closed (lane-083 contract), so the dashboard never
	// assembles panels over denied sources.
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))
	t.Setenv(application.MetricsCollectSourcesEnv, "application")

	res, err := BuildPersesInfraDashboard(m)
	if err == nil {
		t.Fatal("a datasource with only denied source classes must fail closed")
	}
	var notBuilt *PersesInfraDashboardError
	if !errors.As(err, &notBuilt) {
		t.Fatalf("fail-closed must return a *PersesInfraDashboardError, got %v", err)
	}
	if len(res.Dashboard.Spec.Panels) != 0 {
		t.Fatal("a fail-closed denied-source surface must not ship panels")
	}
}

// TestPersesInfraDashboardNoCapabilityClaim asserts the produced dashboard
// declares NO Metrics capability: the marshalled JSON carries only the
// resource, never an IntegrationDescriptor or a Metrics capability.
func TestPersesInfraDashboardNoCapabilityClaim(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesInfraDashboard(m)
	if err != nil {
		t.Fatalf("selected+pinned datasource must build a dashboard: %v", err)
	}
	raw, err := json.Marshal(res.Dashboard)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "Metrics") || strings.Contains(string(raw), "IntegrationDescriptor") {
		t.Fatalf("perses infrastructure dashboard must not declare a Metrics capability or IntegrationDescriptor: %s", raw)
	}
}

// TestPersesInfraDashboardJSONGolden pins the marshalled dashboard against a
// committed golden file, so the native resource output is a stable, reviewed
// witness. The golden document is the probe-less build: every panel present,
// every panel verdict explicitly missing (the visible-gap default).
func TestPersesInfraDashboardJSONGolden(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	persesMaterializeProvider(t, m, persesProviderSource("tempo:runtime", capability.ProviderTempo))

	res, err := BuildPersesInfraDashboard(m)
	if err != nil {
		t.Fatalf("selected+pinned datasource must build a dashboard: %v", err)
	}
	got, err := PersesInfraDashboardJSON(res.Dashboard)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	goldenPath := filepath.Join("testdata", "perses-infra-dashboard", "prometheus-targets.json")
	if os.Getenv("PERSES_INFRA_GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("golden update: %v", err)
		}
		t.Log("golden file updated (PERSES_INFRA_GOLDEN_UPDATE=1)")
		return
	}
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

// persesInfraUnselectedManifest is a manifest WITHOUT the Perses surface.
func persesInfraUnselectedManifest() application.Manifest {
	return application.New("perses-datasource-demo", "dev", false, false, false)
}

var (
	_ = fmt.Sprintf
	_ = observability.MetricsSource{}
)

// TestPersesInfraDashboardAvailabilityProductEntry proves the availability
// entry point the product status path calls (0428-082; precedent: the 077
// PersesHostDashboardAvailability catalog face) is fail-closed and complete:
// without a materialized Prometheus provider it returns EXPLICIT unavailable
// entries for every panel with the concrete reason (never an empty map with
// nil verdict, never a positive derived from the manifest declaration), and
// an unselected surface exposes no face at all.
func TestPersesInfraDashboardAvailabilityProductEntry(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")

	report := PersesInfraDashboardAvailability(t.Context(), persesInfraUnselectedManifest())
	if len(report) != 0 {
		t.Fatalf("unselected surface must expose no infrastructure dashboards, got %+v", report)
	}

	m := persesPrometheusDatasourceManifest()
	report = PersesInfraDashboardAvailability(t.Context(), m)
	if len(report) != len(infraPanelKeys()) {
		t.Fatalf("selected surface must expose every infra panel, got %d entries", len(report))
	}
	for _, key := range infraPanelKeys() {
		entry, ok := report[key.Key]
		if !ok {
			t.Fatalf("panel %q missing from the availability face", key.Key)
		}
		if entry.Available != PersesInfraDashboardUnavailable || entry.Reason == "" {
			t.Fatalf("without a materialized Prometheus backend panel %q must be unavailable with a reason, got %+v", key.Key, entry)
		}
		if entry.Backend != string(capability.ProviderPrometheus) {
			t.Fatalf("panel %q backend = %q, want the Prometheus data owner", key.Key, entry.Backend)
		}
	}
}

// prometheusSeriesServer answers REAL Prometheus /api/v1/query requests over
// TLS with Prometheus list-syntax filtering over a fixed series inventory: a
// query whose selector matches a series returns its rows, a query whose
// family was never scraped returns the empty vector. Every query that lands
// on the wire is recorded, so the caller can assert the round trips happened.
func prometheusSeriesServer(t *testing.T, series []prometheusSeriesFixture) (*httptest.Server, *[]string) {
	t.Helper()
	seen := []string{}
	var mu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query().Get("query")
		mu.Lock()
		seen = append(seen, query)
		mu.Unlock()
		rows := []map[string]any{}
		for _, candidate := range series {
			if !persesQueryMatchesSeries(query, candidate.Labels) {
				continue
			}
			rows = append(rows, map[string]any{
				"metric": candidate.Labels,
				"value":  []any{1700000000, candidate.Value},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(map[string]any{"status": "success", "data": map[string]any{
			"resultType": "vector", "result": rows,
		}})
		if err != nil {
			t.Errorf("marshal series answer: %v", err)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

// prometheusSeriesFixture is one series the endpoint answers with: its label
// set plus the value the query returns.
type prometheusSeriesFixture struct {
	Labels map[string]string
	Value  string
}

// persesQueryMatchesSeries answers a query the way Prometheus would for the
// fixture inventory: the query is the scoped-selector form the builder emits
// (a bare selector or an or-union of them); a series answers the query when
// one of the query's family{matchers} tokens names the series and every
// matcher of that token matches the series' labels exactly.
func persesQueryMatchesSeries(query string, labels map[string]string) bool {
	for _, selector := range prometheusSelectors(query) {
		family, matcherList, ok := strings.Cut(selector, "{")
		if !ok {
			continue
		}
		if labels["__name__"] != family {
			continue
		}
		matcherList = strings.TrimSuffix(matcherList, "}")
		matches := true
		for _, matcher := range splitPrometheusMatchers(matcherList) {
			key, operator, value := parsePrometheusMatcher(matcher)
			if key == "" {
				matches = false
				break
			}
			got, has := labels[key]
			switch operator {
			case "=":
				if !has || got != value {
					matches = false
				}
			case "!=":
				if has && got == value {
					matches = false
				}
			case "=~":
				if !has {
					matches = false
					break
				}
				re, rerr := regexp.Compile("^(?:" + value + ")$")
				if rerr != nil || !re.MatchString(got) {
					matches = false
				}
			case "!~":
				if has {
					re, rerr := regexp.Compile("^(?:" + value + ")$")
					if rerr != nil || re.MatchString(got) {
						matches = false
					}
				}
			}
			if !matches {
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

// prometheusSelectors extracts every "family{matchers}" selector token of a
// PromQL expression, in order of appearance.
func prometheusSelectors(query string) []string {
	var out []string
	for {
		start := strings.IndexByte(query, '{')
		if start < 0 {
			return out
		}
		end := strings.IndexByte(query[start:], '}')
		if end < 0 {
			return out
		}
		familyStart := start
		for familyStart > 0 {
			c := query[familyStart-1]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == ':' {
				familyStart--
				continue
			}
			break
		}
		out = append(out, query[familyStart:start+end+1])
		query = query[start+end+1:]
	}
}

// splitPrometheusMatchers splits a matcher list on commas outside quotes.
func splitPrometheusMatchers(list string) []string {
	var out []string
	var current strings.Builder
	inQuotes := false
	for _, r := range list {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ',' && !inQuotes:
			out = append(out, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		out = append(out, strings.TrimSpace(current.String()))
	}
	return out
}

// parsePrometheusMatcher splits one matcher into key, operator, value.
func parsePrometheusMatcher(matcher string) (string, string, string) {
	for _, operator := range []string{"!=", "=~", "!~", "="} {
		if idx := strings.Index(matcher, operator); idx > 0 {
			key := strings.TrimSpace(matcher[:idx])
			value := matcher[idx+len(operator):]
			value = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value), `"`), `"`)
			return key, operator, value
		}
	}
	return strings.TrimSpace(matcher), "=", ""
}

// TestPersesInfraDashboardVerifiesOverRealHTTPPath proves the dashboard
// verdicts over REAL /api/v1/query HTTP round trips (Luna batch18 unproof
// (b) at the unit layer): the production per-panel probe —
// httpClientSeriesProbe over the package-shared queryUp — against the
// endpoint recorded in provider state. A client that trusts nothing keeps
// every panel the explicit visible gap (the build still succeeds, never a
// silent 0); the production client shape (CA pin + the "prometheus" Host
// alias, providerHTTPClient/withPrometheusHostHeader) fills every panel from
// the scraped series, with one real round trip per panel on the wire.
func TestPersesInfraDashboardVerifiesOverRealHTTPPath(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := persesPrometheusDatasourceManifest()
	source := persesProviderSource("tempo:runtime", capability.ProviderTempo)
	server, seen := prometheusSeriesServer(t, []prometheusSeriesFixture{
		{Labels: map[string]string{"__name__": "up", "job": "baseharbor-providers", "baseharbor_provider": "tempo", "baseharbor_source": "tempo:runtime"}, Value: "1"},
		{Labels: map[string]string{"__name__": "scrape_samples_post_metric_relabeling", "job": "baseharbor-providers", "baseharbor_provider": "tempo", "baseharbor_source": "tempo:runtime"}, Value: "1"},
		{Labels: map[string]string{"__name__": "scrape_samples_scraped", "job": "baseharbor-providers", "baseharbor_provider": "tempo", "baseharbor_source": "tempo:runtime"}, Value: "1"},
		{Labels: map[string]string{"__name__": "scrape_duration_seconds", "job": "baseharbor-providers", "baseharbor_provider": "tempo", "baseharbor_source": "tempo:runtime"}, Value: "1"},
	})
	files := persesMaterializeProvider(t, m, source)
	writeProviderEnv(t, files, "BASEHARBOR_PROMETHEUS_PORT="+strconv.Itoa(endpointPort(t, server.URL))+"\n")

	// The untrusted-transport witness: the production endpoint is HTTPS over
	// the service-access PKI, so a client that trusts nothing completes no
	// handshake — no request reaches the handler and every panel stays the
	// explicit visible gap, while the build itself still succeeds.
	res, err := BuildPersesInfraDashboardWithProbe(m, httpClientSeriesProbe{client: &http.Client{}})
	if err != nil {
		t.Fatalf("a transport failure must keep panels as visible gaps, not fail the build: %v", err)
	}
	for _, signal := range res.Signals {
		if !signal.MissingSignal {
			t.Fatalf("with an untrusted transport every panel must stay a gap, got %+v", signal)
		}
	}

	// The trusted round trip through the production client shape: server.Client
	// stands in for the CA-pinning client providerHTTPClient builds from the
	// service-access PKI in production (the httptest certificate covers
	// 127.0.0.1). One real /api/v1/query per panel lands on the wire; the
	// scraped series answer their panels; the panel verdicts equal the
	// queries on the wire.
	before := len(*seen)
	res, err = BuildPersesInfraDashboardWithProbe(m, httpClientSeriesProbe{client: server.Client()})
	if err != nil {
		t.Fatalf("the production client shape must build over the real round trips: %v", err)
	}
	asked := (*seen)[before:]
	if len(asked) != len(infraPanelKeys()) {
		t.Fatalf("the trusted round trip must add one real /api/v1/query per panel, got %d: %v", len(asked), asked)
	}
	for i, key := range infraPanelKeys() {
		if asked[i] != res.Signals[i].Query {
			t.Fatalf("panel %q wire query %q does not match its recorded query %q", key.Key, asked[i], res.Signals[i].Query)
		}
	}
	for _, signal := range res.Signals {
		if signal.MissingSignal {
			t.Fatalf("every scraped series must answer its own panel over the real round trip: %+v", signal)
		}
	}

	// The production availability face over the same materialized state (no
	// service-access PKI in a unit fixture) fails closed into explicit
	// unavailable entries naming the concrete reason — never a fabricated
	// availability. The cell witness round-5.md proves the same face against
	// a deploy-path-materialized Prometheus with the real PKI.
	entries := PersesInfraDashboardAvailability(t.Context(), m)
	if len(entries) != len(infraPanelKeys()) {
		t.Fatalf("availability face must cover every panel, got %d", len(entries))
	}
	for _, entry := range entries {
		if entry.Available != PersesInfraDashboardUnavailable || entry.Reason == "" {
			t.Fatalf("without service-access PKI every panel must be unavailable with a reason, got %+v", entry)
		}
	}
}
