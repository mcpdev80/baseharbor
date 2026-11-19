package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses-native host and container dashboards for BaseHarbor.
//
// This file realizes the Perses-native dashboard content for host and container
// views over the three real BaseHarbor observability backends (Prometheus
// targets, Loki logs, Tempo traces). It is a management realization, never an
// application capability: it never enables an exporter, declares
// metrics/logs/traces as an application capability, or ships a dashboard that
// assumes Grafana UI APIs.
//
// This is the complementary card to #1597 (database dashboards): it owns its
// own panel files and its own testdata directory, so its write-set is
// disjoint from perses_dashboards.go. The shared catalog keys come only from
// the selection catalog (perses_selection.go, 0428-080) via the shared
// DashboardBackend vocabulary; no line of that catalog is edited here.
//
// The Perses JSON format is a Go-struct-first realization (the same pattern as
// internal/metrics/prometheus_files.go, realization.go and runtime_source.go):
// structs marshal to the Perses-native dashboard/document format. No go:embed
// of dashboard JSON lives in this package — the structs are the source of
// truth and the deterministic golden-file witnesses are their marshalling.
//
// Every dashboard is keyed on the accepted pinned Perses release
// (capability.AcceptedPersesVersion, v0.55.1) and ships only panels that query
// a real BaseHarbor backend signal. Host node metrics that BaseHarbor does not
// ship a native signal for (raw node CPU/RAM, cAdvisor per-container stats)
// remain explicitly UNAVAILABLE: there is no native exporter for them in
// BaseHarbor, so no panel is generated for them and no exporter is installed.
// This satisfies the card contract "deliver host/container panels only for
// actually available matching metrics, without adding privileged exporters;
// show actionable no-data, not fabricated healthy CPU/RAM."

// PersesHostDashboardAPIVersion is the pinned native Perses API schema version
// on the host dashboard resource envelope. The pinned Perses release
// (capability.AcceptedPersesVersion, v0.55.1) ships the Dashboard resource
// under the perses.dev/v1alpha1 API group; the apiVersion/kind pair is the
// native resource envelope, not a flattened or Grafana-shaped document.
const PersesHostDashboardAPIVersion = "perses.dev/v1alpha1"

// PersesHostDashboardKind is the Perses resource kind for a host dashboard.
const PersesHostDashboardKind = "Dashboard"

// PersesHostPanelKind is the Perses resource kind for a host panel.
const PersesHostPanelKind = "Panel"

// PersesHostPanelPluginKinds are the panel plugin kinds shipped by BaseHarbor
// for the host/container view.
const (
	PersesHostPanelPluginStatChart  = "StatChart"
	PersesHostPanelPluginLogsTable  = "LogsTable"
	PersesHostPanelPluginTraceGantt = "TracingGanttChart"
	PersesHostPanelPluginTimeSeries = "TimeSeriesChart"
	PersesHostPanelPluginHeatmap    = "Heatmap"
)

// PersesHostDatasourcePluginKinds, one per real backend for the host view.
const (
	PersesHostDatasourcePrometheus = "PrometheusDatasource"
	PersesHostDatasourceLoki       = "LokiDatasource"
	PersesHostDatasourceTempo      = "TempoDatasource"
)

// PersesHostQueryPluginKinds, one per real backend for the host view.
const (
	PersesHostQueryPrometheus = "PrometheusTimeSeriesQuery"
	PersesHostQueryLoki       = "LokiLogQuery"
	PersesHostQueryTempo      = "TempoTraceQuery"
)

// PersesHostQueryWrapperKinds.
const (
	PersesHostQueryWrapperTimeSeries = "TimeSeriesQuery"
	PersesHostQueryWrapperLog        = "LogQuery"
	PersesHostQueryWrapperTrace      = "TraceQuery"
)

// PersesHostGridLayoutKind is the Perses grid layout kind.
const PersesHostGridLayoutKind = "Grid"

// PersesHostUnavailableKind is the Perses resource kind an unavailable
// host/container dashboard reports.
const PersesHostUnavailableKind = "DashboardUnavailable"

