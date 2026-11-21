package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// Perses infrastructure dashboard layer (0428-082).
//
// This file realizes the native Perses INFRASTRUCTURE DASHBOARD that
// visualises the real BaseHarbor Prometheus backend. Perses is a management/
// visualization surface, never an application capability: this dashboard only
// binds the existing, Prometheus-owned metrics endpoint to a native Perses
// dashboard resource. It declares NO Metrics capability of its own (Perses
// only visualises the backend data owner that already ships the real metrics
// signal) and names the real owner (Prometheus) as an owner reference, never
// as a Perses-declared capability.
//
// Native schema, pinned at the release Core pins (v0.55.1). The produced
// document is the Perses v1 Dashboard RESOURCE: kind "Dashboard" + metadata
// (name/project, per perses/perses pkg/model/api/v1/metadata.go
// ProjectMetadata — the createdAt/updatedAt/version stamps are assigned by
// the Perses API at write time) + spec (display/duration/panels/layouts, per
// perses/spec go/dashboard dashboard.go Spec; the inline datasources map is
// deprecated upstream and unused here). The resource kind carries NO
// apiVersion field — the pinned upstream model declares none, and this file
// never invents one. Panels are native Panel documents (display/plugin/
// queries); the query wrapper kind is "TimeSeriesQuery" around the plugin
// "PrometheusTimeSeriesQuery" whose spec carries the datasource static
// selector ({"kind","name"}, pinned via the go-sdk selector witness) and the
// exact PromQL. Layouts are native Grid
// layouts whose items reference the panels through {"$ref":
// "#/spec/panels/<key>"}, exactly the form the pinned release's own dev/data
// dashboard documents use. These kinds and field shapes are pinned by the
// committed upstream witnesses under testdata/perses-infra-dashboard/
// upstream/ (mirrored with their pinned-commit URLs), and the golden file
// pins the whole produced resource.
//
// Fail-closed: the dashboard layer never fabricates a dashboard resource for a
// missing, unselected, unverified or externally-owned Prometheus path. Its
// builder requires a pinned release that validates, a selected Perses surface,
// a verified Prometheus datasource (the lane-083 adapter, consumed read-only)
// and real authorized materialized sources; a missing datasource, an
// unselected or unpinned surface all propagate as errors, never as a dashboard
// with empty panels and a nil error. Pin validation alone must never map into
// a "filled" dashboard.
//
// Missing-signal contract (0428-082; supersedes the round-1 generic-probe
// shortcut). An absent infra signal is an EXPLICIT, VISIBLE gap PER PANEL,
// derived from the metric the panel itself consumes — never from a generic
// up{} reachability probe. The mechanism is two-fold and schema-supported:
//
//   - The Go-side verdict: the builder answers each panel's OWN series query
//     (one /api/v1/query per panel, through the persesInfraProbe seam and the
//     package's shared queryUp path over the materialized management
//     endpoint). The plugins render an empty result as the native "No Data"
//     state (the behavior CHANGELOG.md pins: "TimeSeriesChart: Empty query
//     results should show \"No Data\"…", "Show no data when query result is
//     empty", "Consistent No Data style for plugins"): the panel keeps its
//     title, scope and query, so the gap is VISIBLE per panel, never a
//     silent 0. A query that returns no rows makes exactly that panel
//     MissingSignal=true (recorded on the panel —
//     PersesInfraPanel.MissingSignal, not serialized into the resource — and
//     in the result's signal inventory); a generic up{} verdict can never
//     fill or blank any panel.
//
// What BaseHarbor actually scrapes is the authority for panel series (Luna
// batch17 finding 3). The provider-scoped prometheus.yml (
// internal/metrics/prometheus_render.go) scrapes ONLY BaseHarbor endpoints
// under the jobs baseharbor-applications / baseharbor-providers, labeled
// baseharbor_application / baseharbor_provider / baseharbor_source. The Core
// provisions NO node_exporter target anywhere in the tree, so this dashboard
// ships NO node_* series and NO host-CPU/memory/disk/network pretence: the
// standard node_exporter name for the CPU signal is node_cpu_seconds_total,
// and the round-1 unprefixed cpu_seconds_total{job="prometheus"} was a
// fabrication in both directions (wrong name, wrong job). The four infra
// panels realize the four standard axes on the signal family THIS
// deployment's Prometheus backend genuinely maintains for every authorized
// source — the target-up and per-scrape series (up,
// scrape_samples_post_metric_relabeling, scrape_samples_scraped,
// scrape_duration_seconds) — scoped by the real job and the real source
// labels, so a panel series exists exactly when a real source backs it: the
// same vocabulary the lane-083 datasource health contract probes.
//
// persesInfraProbe is the injectable per-panel series path: one /api/v1/query
// per panel series, over the materialized management endpoint. Production
// uses the service-access client via the shared queryUp; tests script exact
// per-series answers, so a panel's state always travels through the panel's
// own query (Luna batch17 finding 2: never one generic up{} fake for all
// panels).
type persesInfraProbe interface {
	querySeries(ctx context.Context, endpoint, query string) (bool, error)
}

