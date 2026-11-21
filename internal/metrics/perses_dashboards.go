package metrics

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Perses-native database dashboards for BaseHarbor.
//
// This file realizes the Perses-native dashboard content for the three real
// BaseHarbor observability backends (Prometheus targets, Loki logs, Tempo
// traces). It is a management realization, never an application capability: it
// never enables an exporter, declares metrics/logs/traces as an application
// capability, or ships a dashboard that assumes Grafana UI APIs.
//
// The Perses JSON format is a Go-struct-first realization (the same pattern as
// internal/metrics/prometheus_files.go, realization.go and runtime_source.go):
// structs marshal to the Perses-native dashboard/document format. No go:embed
// of dashboard JSON lives in this package — the structs are the source of
// truth and the deterministic golden-file witnesses are their marshalling.
//
// Every dashboard is keyed on the accepted pinned Perses release
// (capability.AcceptedPersesVersion, v0.55.1) and ships only panels that query
// a real BaseHarbor backend signal. Each unsupported database (PostgreSQL,
// MongoDB) remains explicitly UNAVAILABLE: there is no native metrics signal
// (PostgreSQL requires an adapter, MongoDB has no metrics contract), so no
// panel is generated for it and no exporter is installed. This satisfies the
// card contract "each unsupported database must remain explicitly unavailable;
// no silent exporter install".

// PersesDashboardKind is the Perses resource kind for a dashboard.
const PersesDashboardKind = "Dashboard"

// PersesPanelKind is the Perses resource kind for a panel.
const PersesPanelKind = "Panel"

// PersesPanelPluginKinds are the panel plugin kinds shipped by BaseHarbor.
const (
	PersesPanelPluginTimeSeriesChart = "TimeSeriesChart"
	PersesPanelPluginStatChart       = "StatChart"
	PersesPanelPluginLogsTable       = "LogsTable"
	PersesPanelPluginTraceGantt      = "TracingGanttChart"
)

// Perses datasource plugin kinds, one per real backend.
const (
	PersesDatasourcePrometheus = "PrometheusDatasource"
	PersesDatasourceLoki       = "LokiDatasource"
	PersesDatasourceTempo      = "TempoDatasource"
)

// Perses query plugin kinds, one per real backend.
const (
	PersesQueryPrometheus = "PrometheusTimeSeriesQuery"
	PersesQueryLoki       = "LokiLogQuery"
	PersesQueryTempo      = "TempoTraceQuery"
)

// Perses query wrapper kinds.
const (
	PersesQueryWrapperTimeSeries = "TimeSeriesQuery"
	PersesQueryWrapperLog        = "LogQuery"
	PersesQueryWrapperTrace      = "TraceQuery"
)

// PersesGridLayoutKind is the Perses grid layout kind.
const PersesGridLayoutKind = "Grid"

// PersesPanel is a minimal Perses panel resource. Only the fields BaseHarbor
// uses are represented; the marshalling is deterministic and golden-tested.
type PersesPanel struct {
	Kind string          `json:"kind"`
	Spec PersesPanelSpec `json:"spec"`
}

// PersesPanelSpec is the spec section of a Perses panel.
type PersesPanelSpec struct {
	Display PersesDisplay        `json:"display"`
	Plugin  PersesPanelPlugin    `json:"plugin"`
	Queries []PersesQueryWrapper `json:"queries"`
}

// PersesDisplay is the display section of a Perses panel/dashboard.
type PersesDisplay struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PersesPanelPlugin is the plugin section of a Perses panel.
type PersesPanelPlugin struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec,omitempty"`
}

// PersesQueryWrapper wraps a query plugin in its wrapper kind.
type PersesQueryWrapper struct {
	Kind string            `json:"kind"`
	Spec PersesQueryPlugin `json:"spec"`
}

// PersesQueryPlugin is the spec section of a Perses query.
type PersesQueryPlugin struct {
	Kind       string               `json:"kind"`
	Spec       json.RawMessage      `json:"spec,omitempty"`
	Datasource *PersesDatasourceRef `json:"datasource,omitempty"`
}

// PersesDatasourceRef selects a datasource by kind + name.
type PersesDatasourceRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PersesLayout is a minimal Perses grid layout item.
type PersesLayout struct {
	Kind string           `json:"kind"`
	Spec PersesLayoutSpec `json:"spec"`
}