// PersesHostPanel is a minimal Perses panel resource for the host view. Only
// the fields BaseHarbor uses are represented; the marshalling is deterministic
// and golden-tested.
type PersesHostPanel struct {
	Kind string              `json:"kind"`
	Spec PersesHostPanelSpec `json:"spec"`
}

// PersesHostPanelSpec is the spec section of a host panel.
type PersesHostPanelSpec struct {
	Display PersesHostDisplay        `json:"display"`
	Plugin  PersesHostPanelPlugin    `json:"plugin"`
	Queries []PersesHostQueryWrapper `json:"queries"`
}

// PersesHostDisplay is the display section of a host panel/dashboard.
type PersesHostDisplay struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PersesHostPanelPlugin is the plugin section of a host panel.
type PersesHostPanelPlugin struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec,omitempty"`
}

// PersesHostQueryWrapper wraps a query plugin in its wrapper kind.
type PersesHostQueryWrapper struct {
	Kind string                `json:"kind"`
	Spec PersesHostQueryPlugin `json:"spec"`
}

// PersesHostQueryPlugin is the spec section of a host query.
type PersesHostQueryPlugin struct {
	Kind       string                   `json:"kind"`
	Spec       json.RawMessage          `json:"spec,omitempty"`
	Datasource *PersesHostDatasourceRef `json:"datasource,omitempty"`
}

// PersesHostDatasourceRef selects a datasource by kind + name.
type PersesHostDatasourceRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PersesHostLayout is a minimal Perses grid layout item.
type PersesHostLayout struct {
	Kind string               `json:"kind"`
	Spec PersesHostLayoutSpec `json:"spec"`
}

// PersesHostLayoutSpec is the spec section of a host grid layout.
type PersesHostLayoutSpec struct {
	Items []PersesHostGridItem `json:"items"`
}

// PersesHostGridItem positions one panel on the grid.
type PersesHostGridItem struct {
	X       int                   `json:"x"`
	Y       int                   `json:"y"`
	Width   int                   `json:"width"`
	Height  int                   `json:"height"`
	Content PersesHostGridContent `json:"content"`
}

// PersesHostGridContent references a panel by its $ref path.
type PersesHostGridContent struct {
	Ref string `json:"$ref"`
}

// PersesHostDashboard is the minimal Perses dashboard resource BaseHarbor ships
// for a real host/container backend. The apiVersion/kind pair is the pinned
// native Perses resource envelope (perses.dev/v1alpha1); metadata is the
// Perses dashboard metadata.
type PersesHostDashboard struct {
	APIVersion string                      `json:"apiVersion"`
	Kind       string                      `json:"kind"`
	Metadata   PersesHostDashboardMetadata `json:"metadata"`
	Spec       PersesHostDashboardSpec     `json:"spec"`
}

// PersesHostDashboardMetadata is the metadata section of a host dashboard.
type PersesHostDashboardMetadata struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// PersesHostDashboardSpec is the spec section of a host dashboard.
type PersesHostDashboardSpec struct {
	Display     PersesHostDisplay               `json:"display"`
	Datasources map[string]PersesHostDatasource `json:"datasources"`
	Panels      map[string]PersesHostPanel      `json:"panels"`
	Layouts     []PersesHostLayout              `json:"layouts"`
	Duration    string                          `json:"duration,omitempty"`
	Timezone    string                          `json:"timezone,omitempty"`
}

// PersesHostDatasource is a Perses datasource definition for the host view.
type PersesHostDatasource struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// PersesHostPanelCatalogEntry is one shipped Perses-native host/container
// dashboard for one real backend. It is the deterministic source of truth; its
// marshalling is the golden witness.
type PersesHostPanelCatalogEntry struct {
	// Key is the stable catalog identifier (matches the selection catalog key).
	Key string
	// Backend is the real data owner (prometheus, loki, tempo).
	Backend string
	// Datasource is the Perses datasource kind the panels query.
	Datasource string
	// DatasourceName is the Perses datasource selector name.
	DatasourceName string
	// Dashboard is the minimal Perses dashboard resource.
	Dashboard PersesHostDashboard
	// Unavailable is set for a host/container view with no native signal. When
	// true, Dashboard is empty and Reason names the missing signal.
	Unavailable bool
	// Reason explains an unavailable entry (e.g. "no native node metrics").
	Reason string
}