// unavailablePersesInfraProbe performs no live observation: every series
// answers "no rows", so a probe-less build keeps every panel as the explicit
// visible gap — unobserved is never optimistically filled.
type unavailablePersesInfraProbe struct{}

func (unavailablePersesInfraProbe) querySeries(context.Context, string, string) (bool, error) {
	return false, nil
}

// httpClientSeriesProbe answers panel series over the package's shared queryUp
// with the management TLS client (the same client the datasource verify lane
// uses). Production wiring passes it through the verify entry point below; it
// is referenced there, never dead.
type httpClientSeriesProbe struct {
	client *http.Client
}

func (p httpClientSeriesProbe) querySeries(ctx context.Context, endpoint, query string) (bool, error) {
	return queryUp(ctx, p.client, endpoint, query)
}

// VerifyPersesInfraDashboard runs the dashboard build against the REAL
// per-panel series queries over the materialized management endpoint (the
// provider service-access client, same path the datasource verify lane
// uses). Every panel verdict therefore comes from the panel's own query
// result against the live backend; unmaterialized endpoints and denied
// sources fail closed exactly like the build path.
func VerifyPersesInfraDashboard(ctx context.Context, m application.Manifest) (PersesInfraDashboardResult, error) {
	files, _, err := persesPrometheusDatasourceEndpoint(m)
	if err != nil {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: err.Error()}
	}
	client, err := providerHTTPClient(m, files)
	if err != nil {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: err.Error()}
	}
	res, err := BuildPersesInfraDashboardWithProbe(m, httpClientSeriesProbe{client: client})
	if err != nil {
		return PersesInfraDashboardResult{}, err
	}
	// Fail closed on a fully-missing live verdict the same way the datasource
	// health contract does: if NO panel's own series answered, the verified
	// path reports the error, not an all-empty success.
	if res.MissingSignalCount() == len(res.Signals) {
		return res, &PersesInfraDashboardError{Detail: "no infrastructure panel series answered on the live Prometheus backend (all visible gaps)"}
	}
	return res, nil
}

// Signal-source bridge (production evidence): every panel query family is
// one Prometheus WRITES for a scraped target, not one its monitored process
// must expose. up{...} is Prometheus's own per-target readiness series (the
// same vocabulary persesPrometheusDatasourceQuery probes), and
// scrape_samples_post_metric_relabeling / scrape_samples_scraped /
// scrape_duration_seconds are the per-scrape metadata series the scrape loop
// itself emits. A panel is therefore backed exactly when a real authorized
// source is materialized and the query answers rows — never when a job
// happens to expose private process_* metrics.

// Perses infrastructure dashboard availability. Precedent: the dashboard
// catalog lines surface availability on the public `baha status --json` face
// (0428-077 `host_containers_dashboards`, cmd/baha/app_status_result.go; the
// 0428-076 `database_dashboards` face). The verdicts are EXACTLY
// "available" | "unavailable": a positive NEVER comes from a caller-supplied
// boolean (owner decision 2026-11-16: "Keine Verfuegbarkeit aus einem
// caller-supplied Status ableiten"). It comes only from the REAL per-panel
// series queries this entry point runs against the materialized Prometheus
// endpoint. This is the product call site of the dashboard builder/verifier:
// cmd/baha/app_status_result.go computes it on every status face.
const (
	PersesInfraDashboardAvailable   = "available"
	PersesInfraDashboardUnavailable = "unavailable"
)