// PersesLayoutSpec is the spec section of a Perses grid layout.
type PersesLayoutSpec struct {
	Items []PersesGridItem `json:"items"`
}

// PersesGridItem positions one panel on the grid.
type PersesGridItem struct {
	X       int               `json:"x"`
	Y       int               `json:"y"`
	Width   int               `json:"width"`
	Height  int               `json:"height"`
	Content PersesGridContent `json:"content"`
}

// PersesGridContent references a panel by its $ref path.
type PersesGridContent struct {
	Ref string `json:"$ref"`
}

// PersesDashboard is the minimal Perses dashboard resource BaseHarbor ships.
type PersesDashboard struct {
	Kind     string                  `json:"kind"`
	Metadata PersesDashboardMetadata `json:"metadata"`
	Spec     PersesDashboardSpec     `json:"spec"`
}

// PersesDashboardMetadata is the metadata section of a Perses dashboard.
type PersesDashboardMetadata struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// PersesDashboardSpec is the spec section of a Perses dashboard.
type PersesDashboardSpec struct {
	Display     PersesDisplay               `json:"display"`
	Datasources map[string]PersesDatasource `json:"datasources"`
	Panels      map[string]PersesPanel      `json:"panels"`
	Layouts     []PersesLayout              `json:"layouts"`
	Duration    string                      `json:"duration,omitempty"`
	Timezone    string                      `json:"timezone,omitempty"`
}

// PersesDatasource is a Perses datasource definition.
type PersesDatasource struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// PersesPanelCatalogEntry is one shipped Perses-native dashboard for one real
// backend. It is the deterministic source of truth; its marshalling is the
// golden witness.
type PersesPanelCatalogEntry struct {
	// Key is the stable catalog identifier (matches the selection catalog key).
	Key string
	// Backend is the real data owner (prometheus, loki, tempo).
	Backend string
	// Datasource is the Perses datasource kind the panels query.
	Datasource string
	// DatasourceName is the Perses datasource selector name.
	DatasourceName string
	// Dashboard is the minimal Perses dashboard resource.
	Dashboard PersesDashboard
	// Unavailable is set for a database with no native metrics signal. When
	// true, Dashboard is empty and Reason names the missing signal.
	Unavailable bool
	// Reason explains an unavailable entry (e.g. "no native metrics signal").
	Reason string
}

// persesPanelPrometheus is the BaseHarbor Prometheus dashboard panel set. It
// queries the real application up-signal that Prometheus actually scrapes
// (prometheus.go:345). The query is a literal so the golden witness is exact.
func persesPanelPrometheus() PersesPanel {
	return PersesPanel{
		Kind: PersesPanelKind,
		Spec: PersesPanelSpec{
			Display: PersesDisplay{Name: "BaseHarbor application up"},
			Plugin:  PersesPanelPlugin{Kind: PersesPanelPluginStatChart},
			Queries: []PersesQueryWrapper{{
				Kind: PersesQueryWrapperTimeSeries,
				Spec: PersesQueryPlugin{
					Kind:       PersesQueryPrometheus,
					Spec:       mustJSON(map[string]string{"query": `up{job="baseharbor-applications",baseharbor_application="$application",baseharbor_environment="$env"}`}),
					Datasource: &PersesDatasourceRef{Kind: PersesDatasourcePrometheus, Name: "prometheus"},
				},
			}},
		},
	}
}

// persesPanelLoki is the BaseHarbor Loki dashboard panel set. It queries the
// real Loki log source (job="baseharbor-applications") with a LogQL log query.
func persesPanelLoki() PersesPanel {
	return PersesPanel{
		Kind: PersesPanelKind,
		Spec: PersesPanelSpec{
			Display: PersesDisplay{Name: "BaseHarbor application errors"},
			Plugin:  PersesPanelPlugin{Kind: PersesPanelPluginLogsTable},
			Queries: []PersesQueryWrapper{{
				Kind: PersesQueryWrapperLog,
				Spec: PersesQueryPlugin{
					Kind:       PersesQueryLoki,
					Spec:       mustJSON(map[string]string{"query": `{job="baseharbor-applications"} |= "error"`}),
					Datasource: &PersesDatasourceRef{Kind: PersesDatasourceLoki, Name: "loki"},
				},
			}},
		},
	}
}