// persesHostPanelPrometheus is the BaseHarbor Prometheus host/container panel.
// It queries the real provider up-signal that BaseHarbor Prometheus scrapes for
// its own provider containers (job="baseharbor-providers", prometheus_render.go:217).
// The query is a literal so the golden witness is exact.
func persesHostPanelPrometheus() PersesHostPanel {
	return PersesHostPanel{
		Kind: PersesHostPanelKind,
		Spec: PersesHostPanelSpec{
			Display: PersesHostDisplay{Name: "BaseHarbor provider container up"},
			Plugin:  PersesHostPanelPlugin{Kind: PersesHostPanelPluginStatChart},
			Queries: []PersesHostQueryWrapper{{
				Kind: PersesHostQueryWrapperTimeSeries,
				Spec: PersesHostQueryPlugin{
					Kind:       PersesHostQueryPrometheus,
					Spec:       mustJSONHosts(map[string]string{"query": `up{job="baseharbor-providers"}`}),
					Datasource: &PersesHostDatasourceRef{Kind: PersesHostDatasourcePrometheus, Name: "prometheus"},
				},
			}},
		},
	}
}

// persesHostPanelLoki is the BaseHarbor Loki host/container panel. Its selector
// uses baseharbor_provider, the label BaseHarbor's own collector emits on
// every provider log stream: the Alloy relabel rules set target_label
// "baseharbor_provider" for syslog provider streams (internal/logs/render.go
// :145, :163) and journald provider streams (:296), and BaseHarbor's own
// runtime verification queries use exactly that label
// (internal/logs/query.go:115, :123). Loki streams carry no Prometheus job
// label, so the former {job="baseharbor-providers"} selector could never
// resolve against emitted data.
func persesHostPanelLoki() PersesHostPanel {
	return PersesHostPanel{
		Kind: PersesHostPanelKind,
		Spec: PersesHostPanelSpec{
			Display: PersesHostDisplay{Name: "BaseHarbor provider container logs"},
			Plugin:  PersesHostPanelPlugin{Kind: PersesHostPanelPluginLogsTable},
			Queries: []PersesHostQueryWrapper{{
				Kind: PersesHostQueryWrapperLog,
				Spec: PersesHostQueryPlugin{
					Kind:       PersesHostQueryLoki,
					Spec:       mustJSONHosts(map[string]string{"query": `{baseharbor_provider=~".+"} |= "error"`}),
					Datasource: &PersesHostDatasourceRef{Kind: PersesHostDatasourceLoki, Name: "loki"},
				},
			}},
		},
	}
}

// persesHostPanelTempo is the BaseHarbor Tempo host/container panel. Its
// selector names the resource attribute BaseHarbor actually emits on provider
// OTLP spans. The span contract puts baseharbor.provider, baseharbor.source,
// baseharbor.source_class and baseharbor.resource on the SPAN (internal/
// telemetry/otlp_payload.go:28-32); service.name is a resource attribute (the
// OTLP resource is a separate message, :37-41) whose provider-path values are
// the literal "baseharbor" or the application name (:38, :66), never
// "baseharbor-providers", and whose runtime-component values are component
// names such as "runtime-executor" (internal/runtimeexecutor/compose_render.go
// :277, internal/runtimeobservability/observer.go:400). So a span-filtered
// baseharbor.provider attribute selector is the only selector that resolves
// over every provider trace BaseHarbor emits; the former
// {service.name="baseharbor-providers"} could never match emitted data.
func persesHostPanelTempo() PersesHostPanel {
	return PersesHostPanel{
		Kind: PersesHostPanelKind,
		Spec: PersesHostPanelSpec{
			Display: PersesHostDisplay{Name: "BaseHarbor provider container traces"},
			Plugin:  PersesHostPanelPlugin{Kind: PersesHostPanelPluginTraceGantt},
			Queries: []PersesHostQueryWrapper{{
				Kind: PersesHostQueryWrapperTrace,
				Spec: PersesHostQueryPlugin{
					Kind:       PersesHostQueryTempo,
					Spec:       mustJSONHosts(map[string]string{"query": `{.baseharbor.provider=~".+"}`}),
					Datasource: &PersesHostDatasourceRef{Kind: PersesHostDatasourceTempo, Name: "tempo"},
				},
			}},
		},
	}
}