// PersesInfraDashboardAvailabilityEntry is one panel's availability verdict on
// the CLI status JSON: the exact query the panel consumes (the real
// /api/v1/query it was answered by, or would be answered by) plus the verdict
// and, for a gap, the concrete reason.
type PersesInfraDashboardAvailabilityEntry struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Query      string `json:"query,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// PersesInfraDashboardAvailability runs the dashboard's live verification
// against the materialized Prometheus endpoint (one real /api/v1/query per
// panel through the service-access client, VerifyPersesInfraDashboard) and
// maps its fail-closed result to the per-panel availability face. Any error —
// unpinned release, unselected surface, missing/denied datasource, an
// endpoint that answers nothing — is an EXPLICIT unavailable entry with the
// concrete reason on every panel, never a silent omission and never a
// fabricated availability. An unselected surface exposes NO face at all
// (omitted key), matching the manifest gate of the three-state contract.
// With m.Services.Perses already selected, the remaining builder gates
// (pinned release, authorized metrics policy, materialized provider and
// sources) are properties of the deployment STATE, not of the caller: this
// entry point derives the verdict ONLY from the verified build result — never
// from a caller-supplied status boolean.
func PersesInfraDashboardAvailability(ctx context.Context, m application.Manifest) map[string]PersesInfraDashboardAvailabilityEntry {
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return nil
	}
	out := make(map[string]PersesInfraDashboardAvailabilityEntry, len(infraPanelKeys()))
	res, err := VerifyPersesInfraDashboard(ctx, m)
	if err != nil {
		reason := err.Error()
		for _, key := range infraPanelKeys() {
			out[key.Key] = PersesInfraDashboardAvailabilityEntry{
				Key: key.Key, Backend: string(capability.ProviderPrometheus),
				Available: PersesInfraDashboardUnavailable, Reason: reason,
			}
		}
		return out
	}
	for _, signal := range res.Signals {
		entry := PersesInfraDashboardAvailabilityEntry{
			Key: signal.Panel, Backend: string(capability.ProviderPrometheus),
			Available: PersesInfraDashboardAvailable, Query: signal.Query,
			Datasource: PersesInfraDatasourceName,
		}
		if signal.MissingSignal {
			entry.Available = PersesInfraDashboardUnavailable
			entry.Reason = "the panel's own series query answered no rows on the materialized Prometheus backend (explicit per-panel gap, never a silent 0)"
		}
		out[entry.Key] = entry
	}
	return out
}

// PersesInfraDashboardJSONFor builds the dashboard (no live observation: every
// panel keeps its explicit gap) and marshals the native Perses Dashboard
// resource deterministically — the golden-witness pattern of this package.
func PersesInfraDashboardJSONFor(m application.Manifest) ([]byte, error) {
	res, err := BuildPersesInfraDashboard(m)
	if err != nil {
		return nil, err
	}
	return PersesInfraDashboardJSON(res.Dashboard)
}

// Perses infrastructure dashboard identity.
const (
	// PersesInfraDashboardName is the Perses dashboard resource name and the
	// stable catalog key the selection catalog (perses_selection.go) keys the
	// prometheus backend dashboard on.
	PersesInfraDashboardName = "prometheus-targets"
	// PersesInfraProject is the Perses project the infrastructure dashboard
	// resource lives in, the same project the pinned management surface uses.
	PersesInfraProject = "baseharbor"
	// PersesInfraResourceKind is the pinned native Perses resource kind. It is
	// the upstream constant ("Dashboard"), never a lowercase private kind.
	PersesInfraResourceKind = "Dashboard"
	// PersesInfraPanelKind is the pinned native Perses panel document kind.
	PersesInfraPanelKind = "Panel"
	// PersesInfraPanelPluginKind is the pinned native stat panel plugin kind.
	PersesInfraPanelPluginKind = "StatChart"
	// PersesInfraQueryWrapperKind is the pinned native query-wrapper kind.
	PersesInfraQueryWrapperKind = "TimeSeriesQuery"
	// PersesInfraQueryPluginKind is the pinned native Prometheus query plugin
	// kind, the same kind the datasource health path uses.
	PersesInfraQueryPluginKind = "PrometheusTimeSeriesQuery"
	// PersesInfraDatasourceSelectorKind is the kind field of the inline
	// static datasource selector object ("Datasource", the _kind the cue
	// selector constraint and the go-sdk StaticSelector carry).
	PersesInfraDatasourceSelectorKind = "Datasource"
	// PersesInfraLayoutKind is the pinned native grid layout kind.
	PersesInfraLayoutKind = "Grid"
	// PersesInfraDatasourceName is the selector name the infra panels query.
	// It matches the lane-083 Prometheus datasource selector literal, never an
	// invented datasource.
	PersesInfraDatasourceName = "prometheus"
)

// PersesInfraDashboardError is returned by the fail-closed dashboard builder
// when a native Perses infrastructure dashboard cannot be produced. It names
// the concrete reason so callers and tests can assert the exact fail-closed
// verdict rather than a fabricated dashboard.
type PersesInfraDashboardError struct {
	Detail string
}

func (e *PersesInfraDashboardError) Error() string {
	return fmt.Sprintf("Perses infrastructure dashboard cannot be built: %s — the Perses infrastructure dashboard requires a selected, pinned, verified Prometheus datasource with authorized materialized sources, so this fails closed rather than fabricating a dashboard document", e.Detail)
}

// PersesInfraJSONRef is the native JSON reference the pinned release's own
// dev/data layout items use ({"$ref": "#/spec/panels/<key>"}).
type PersesInfraJSONRef struct {
	Ref string `json:"$ref"`
}

// PersesInfraDisplay is the native display block (common.Display).
type PersesInfraDisplay struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// PersesInfraPlugin is the native {kind, spec} plugin envelope (
// perses/spec go/plugin Plugin); the spec is raw JSON so the envelope shape
// stays exactly the upstream one.
type PersesInfraPlugin struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec"`
}