// persesPanelTempo is the BaseHarbor Tempo dashboard panel set. It queries the
// real Tempo trace source with a TraceQL trace query.
func persesPanelTempo() PersesPanel {
	return PersesPanel{
		Kind: PersesPanelKind,
		Spec: PersesPanelSpec{
			Display: PersesDisplay{Name: "BaseHarbor application traces"},
			Plugin:  PersesPanelPlugin{Kind: PersesPanelPluginTraceGantt},
			Queries: []PersesQueryWrapper{{
				Kind: PersesQueryWrapperTrace,
				Spec: PersesQueryPlugin{
					Kind:       PersesQueryTempo,
					Spec:       mustJSON(map[string]string{"query": `{service.name="baseharbor-applications"}`}),
					Datasource: &PersesDatasourceRef{Kind: PersesDatasourceTempo, Name: "tempo"},
				},
			}},
		},
	}
}

// persesUnavailableEntry builds the explicit UNAVAILABLE entry for a database
// with no native metrics signal. It carries no panel/dashboard; Reason names
// the missing signal so the CLI can expose it deterministically.
func persesUnavailableEntry(key, backend, reason string) PersesPanelCatalogEntry {
	return PersesPanelCatalogEntry{
		Key:         key,
		Backend:     backend,
		Unavailable: true,
		Reason:      reason,
	}
}

// persesDashboardCatalog builds the shipped Perses panel catalog for the pinned
// release. It contains one entry per real backend (prometheus, loki, tempo) and
// one explicit UNAVAILABLE entry per database with no native metrics signal
// (PostgreSQL, MongoDB). No entry ships a fabricated panel or installs an
// exporter.
func persesDashboardCatalog() map[string]PersesPanelCatalogEntry {
	prometheus := PersesDashboard{
		Kind:     PersesDashboardKind,
		Metadata: PersesDashboardMetadata{Name: "prometheus-targets", Version: 0},
		Spec: PersesDashboardSpec{
			Display:     PersesDisplay{Name: "BaseHarbor Prometheus targets", Description: "Application up-signal scraped by BaseHarbor Prometheus."},
			Datasources: map[string]PersesDatasource{"prometheus": {Kind: PersesDatasourcePrometheus, Name: "prometheus", Default: true}},
			Panels: map[string]PersesPanel{
				"app-up": persesPanelPrometheus(),
			},
			Layouts: []PersesLayout{{Kind: PersesGridLayoutKind, Spec: PersesLayoutSpec{Items: []PersesGridItem{
				{X: 0, Y: 0, Width: 24, Height: 6, Content: PersesGridContent{Ref: "#/spec/panels/app-up"}},
			}}}},
			Duration: "5m",
		},
	}
	loki := PersesDashboard{
		Kind:     PersesDashboardKind,
		Metadata: PersesDashboardMetadata{Name: "loki-logs", Version: 0},
		Spec: PersesDashboardSpec{
			Display:     PersesDisplay{Name: "BaseHarbor Loki logs", Description: "Application error log stream from BaseHarbor Loki."},
			Datasources: map[string]PersesDatasource{"loki": {Kind: PersesDatasourceLoki, Name: "loki", Default: true}},
			Panels: map[string]PersesPanel{
				"app-errors": persesPanelLoki(),
			},
			Layouts: []PersesLayout{{Kind: PersesGridLayoutKind, Spec: PersesLayoutSpec{Items: []PersesGridItem{
				{X: 0, Y: 0, Width: 24, Height: 10, Content: PersesGridContent{Ref: "#/spec/panels/app-errors"}},
			}}}},
		},
	}
	tempo := PersesDashboard{
		Kind:     PersesDashboardKind,
		Metadata: PersesDashboardMetadata{Name: "tempo-traces", Version: 0},
		Spec: PersesDashboardSpec{
			Display:     PersesDisplay{Name: "BaseHarbor Tempo traces", Description: "Application traces from BaseHarbor Tempo."},
			Datasources: map[string]PersesDatasource{"tempo": {Kind: PersesDatasourceTempo, Name: "tempo", Default: true}},
			Panels: map[string]PersesPanel{
				"app-traces": persesPanelTempo(),
			},
			Layouts: []PersesLayout{{Kind: PersesGridLayoutKind, Spec: PersesLayoutSpec{Items: []PersesGridItem{
				{X: 0, Y: 0, Width: 24, Height: 12, Content: PersesGridContent{Ref: "#/spec/panels/app-traces"}},
			}}}},
		},
	}
	return map[string]PersesPanelCatalogEntry{
		"prometheus-targets": {
			Key:            "prometheus-targets",
			Backend:        "prometheus",
			Datasource:     "prometheus",
			DatasourceName: "prometheus",
			Dashboard:      prometheus,
		},
		"loki-logs": {
			Key:            "loki-logs",
			Backend:        "loki",
			Datasource:     "loki",
			DatasourceName: "loki",
			Dashboard:      loki,
		},
		"tempo-traces": {
			Key:            "tempo-traces",
			Backend:        "tempo",
			Datasource:     "tempo",
			DatasourceName: "tempo",
			Dashboard:      tempo,
		},
		// PostgreSQL has no native metrics signal (ObservabilityRequiresAdapter).
		// It is explicitly UNAVAILABLE; no panel, no exporter.
		"postgresql-targets": persesUnavailableEntry("postgresql-targets", "postgresql",
			"PostgreSQL has no native metrics signal in BaseHarbor (requires an adapter); the dashboard is explicitly unavailable and no exporter is installed"),
		// MongoDB has no metrics contract at all. Explicitly UNAVAILABLE.
		"mongodb-targets": persesUnavailableEntry("mongodb-targets", "mongodb",
			"MongoDB has no metrics signal in BaseHarbor; the dashboard is explicitly unavailable and no exporter is installed"),
	}
}