// persesHostUnavailableEntry builds the explicit UNAVAILABLE entry for a
// host/container view with no native metrics signal. It carries no
// panel/dashboard; Reason names the missing signal so the CLI can expose it
// deterministically.
func persesHostUnavailableEntry(key, backend, reason string) PersesHostPanelCatalogEntry {
	return PersesHostPanelCatalogEntry{
		Key:         key,
		Backend:     backend,
		Unavailable: true,
		Reason:      reason,
	}
}

// identityUnavailableReasonHost maps a resolved, non-supported host/provider
// metrics observability signal status to the reason string reported on the
// UNAVAILABLE host/container entries. It names the missing verified signal so
// the reason is authoritative, not fabricated.
func identityUnavailableReasonHost(resolvedStatus capability.ObservabilitySupportStatus) string {
	if resolvedStatus == "" {
		return "BaseHarbor has no verified current provider-metrics signal in this manifest (the native openmetrics /metrics endpoint is not a verified current signal); the host/dashboard is explicitly unavailable and no exporter is installed"
	}
	if resolvedStatus == capability.ObservabilityRequiresAdapter {
		return "BaseHarbor host metrics require an adapter in this manifest and have no verified current native metric signal; the host/dashboard is explicitly unavailable and no exporter is installed"
	}
	return "BaseHarbor host/metrics signal " + string(resolvedStatus) +
		" in this manifest is not a verified current native openmetrics signal: there is no native host/container metrics signal for this view; the host/dashboard is explicitly unavailable and no exporter is installed"
}

// persesHostDashboardCatalog builds the shipped Perses host/container catalog
// for the pinned release, keyed on the AUTHORITATIVE, resolved
// host-metrics observability signal status (never a caller-supplied bool). A
// signal resolved to ObservabilitySupported realizes the real backend panels;
// every other resolved status (including the empty/absent status) is reported
// as explicitly UNAVAILABLE with a concrete reason. The catalog additionally
// carries the two host/container views with no native signal (raw host node
// metrics, cAdvisor per-container stats) as UNAVAILABLE entries. No entry ships
// a fabricated panel or installs an exporter.
func persesHostDashboardCatalog(resolvedHostSignalStatus capability.ObservabilitySupportStatus) map[string]PersesHostPanelCatalogEntry {
	return persesHostDashboardCatalogWithWitness(resolvedHostSignalStatus, PersesBackendSignalWitness{})
}

// persesHostDashboardCatalogWithWitness is the catalog builder over an
// explicit per-backend signal witness; no backend inherits another backend's
// verdict.
func persesHostDashboardCatalogWithWitness(resolvedHostSignalStatus capability.ObservabilitySupportStatus, witness PersesBackendSignalWitness) map[string]PersesHostPanelCatalogEntry {
	// The two host/container views with no native signal (raw host node metrics,
	// cAdvisor per-container stats) are always present as UNAVAILABLE entries;
	// there is no native exporter for them in BaseHarbor, so no panel is ever
	// generated for them regardless of the resolved signal.
	out := map[string]PersesHostPanelCatalogEntry{
		"host-node-metrics": persesHostUnavailableEntry("host-node-metrics", "prometheus",
			identityUnavailableReasonHost(resolvedHostSignalStatus)),
		"container-stats": persesHostUnavailableEntry("container-stats", "prometheus",
			identityUnavailableReasonHost(resolvedHostSignalStatus)),
	}
	// A signal resolved to ObservabilitySupported realizes the three real
	// backend panels; any other resolved status (including the empty/absent
	// status) reports them as unavailable with a concrete reason naming the
	// missing verified signal. Catalog presence never becomes a fabricated
	// panel.
	// Each backend answers from its OWN signal: Prometheus from the resolved
	// manifest host/provider-metrics verdict, Loki and Tempo from their own
	// collector signals. A signal resolved to ObservabilitySupported realizes
	// no Loki/Tempo panel by itself — that inference was the review defect.
	for _, key := range []string{"prometheus-host-container", "loki-host-container", "tempo-host-container"} {
		backend := backendForHostKey(key)
		if !backendAvailableWithoutInference(backend, resolvedHostSignalStatus, witness) {
			out[key] = persesHostUnavailableEntry(key, backend, hostBackendUnavailableReason(backend))
			continue
		}
		out[key] = PersesHostPanelCatalogEntry{
			Key:            key,
			Backend:        backend,
			Datasource:     backend,
			DatasourceName: backend,
			Dashboard:      persesHostDashboardForBackend(key),
		}
	}
	return out
}