// PersesInfraDatasourceSelector is the native inline static datasource
// selector of a PrometheusTimeSeriesQuery spec, {"kind": "Datasource",
// "name": ...}: the cue selector form (perses/shared cue/common/datasource.cue
// "#datasourceSelector: {kind: _kind, name?}" — the _kind the Prometheus
// plugin instantiates is "Datasource", mirrored at
// testdata/perses-infra-dashboard/upstream/shared-datasource-selector.cue.txt
// and prometheus/schemas/prometheus-time-series-query/query.cue). The
// "$variable" string form is the other serialized variant; BaseHarbor
// selects the installed concrete datasource, so it ships the object form. It
// points at the installed lane-083 Prometheus datasource, never an invented
// one.
type PersesInfraDatasourceSelector struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PersesInfraPrometheusQuery is the native PrometheusTimeSeriesQuery plugin
// spec. The pinned schema (perses/plugins
// prometheus/schemas/prometheus-time-series-query/query.cue, mirrored at
// testdata/perses-infra-dashboard/upstream/perses-prometheus-time-series-query.cue.txt)
// closes the spec to: the datasource selector, the query, and the optional
// rendering fields. BaseHarbor ships only selector + query; the schema is
// closed, so no private field (noData, noSeries, status) can exist here.
type PersesInfraPrometheusQuery struct {
	// Datasource is the pinned native selector form (perses/shared
	// cue/common/datasource.cue): an object {kind, name}.
	Datasource *PersesInfraDatasourceSelector `json:"datasource,omitempty"`
	Query      string                         `json:"query"`
}

// PersesInfraQuerySpec is the spec section of a query wrapper.
type PersesInfraQuerySpec struct {
	Plugin PersesInfraPlugin `json:"plugin"`
}

// PersesInfraQuery is the native query wrapper (kind TimeSeriesQuery).
type PersesInfraQuery struct {
	Kind string               `json:"kind"`
	Spec PersesInfraQuerySpec `json:"spec"`
}

// PersesInfraPanelSpec is the native PanelSpec (display + plugin + queries).
type PersesInfraPanelSpec struct {
	Display *PersesInfraDisplay `json:"display,omitempty"`
	Plugin  PersesInfraPlugin   `json:"plugin"`
	Queries []PersesInfraQuery  `json:"queries,omitempty"`
}

// PersesInfraPanel is one native Perses Panel document.
type PersesInfraPanel struct {
	Kind string               `json:"kind"`
	Spec PersesInfraPanelSpec `json:"spec"`
	// MissingSignal is the Go-side per-panel verdict derived from THIS
	// panel's own series evidence (never from a generic up{} probe): true
	// when the panel's backing series has no real signal. It is NOT
	// serialized into the Perses resource — the serialized gap behavior is
	// the pinned plugin's native "No Data" rendering of an empty result.
	MissingSignal bool `json:"-"`
}