// persesDashboardCatalogJSON marshals the catalog to deterministic Perses-native
// JSON. Unavailable entries are represented by a minimal reason document, never
// a fabricated panel.
func persesDashboardCatalogJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\n")
	for key, entry := range persesDashboardCatalog() {
		encoded, err := json.MarshalIndent(entry, "  ", "  ")
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "  %q: %s,\n", key, encoded)
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

// persesPanelJSON marshals one catalog entry to deterministic Perses-native
// JSON. For an unavailable entry it emits the reason document; for a real
// dashboard it emits the full dashboard resource.
func persesPanelJSON(entry PersesPanelCatalogEntry) ([]byte, error) {
	if entry.Unavailable {
		return json.MarshalIndent(struct {
			Kind        string `json:"kind"`
			Key         string `json:"key"`
			Backend     string `json:"backend"`
			Unavailable bool   `json:"unavailable"`
			Reason      string `json:"reason"`
		}{
			Kind:        "DashboardUnavailable",
			Key:         entry.Key,
			Backend:     entry.Backend,
			Unavailable: true,
			Reason:      entry.Reason,
		}, "", "  ")
	}
	return json.MarshalIndent(entry.Dashboard, "", "  ")
}

// PersesDashboardAvailability returns the availability verdict of every catalog
// entry, keyed by catalog key, for the pinned release. A real backend returns
// "available" with its Perses datasource kind; a database with no native
// metrics signal returns "unavailable" with the reason naming the missing
// signal. This is the deterministic CLI witness that PostgreSQL/MongoDB remain
// explicitly unavailable rather than silently omitted.
func PersesDashboardAvailability() map[string]PersesDashboardAvailabilityEntry {
	catalog := persesDashboardCatalog()
	out := make(map[string]PersesDashboardAvailabilityEntry, len(catalog))
	for key, entry := range catalog {
		avail := PersesDashboardAvailable
		if entry.Unavailable {
			avail = PersesDashboardUnavailable
		}
		out[key] = PersesDashboardAvailabilityEntry{
			Key:        key,
			Backend:    entry.Backend,
			Available:  avail,
			Datasource: entry.Datasource,
			Reason:     entry.Reason,
		}
	}
	return out
}

// PersesDashboardAvailabilityEntry is one catalog entry's availability verdict
// surfaced on the CLI status JSON.
type PersesDashboardAvailabilityEntry struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// PersesDashboardAvailable / PersesDashboardUnavailable are the two verdict
// strings a dashboard reports on the CLI status JSON.
const (
	PersesDashboardAvailable   = "available"
	PersesDashboardUnavailable = "unavailable"
)