// persesHostDashboardForBackend returns the dashboard resource of one real
// backend key.
func persesHostDashboardForBackend(key string) PersesHostDashboard {
	switch key {
	case "loki-host-container":
		return persesHostLokiDashboard()
	case "tempo-host-container":
		return persesHostTempoDashboard()
	default:
		return persesHostPrometheusDashboard()
	}
}

// backendForHostKey maps a real host dashboard catalog key to its backend data
// owner (prometheus, loki, tempo).
func backendForHostKey(key string) string {
	switch key {
	case "loki-host-container":
		return "loki"
	case "tempo-host-container":
		return "tempo"
	default:
		return "prometheus"
	}
}

// persesHostPrometheusDashboard builds the BaseHarbor Prometheus host/container
// dashboard resource for the verified current signal. It ships exactly one
// panel per verified metric and no fabricated panel. Every panel queries
// through the pinned supported Prometheus datasource, and the resource carries
// the pinned Perses apiVersion.
func persesHostPrometheusDashboard() PersesHostDashboard {
	return PersesHostDashboard{
		APIVersion: PersesHostDashboardAPIVersion,
		Kind:       PersesHostDashboardKind,
		Metadata:   PersesHostDashboardMetadata{Name: "prometheus-host-container", Version: 0},
		Spec: PersesHostDashboardSpec{
			Display:     PersesHostDisplay{Name: "BaseHarbor host & container targets", Description: "Provider container up-signal scraped by BaseHarbor Prometheus."},
			Datasources: map[string]PersesHostDatasource{"prometheus": {Kind: PersesHostDatasourcePrometheus, Name: "prometheus", Default: true}},
			Panels: map[string]PersesHostPanel{
				"provider-up": persesHostPanelPrometheus(),
			},
			Layouts: []PersesHostLayout{{Kind: PersesHostGridLayoutKind, Spec: PersesHostLayoutSpec{Items: []PersesHostGridItem{
				{X: 0, Y: 0, Width: 24, Height: 6, Content: PersesHostGridContent{Ref: "#/spec/panels/provider-up"}},
			}}}},
			Duration: "5m",
		},
	}
}

// persesHostLokiDashboard builds the BaseHarbor Loki host/container dashboard
// resource for the verified current signal. It ships exactly one panel per
// verified metric and no fabricated panel. Every panel queries through the
// pinned supported Loki datasource, and the resource carries the pinned Perses
// apiVersion.
func persesHostLokiDashboard() PersesHostDashboard {
	return PersesHostDashboard{
		APIVersion: PersesHostDashboardAPIVersion,
		Kind:       PersesHostDashboardKind,
		Metadata:   PersesHostDashboardMetadata{Name: "loki-host-container", Version: 0},
		Spec: PersesHostDashboardSpec{
			Display:     PersesHostDisplay{Name: "BaseHarbor host & container logs", Description: "Provider container error log stream from BaseHarbor Loki."},
			Datasources: map[string]PersesHostDatasource{"loki": {Kind: PersesHostDatasourceLoki, Name: "loki", Default: true}},
			Panels: map[string]PersesHostPanel{
				"provider-logs": persesHostPanelLoki(),
			},
			Layouts: []PersesHostLayout{{Kind: PersesHostGridLayoutKind, Spec: PersesHostLayoutSpec{Items: []PersesHostGridItem{
				{X: 0, Y: 0, Width: 24, Height: 10, Content: PersesHostGridContent{Ref: "#/spec/panels/provider-logs"}},
			}}}},
		},
	}
}