// PersesInfraLayoutItem is one grid item referencing a panel by JSON ref.
type PersesInfraLayoutItem struct {
	X       int64              `json:"x"`
	Y       int64              `json:"y"`
	Width   int64              `json:"width"`
	Height  int64              `json:"height"`
	Content PersesInfraJSONRef `json:"content"`
}

// PersesInfraGridLayoutSpec is the native grid layout spec.
type PersesInfraGridLayoutSpec struct {
	Display *PersesInfraDisplay     `json:"display,omitempty"`
	Items   []PersesInfraLayoutItem `json:"items"`
}

// PersesInfraLayout is the native layout document (kind Grid).
type PersesInfraLayout struct {
	Kind string                    `json:"kind"`
	Spec PersesInfraGridLayoutSpec `json:"spec"`
}

// PersesInfraMetadata is the native Perses project-scoped resource metadata
// (pkg/model/api/v1/metadata.go: Metadata {name, createdAt, updatedAt,
// version} + ProjectMetadataWrapper {project}). createdAt/updatedAt/version
// are assigned by the Perses API at write time, so the deterministic document
// producer ships name + project only.
type PersesInfraMetadata struct {
	Name    string `json:"name"`
	Project string `json:"project"`
}

// PersesInfraDashboardSpec is the native dashboard spec block.
type PersesInfraDashboardSpec struct {
	Display  *PersesInfraDisplay         `json:"display,omitempty"`
	Duration string                      `json:"duration"`
	Panels   map[string]PersesInfraPanel `json:"panels"`
	Layouts  []PersesInfraLayout         `json:"layouts"`
}

// PersesInfraDashboard is the native Perses v1 Dashboard RESOURCE produced by
// this file. The kind is the pinned resource kind constant; there is no
// apiVersion field because the pinned upstream model has none.
type PersesInfraDashboard struct {
	Kind     string                   `json:"kind"`
	Metadata PersesInfraMetadata      `json:"metadata"`
	Spec     PersesInfraDashboardSpec `json:"spec"`
	// Owner is the real observability capability owner Perses visualises. It
	// is the Prometheus provider kind, named as an owner reference, NOT a
	// Perses-declared capability. It is not serialized.
	Owner string `json:"-"`
}

// infraPanelKey is one infra panel: its catalog key, display title and the
// signal axis it visualises (for tests and docs, not serialized). The titles
// name the REAL backend-side signal, not a host axis Core does not scrape.
type infraPanelKey struct {
	// Key is the native panel name (the spec.panels map key).
	Key string
	// Title is the native panel display name.
	Title string
	// KindAxis is the signal axis this panel visualises (for tests and docs,
	// not serialized).
	Axis string
}

// infraPanelKeys is the deterministic panel inventory. Every family is one
// BaseHarbor's own scrape config provably attaches to a materialized,
// authorized source (see the file header); NO node_exporter family appears.
func infraPanelKeys() []infraPanelKey {
	return []infraPanelKey{
		{Key: "cpu", Title: "Target health load", Axis: "cpu"},
		{Key: "memory", Title: "Samples after relabeling", Axis: "memory"},
		{Key: "disk", Title: "Samples stored", Axis: "disk"},
		{Key: "network", Title: "Scrape latency", Axis: "network"},
	}
}

// infraQueryFor is the exact PromQL a panel consumes, built from the REAL
// materialized sources and the labels the scrape relabel rules actually
// attach. The families are the ones Prometheus ITSELF maintains for every
// scraped target — up and the per-scrape series (scrape_samples_post_metric_relabeling,
// scrape_samples_scraped, scrape_duration_seconds) — i.e. the storage-side
// axes CPU, memory, disk and network of THIS deployment's monitoring
// backend. They exist exactly when a source is materialized and authorized
// (the same vocabulary persesPrometheusDatasourceQuery probes); the Core
// provisions no node_exporter, so NO node_* family appears. An empty return
// means no authorized source backs the panel at all.
func infraQueryFor(key infraPanelKey, sources []observability.MetricsSource, m application.Manifest) string {
	var clauses []string
	for _, source := range sources {
		if source.Class == observability.SourceApplication && source.OwnerApplication != m.Name {
			continue
		}
		scope := infraScopeLabel(source, m)
		if scope == "" {
			continue
		}
		switch key.Axis {
		case "cpu":
			clauses = append(clauses, fmt.Sprintf(`max(up{%s})`, scope))
		case "memory":
			clauses = append(clauses, fmt.Sprintf(`scrape_samples_post_metric_relabeling{%s}`, scope))
		case "disk":
			clauses = append(clauses, fmt.Sprintf(`scrape_samples_scraped{%s}`, scope))
		case "network":
			clauses = append(clauses, fmt.Sprintf(`scrape_duration_seconds{%s}`, scope))
		}
	}
	if len(clauses) == 0 {
		return ""
	}
	if len(clauses) == 1 {
		return clauses[0]
	}
	return "max(" + strings.Join(clauses, " or ") + ")"
}