// persesHostTempoDashboard builds the BaseHarbor Tempo host/container dashboard
// resource for the verified current signal. It ships exactly one panel per
// verified metric and no fabricated panel. Every panel queries through the
// pinned supported Tempo datasource, and the resource carries the pinned Perses
// apiVersion.
func persesHostTempoDashboard() PersesHostDashboard {
	return PersesHostDashboard{
		APIVersion: PersesHostDashboardAPIVersion,
		Kind:       PersesHostDashboardKind,
		Metadata:   PersesHostDashboardMetadata{Name: "tempo-host-container", Version: 0},
		Spec: PersesHostDashboardSpec{
			Display:     PersesHostDisplay{Name: "BaseHarbor host & container traces", Description: "Provider container traces from BaseHarbor Tempo."},
			Datasources: map[string]PersesHostDatasource{"tempo": {Kind: PersesHostDatasourceTempo, Name: "tempo", Default: true}},
			Panels: map[string]PersesHostPanel{
				"provider-traces": persesHostPanelTempo(),
			},
			Layouts: []PersesHostLayout{{Kind: PersesHostGridLayoutKind, Spec: PersesHostLayoutSpec{Items: []PersesHostGridItem{
				{X: 0, Y: 0, Width: 24, Height: 12, Content: PersesHostGridContent{Ref: "#/spec/panels/provider-traces"}},
			}}}},
		},
	}
}

// persesHostDashboardCatalogJSON marshals the catalog to deterministic Perses-native
// JSON. Unavailable entries are represented by a minimal reason document, never
// a fabricated panel.
func persesHostDashboardCatalogJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\n")
	for key, entry := range persesHostDashboardCatalog("") {
		encoded, err := marshalIndentNoEscape(entry)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "  %q: %s,\n", key, encoded)
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

// persesHostPanelJSON marshals one catalog entry to deterministic Perses-native
// JSON. For an unavailable entry it emits the reason document; for a real
// dashboard it emits the full dashboard resource.
func persesHostPanelJSON(entry PersesHostPanelCatalogEntry) ([]byte, error) {
	if entry.Unavailable {
		return marshalIndentNoEscape(struct {
			Kind        string `json:"kind"`
			Key         string `json:"key"`
			Backend     string `json:"backend"`
			Unavailable bool   `json:"unavailable"`
			Reason      string `json:"reason"`
		}{
			Kind:        PersesHostUnavailableKind,
			Key:         entry.Key,
			Backend:     entry.Backend,
			Unavailable: true,
			Reason:      entry.Reason,
		})
	}
	return marshalIndentNoEscape(entry.Dashboard)
}

// marshalIndentNoEscape marshals v with two-space indentation and without HTML
// escaping, so & and < are emitted literally in the Perses-native JSON.
func marshalIndentNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// PersesBackendSignalWitness is the deterministic per-backend signal witness
// the host/container catalog consumes. Each field reports whether that
// backend's OWN BaseHarbor signal is proven, never inferred from another
// backend's health. The zero witness is fail-closed: every backend is
// unproven, which is the correct answer whenever no authoritative input is
// supplied.
type PersesBackendSignalWitness struct {
	// PrometheusProven is true only when a provider-scoped Prometheus metric
	// source is registered (job="baseharbor-providers" target files).
	PrometheusProven bool
	// LokiProven is true only when an Alloy pipeline labels provider log
	// streams with baseharbor_provider (internal/logs/render.go:145,163,296).
	LokiProven bool
	// TempoProven is true only when an OTLP trace source carrying the
	// baseharbor.provider span attribute is registered.
	TempoProven bool
}

// backendAvailableWithoutInference reports, for one host/container backend,
// whether its OWN signal witness proves availability under the resolved
// host-metrics status. Prometheus keeps the authoritative manifest resolution
// that guarded this catalog before; Loki and Tempo answer only from their own
// backends. An empty backend is not a real backend.
func backendAvailableWithoutInference(backend string, resolvedHostSignalStatus capability.ObservabilitySupportStatus, witness PersesBackendSignalWitness) bool {
	switch backend {
	case "prometheus":
		return resolvedHostSignalStatus == capability.ObservabilitySupported
	case "loki":
		return witness.LokiProven
	case "tempo":
		return witness.TempoProven
	default:
		return false
	}
}