// infraScopeLabel renders the label selector of one materialized source
// exactly as the scrape config attaches it: applications carry
// baseharbor_application, providers carry baseharbor_provider, and every
// source carries baseharbor_source. The job is the real job name.
func infraScopeLabel(source observability.MetricsSource, m application.Manifest) string {
	switch source.Class {
	case observability.SourceApplication:
		return fmt.Sprintf(`job="baseharbor-applications",baseharbor_application=%q,baseharbor_source=%q`, m.Name, source.ID)
	case observability.SourceApplicationProvider, observability.SourcePlatformProvider:
		return fmt.Sprintf(`job="baseharbor-providers",baseharbor_provider=%q,baseharbor_source=%q`, string(source.Provider), source.ID)
	}
	return ""
}

// persesInfraPanel builds one native Panel document for an infra key.
func persesInfraPanel(key infraPanelKey, query string, datasourceName string) PersesInfraPanel {
	querySpec, err := json.Marshal(PersesInfraPrometheusQuery{
		Query:      query,
		Datasource: &PersesInfraDatasourceSelector{Kind: PersesInfraDatasourceSelectorKind, Name: datasourceName},
	})
	if err != nil { // json.Marshal of these value types cannot fail.
		panic("perses infra panel query spec: " + err.Error())
	}
	statSpec, err := json.Marshal(map[string]string{"calculation": "last-value"})
	if err != nil {
		panic("perses infra panel plugin spec: " + err.Error())
	}
	var p PersesInfraPanel
	p.Kind = PersesInfraPanelKind
	p.Spec.Display = &PersesInfraDisplay{Name: key.Title}
	p.Spec.Plugin = PersesInfraPlugin{Kind: PersesInfraPanelPluginKind, Spec: statSpec}
	p.Spec.Queries = []PersesInfraQuery{{
		Kind: PersesInfraQueryWrapperKind,
		Spec: PersesInfraQuerySpec{Plugin: PersesInfraPlugin{Kind: PersesInfraQueryPluginKind, Spec: querySpec}},
	}}
	return p
}

// buildPersesInfraDashboardResource assembles the native resource (panels
// addressed by key in spec.panels, referenced from one Grid layout through
// $ref items, in deterministic order).
func buildPersesInfraDashboardResource(datasourceName string, queries map[string]string) PersesInfraDashboard {
	d := PersesInfraDashboard{
		Kind:     PersesInfraResourceKind,
		Metadata: PersesInfraMetadata{Name: PersesInfraDashboardName, Project: PersesInfraProject},
		Owner:    string(capability.ProviderPrometheus),
		Spec: PersesInfraDashboardSpec{
			Display:  &PersesInfraDisplay{Name: "BaseHarbor infrastructure"},
			Duration: "15m",
			Panels:   map[string]PersesInfraPanel{},
		},
	}
	layout := PersesInfraLayout{Kind: PersesInfraLayoutKind}
	layout.Spec.Display = &PersesInfraDisplay{Name: "Infrastructure signals"}
	for i, key := range infraPanelKeys() {
		d.Spec.Panels[key.Key] = persesInfraPanel(key, queries[key.Key], datasourceName)
		item := PersesInfraLayoutItem{X: int64((i % 2) * 12), Y: int64(i / 2 * 8), Width: 12, Height: 8}
		item.Content = PersesInfraJSONRef{Ref: "#/spec/panels/" + key.Key}
		layout.Spec.Items = append(layout.Spec.Items, item)
	}
	d.Spec.Layouts = []PersesInfraLayout{layout}
	return d
}

// PersesInfraPanelSignal is the per-panel evidence recorded on the result:
// the panel key, the exact query the panel consumes and the panel's own
// missing-signal verdict (never a generic probe answer).
type PersesInfraPanelSignal struct {
	Panel         string
	Query         string
	MissingSignal bool
}

// PersesInfraDashboardResult is the builder's fail-closed verdict for the
// Prometheus infrastructure dashboard.
type PersesInfraDashboardResult struct {
	// Dashboard is the produced native Perses Dashboard resource.
	Dashboard PersesInfraDashboard
	// Signals is the per-panel signal inventory, in deterministic panel
	// order.
	Signals []PersesInfraPanelSignal
}

// MissingSignalCount is the number of panels whose own series query answered
// no rows (explicit, visible per-panel gaps; never silent zeros).
func (r PersesInfraDashboardResult) MissingSignalCount() int {
	n := 0
	for _, s := range r.Signals {
		if s.MissingSignal {
			n++
		}
	}
	return n
}

// buildInfraWithProbe is the shared implementation: the fail-closed gates,
// the verified datasource (read-only), the authorized materialized sources,
// one per-panel series query through probe, and the native resource assembly.
func buildInfraWithProbe(m application.Manifest, probe persesInfraProbe) (PersesInfraDashboardResult, error) {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: err.Error()}
	}
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: "perses infrastructure dashboard requires the observability management UI and Perses selected in the manifest"}
	}
	// Fail-closed: the dashboard only ever sits on top of a verified
	// Prometheus datasource. A missing, denied, invalid or unusable
	// datasource propagates as an error, never as a dashboard with empty
	// panels and a nil error.
	datasourceResult, err := BuildPersesPrometheusDatasource(m)
	if err != nil {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: err.Error()}
	}
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: "perses infrastructure dashboard metrics authorization: " + err.Error()}
	}
	files, endpoint, err := persesPrometheusDatasourceEndpoint(m)
	if err != nil {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: err.Error()}
	}
	sources, err := persesPrometheusDatasourceSources(files, m, policy)
	if err != nil {
		return PersesInfraDashboardResult{}, &PersesInfraDashboardError{Detail: err.Error()}
	}
	queries := map[string]string{}
	for _, key := range infraPanelKeys() {
		queries[key.Key] = infraQueryFor(key, sources, m)
	}
	dashboard := buildPersesInfraDashboardResource(datasourceResult.Datasource.Name, queries)
	signals := make([]PersesInfraPanelSignal, 0, len(infraPanelKeys()))
	for _, key := range infraPanelKeys() {
		query := queries[key.Key]
		panel := dashboard.Spec.Panels[key.Key]
		seriesPresent := false
		if query != "" {
			// The panel's OWN series query decides. A transport error keeps
			// the panel as the visible gap (fail toward showing missing,
			// never toward a silent filled panel).
			present, err := probe.querySeries(context.Background(), endpoint, query)
			seriesPresent = err == nil && present
		}
		panel.MissingSignal = !seriesPresent
		dashboard.Spec.Panels[key.Key] = panel
		signals = append(signals, PersesInfraPanelSignal{Panel: key.Key, Query: query, MissingSignal: !seriesPresent})
	}
	return PersesInfraDashboardResult{Dashboard: dashboard, Signals: signals}, nil
}

// BuildPersesInfraDashboard builds the native Perses infrastructure dashboard
// for the selected and pinned surface. It is fail-closed (unpinned release,
// unselected surface, missing/invalid/unauthorized datasource => error, never
// a fabricated document). It performs no live series observation, so every
// panel is explicitly MISSING (a visible gap): an unobserved panel is never
// optimistically filled.
func BuildPersesInfraDashboard(m application.Manifest) (PersesInfraDashboardResult, error) {
	return buildInfraWithProbe(m, unavailablePersesInfraProbe{})
}

// BuildPersesInfraDashboardWithProbe builds the dashboard and answers every
// panel's missing-signal state from THAT panel's own series query (one
// /api/v1/query per panel over the materialized management endpoint, through
// the shared queryUp path). Tests script the same seam per series, so a
// filled/missing panel state always travels the panel's own query path —
// never a generic up{} verdict.
func BuildPersesInfraDashboardWithProbe(m application.Manifest, probe persesInfraProbe) (PersesInfraDashboardResult, error) {
	if probe == nil {
		probe = unavailablePersesInfraProbe{}
	}
	return buildInfraWithProbe(m, probe)
}

// PersesInfraDashboardJSON marshals the produced dashboard deterministically
// (json.MarshalIndent with trailing newline), mirroring the golden-file
// witness pattern used across the metrics package and the datasource adapter.
func PersesInfraDashboardJSON(d PersesInfraDashboard) ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