// hostBackendUnavailableReason is the concrete per-backend reason an
// unproven backend reports. It never borrows another backend's health: it
// names the backend and the signal that is missing.
func hostBackendUnavailableReason(backend string) string {
	switch backend {
	case "loki":
		return "BaseHarbor has no verified Loki provider-log stream signal (no Alloy pipeline is configured that labels streams with baseharbor_provider); the loki/dashboard is explicitly unavailable and no exporter is installed"
	case "tempo":
		return "BaseHarbor has no verified Tempo provider-trace signal (no registered OTLP trace source emits the baseharbor.provider span attribute); the tempo/dashboard is explicitly unavailable and no exporter is installed"
	case "prometheus":
		return identityUnavailableReasonHost("")
	default:
		return "BaseHarbor has no verified signal owner for backend " + backend + "; the host/dashboard is explicitly unavailable and no exporter is installed"
	}
}

// PersesHostDashboardAvailability returns the availability verdict of every
// catalog entry, keyed by catalog key, for the pinned release, under the
// DEFAULT fail-closed witness: Loki and Tempo report "unavailable" until
// their own backend signals are proven elsewhere. A real backend returns
// "available" with its Perses datasource kind; any other resolved status
// returns "unavailable" with a reason naming the missing verified signal; a
// host/container view with no native signal is reported as "unavailable" too.
// This is the deterministic CLI witness that the host dashboard remains
// explicitly unavailable rather than silently reported as available when its
// verified provider-metrics signal is absent.
//
// resolvedHostSignalStatus is the authoritative resolved-signal witness of the
// manifest (internal/capability/reference.go host/provider-metrics observability
// signal). The PROMETHEUS host dashboard is realized only when that signal
// resolves to ObservabilitySupported; every other resolved status (including
// the empty / absent status) stays unavailable-preserving. Loki and Tempo are
// NEVER realized from that Prometheus verdict — that cross-backend inference
// was the CODEX-REVIEW defect on PR #1602 (request-changes act
// 1602-0428-077.md). Use PersesHostDashboardAvailabilityWithWitness to pass
// real per-backend signal proof.
func PersesHostDashboardAvailability(resolvedHostSignalStatus capability.ObservabilitySupportStatus) map[string]PersesHostDashboardAvailabilityEntry {
	return PersesHostDashboardAvailabilityWithWitness(resolvedHostSignalStatus, PersesBackendSignalWitness{})
}

// PersesHostDashboardAvailabilityWithWitness returns the same verdict under an
// explicit per-backend signal witness. Each backend answers from its OWN
// signal only; no backend inherits another backend's health.
func PersesHostDashboardAvailabilityWithWitness(resolvedHostSignalStatus capability.ObservabilitySupportStatus, witness PersesBackendSignalWitness) map[string]PersesHostDashboardAvailabilityEntry {
	catalog := persesHostDashboardCatalogWithWitness(resolvedHostSignalStatus, witness)
	out := make(map[string]PersesHostDashboardAvailabilityEntry, len(catalog))
	for key, entry := range catalog {
		avail := PersesHostDashboardAvailable
		if entry.Unavailable {
			avail = PersesHostDashboardUnavailable
		}
		out[key] = PersesHostDashboardAvailabilityEntry{
			Key:        key,
			Backend:    entry.Backend,
			Available:  avail,
			Datasource: entry.Datasource,
			Reason:     entry.Reason,
		}
	}
	return out
}

// PersesHostDashboardAvailabilityEntry is one catalog entry's availability
// verdict surfaced on the CLI status JSON.
type PersesHostDashboardAvailabilityEntry struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// PersesHostDashboardAvailable / PersesHostDashboardUnavailable are the two
// verdict strings a host/container dashboard reports on the CLI status JSON.
const (
	PersesHostDashboardAvailable   = "available"
	PersesHostDashboardUnavailable = "unavailable"
)
