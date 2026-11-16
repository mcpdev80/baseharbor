package metrics

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses-native identity dashboards for BaseHarbor.
//
// This file realizes the Perses-native dashboard content for the identity
// provider (Keycloak) on the verified current Keycloak metric signal only. It
// is a management realization, never an application capability: it never
// enables an exporter, declares metrics as an application capability, or ships
// a dashboard that assumes Grafana UI APIs or the obsolete Keycloak SPI.
//
// The Perses JSON format is a Go-struct-first realization (the same pattern as
// internal/metrics/prometheus_files.go, internal/metrics/realization.go and
// internal/metrics/runtime_source.go): structs marshal to the Perses-native
// dashboard/document format. No go:embed of dashboard JSON lives in this
// package — the structs are the source of truth and the deterministic
// golden-file witnesses are their marshalling.
//
// The verified current Keycloak metric signal is the identity provider's
// native openmetrics endpoint (internal/capability/reference.go Keycloak
// integration descriptor, signal "keycloak-metrics", Kind=metrics,
// Status=supported, Mode=native, Protocol=openmetrics, Verification=backend
// -query, Port=9000, Path=/metrics). It is the CURRENT signal: Keycloak
// (Quarkus/Micrometer) exposes standard openmetrics there. It is NOT the
// obsolete legacy Keycloak SPI. Identity panels are built only on this
// verified signal.
//
// The panels issue PromQL against the verified current Keycloak metric. Perses
// ships a single native PromQL datasource, the Prometheus datasource
// (PrometheusDatasource, see https://perses.dev/docs/plugins/prometheus);
// there is no Keycloak datasource plugin in the pinned Perses release. The
// Keycloak openmetrics endpoint is therefore realized through the pinned
// supported Prometheus datasource — the same authorized-datasource pattern the
// database-dashboards precedent uses — and never through a fabricated
// KeycloakDatasource. Keycloak metrics reach Prometheus by scrape; Perses
// queries them by PromQL through that datasource.
//
// A dashboard is realized only when the verified Keycloak metric signal is
// present for the manifest. When the signal is absent the identity dashboard
// is explicitly UNAVAILABLE with a reason naming the missing verified signal:
// no fabricated panel, no exporter installed, no silent Grafana-parity claim.
//
// Availability is proven by the AUTHORITATIVE, resolved Keycloak metric signal
// (internal/capability/reference.go), never by a caller-supplied bool. The
// resolved status is mapped to the available verdict: only
// ObservabilitySupported (native openmetrics /metrics, verified by
// backend-query) realizes the panels; every other resolved status is reported
// as explicitly unavailable. Where the signal is not resolvable to a
// supported status at this layer the catalog stays unavailable-preserving (the
// enable-path resolver that wires the registry signal is a separate owner task).
//
// The enable-path binding of this catalog onto the selection catalog
// (internal/capability/perses_selection.go) is OUT OF SCOPE for this lane
// (FÄCHERREGEL: that file belongs to the separate 0428-080 lane whose card is
// not yet accepted). This file owns its own catalog, its own Perses JSON types
// (deliberately distinct from the 0428-076 database-dashboards types, which
// live on a different branch) and its own availability witness; the fan-in
// wiring is left for the coordinator.

// identityKeycloakBackend is the identity backend data owner the dashboard
// realizes. It must match a real BaseHarbor backend, never a Grafana
// assumption.
const identityKeycloakBackend = "keycloak"

// identityDashboardKey is the stable catalog key of the identity dashboard.
const identityDashboardKey = "identity-keycloak"

// Perses identity dashboard/panel resource kinds. The identity dashboard uses
// the single pinned REST/DaC Perses form: `kind: Dashboard` + metadata + spec,
// NO apiVersion (the apiVersion envelope belongs to the separate Operator CRD
// form and mixing the two is not valid against either contract).
const (
	// PersesIdentityDashboardKind is the Perses Dashboard resource kind.
	PersesIdentityDashboardKind = "Dashboard"
	// PersesIdentityPanelKind is the Perses Panel resource kind.
	PersesIdentityPanelKind = "Panel"
)

// Perses identity panel plugin kinds shipped by BaseHarbor.
const (
	PersesIdentityPanelGaugeChart = "GaugeChart"
	PersesIdentityPanelTimeSeries = "TimeSeriesChart"
	PersesIdentityPanelStatChart  = "StatChart"
)

// Perses identity datasource plugin kind. The only native PromQL datasource
// shipped with the pinned Perses release is the Prometheus datasource; the
// identity panels query the verified Keycloak metric through it.
const PersesIdentityDatasourcePrometheus = "PrometheusDatasource"

// Perses identity datasource selector name. Prometheus scrapes the Keycloak
// openmetrics endpoint; Perses selects that scrape target here.
const PersesIdentityDatasourceName = "keycloak"

// Perses identity query plugin kind for the TimeSeriesQuery type. Perses
// ships only PrometheusTimeSeriesQuery as its native PromQL query.
const (
	PersesIdentityQueryWrapperTimeSeries = "TimeSeriesQuery"
	PersesIdentityQueryPluginTimeSeries  = "PrometheusTimeSeriesQuery"
)

// Perses identity grid layout kind.
const PersesIdentityGridLayoutKind = "Grid"

// IdentityKeycloakMetricName are the documented current Keycloak (Quarkus/
// Micrometer) openmetrics the identity panels query. These are real, verified
// current-version metrics exposed at the native /metrics endpoint (Port 9000),
// NOT the obsolete legacy Keycloak SPI.
const (
	IdentityMetricJvmMemoryUsed      = "jvm_memory_used_bytes"
	IdentityMetricHttpServerRequests = "http_server_requests_seconds_count"
	IdentityMetricKeycloakUsers      = "keycloak_users"
)

// PersesIdentityPanelSpec is the spec section of a Perses identity panel.
type PersesIdentityPanelSpec struct {
	Display PersesIdentityDisplay        `json:"display"`
	Plugin  PersesIdentityPanelPlugin    `json:"plugin"`
	Queries []PersesIdentityQueryWrapper `json:"queries"`
}

// PersesIdentityDisplay is the display section of a Perses panel/dashboard.
type PersesIdentityDisplay struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PersesIdentityPanelPlugin is the plugin section of a Perses identity panel.
type PersesIdentityPanelPlugin struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec,omitempty"`
}

// PersesIdentityQueryWrapper wraps a query plugin in its wrapper kind.
type PersesIdentityQueryWrapper struct {
	Kind string                    `json:"kind"`
	Spec PersesIdentityQueryPlugin `json:"spec"`
}

// PersesIdentityQueryPlugin is the spec section of a Perses identity query.
type PersesIdentityQueryPlugin struct {
	Kind       string                       `json:"kind"`
	Spec       json.RawMessage              `json:"spec,omitempty"`
	Datasource *PersesIdentityDataSourceRef `json:"datasource,omitempty"`
}

// PersesIdentityDataSourceRef selects a datasource by kind + name.
type PersesIdentityDataSourceRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PersesIdentityPanel is one Perses identity panel resource. Only the fields
// BaseHarbor uses are represented; the marshalling is deterministic and
// golden-tested.
type PersesIdentityPanel struct {
	Kind string                  `json:"kind"`
	Spec PersesIdentityPanelSpec `json:"spec"`
}

// PersesIdentityDashboardResource is a Perses-native dashboard resource in the
// single pinned REST/DaC form (the form the integration path actually emits):
// `kind: Dashboard` + metadata + spec, exactly as the Perses REST Dashboard API
// (/api/v1/projects/<project>/dashboards) and dashboard-as-code SDK expect. It
// deliberately carries NO `apiVersion`: that field belongs to the separate
// Perses Operator CRD form (`apiVersion: perses.dev/v1alpha1` +
// `kind: PersesDashboard`), and mixing the two into one resource is not valid
// against either contract. This matches BaseHarbor's host-dashboards precedent
// (perses_dashboards_hosts.go: PersesHostDashboard{Kind, Metadata, Spec}, no
// APIVersion), which emits the same `kind: Dashboard` + metadata + spec
// envelope.
type PersesIdentityDashboardResource struct {
	Kind     string                          `json:"kind"`
	Metadata PersesIdentityDashboardMetadata `json:"metadata"`
	Spec     PersesIdentityDashboardSpec     `json:"spec"`
}

// PersesIdentityDashboardMetadata is the metadata section of the dashboard.
type PersesIdentityDashboardMetadata struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// PersesIdentityDashboardSpec is the spec section of the identity dashboard.
type PersesIdentityDashboardSpec struct {
	Display     PersesIdentityDisplay               `json:"display"`
	Datasources map[string]PersesIdentityDataSource `json:"datasources"`
	Panels      map[string]PersesIdentityPanel      `json:"panels"`
	Layouts     []PersesIdentityLayout              `json:"layouts"`
	Duration    string                              `json:"duration,omitempty"`
	Timezone    string                              `json:"timezone,omitempty"`
}

// PersesIdentityDataSource is a Perses datasource definition.
type PersesIdentityDataSource struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// PersesIdentityLayout is a minimal Perses grid layout item.
type PersesIdentityLayout struct {
	Kind string                   `json:"kind"`
	Spec PersesIdentityLayoutSpec `json:"spec"`
}

// PersesIdentityLayoutSpec is the spec section of the grid layout.
type PersesIdentityLayoutSpec struct {
	Items []PersesIdentityGridItem `json:"items"`
}

// PersesIdentityGridItem positions one panel on the grid.
type PersesIdentityGridItem struct {
	X       int                       `json:"x"`
	Y       int                       `json:"y"`
	Width   int                       `json:"width"`
	Height  int                       `json:"height"`
	Content PersesIdentityGridContent `json:"content"`
}

// PersesIdentityGridContent references a panel by its $ref path.
type PersesIdentityGridContent struct {
	Ref string `json:"$ref"`
}

// PersesIdentityPanelCatalogEntry is one shipped Perses-native identity
// dashboard entry. It is the deterministic source of truth; its marshalling is
// the golden witness.
type PersesIdentityPanelCatalogEntry struct {
	// Key is the stable catalog identifier (the identity dashboard key).
	Key string
	// Backend is the real data owner this dashboard realizes.
	Backend string
	// Datasource is the Perses datasource kind the panels query.
	Datasource string
	// DatasourceName is the Perses datasource selector name.
	DatasourceName string
	// Dashboard is the minimal Perses identity dashboard resource.
	Dashboard PersesIdentityDashboardResource
	// Unavailable is set when the verified Keycloak metric signal is absent.
	Unavailable bool
	// Reason explains an unavailable entry.
	Reason string
}

// persesIdentityPanelJvmMemory is the BaseHarbor Keycloak JVM-memory panel. It
// queries the verified current Keycloak metric jvm_memory_used_bytes through the
// pinned supported Prometheus datasource. The query is a literal so the golden
// witness is exact.
func persesIdentityPanelJvmMemory() PersesIdentityPanel {
	return PersesIdentityPanel{
		Kind: PersesIdentityPanelKind,
		Spec: PersesIdentityPanelSpec{
			Display: PersesIdentityDisplay{Name: "Keycloak JVM memory used"},
			Plugin:  PersesIdentityPanelPlugin{Kind: PersesIdentityPanelGaugeChart},
			Queries: []PersesIdentityQueryWrapper{{
				Kind: PersesIdentityQueryWrapperTimeSeries,
				Spec: PersesIdentityQueryPlugin{
					Kind:       PersesIdentityQueryPluginTimeSeries,
					Spec:       mustIdentityJSON(map[string]string{"query": `jvm_memory_used_bytes{area="heap"}`}),
					Datasource: &PersesIdentityDataSourceRef{Kind: PersesIdentityDatasourcePrometheus, Name: PersesIdentityDatasourceName},
				},
			}},
		},
	}
}

// persesIdentityPanelHttpServer is the BaseHarbor Keycloak HTTP server request
// panel. It queries the verified current Keycloak metric
// http_server_requests_seconds_count through the pinned supported Prometheus
// datasource. The query is a literal so the golden witness is exact.
func persesIdentityPanelHttpServer() PersesIdentityPanel {
	return PersesIdentityPanel{
		Kind: PersesIdentityPanelKind,
		Spec: PersesIdentityPanelSpec{
			Display: PersesIdentityDisplay{Name: "Keycloak HTTP server request duration"},
			Plugin:  PersesIdentityPanelPlugin{Kind: PersesIdentityPanelTimeSeries},
			Queries: []PersesIdentityQueryWrapper{{
				Kind: PersesIdentityQueryWrapperTimeSeries,
				Spec: PersesIdentityQueryPlugin{
					Kind:       PersesIdentityQueryPluginTimeSeries,
					Spec:       mustIdentityJSON(map[string]string{"query": `http_server_requests_seconds_count{status!="2xx"}`}),
					Datasource: &PersesIdentityDataSourceRef{Kind: PersesIdentityDatasourcePrometheus, Name: PersesIdentityDatasourceName},
				},
			}},
		},
	}
}

// persesIdentityPanelUsers is the BaseHarbor Keycloak users panel. It queries
// the verified current Keycloak metric keycloak_users through the pinned
// supported Prometheus datasource. The query is a literal so the golden
// witness is exact.
func persesIdentityPanelUsers() PersesIdentityPanel {
	return PersesIdentityPanel{
		Kind: PersesIdentityPanelKind,
		Spec: PersesIdentityPanelSpec{
			Display: PersesIdentityDisplay{Name: "Keycloak users"},
			Plugin:  PersesIdentityPanelPlugin{Kind: PersesIdentityPanelStatChart},
			Queries: []PersesIdentityQueryWrapper{{
				Kind: PersesIdentityQueryWrapperTimeSeries,
				Spec: PersesIdentityQueryPlugin{
					Kind:       PersesIdentityQueryPluginTimeSeries,
					Spec:       mustIdentityJSON(map[string]string{"query": `keycloak_users{type="real"}`}),
					Datasource: &PersesIdentityDataSourceRef{Kind: PersesIdentityDatasourcePrometheus, Name: PersesIdentityDatasourceName},
				},
			}},
		},
	}
}

// identityKeycloakDashboard builds the BaseHarbor identity dashboard resource
// for the verified current Keycloak signal. It ships exactly one panel per
// verified metric and no fabricated panel. Every panel queries through the
// pinned supported Prometheus datasource, and the resource is the single pinned
// REST/DaC Perses form: `kind: Dashboard` + metadata + spec, NO apiVersion.
func identityKeycloakDashboard() PersesIdentityDashboardResource {
	return PersesIdentityDashboardResource{
		Kind:     PersesIdentityDashboardKind,
		Metadata: PersesIdentityDashboardMetadata{Name: identityDashboardKey, Version: 0},
		Spec: PersesIdentityDashboardSpec{
			Display: PersesIdentityDisplay{
				Name:        "BaseHarbor Keycloak identity",
				Description: "Identity-provider panels for the verified current Keycloak metric signal (native openmetrics /metrics).",
			},
			Datasources: map[string]PersesIdentityDataSource{
				// The Keycloak openmetrics endpoint is reached through the
				// pinned supported Prometheus datasource (the only native
				// PromQL datasource shipped with the pinned Perses release).
				"keycloak": {Kind: PersesIdentityDatasourcePrometheus, Name: PersesIdentityDatasourceName, Default: true},
			},
			Panels: map[string]PersesIdentityPanel{
				"keycloak-jvm-memory":    persesIdentityPanelJvmMemory(),
				"keycloak-http-requests": persesIdentityPanelHttpServer(),
				"keycloak-users":         persesIdentityPanelUsers(),
			},
			Layouts: []PersesIdentityLayout{{Kind: PersesIdentityGridLayoutKind, Spec: PersesIdentityLayoutSpec{Items: []PersesIdentityGridItem{
				{X: 0, Y: 0, Width: 8, Height: 6, Content: PersesIdentityGridContent{Ref: "#/spec/panels/keycloak-jvm-memory"}},
				{X: 8, Y: 0, Width: 8, Height: 6, Content: PersesIdentityGridContent{Ref: "#/spec/panels/keycloak-http-requests"}},
				{X: 16, Y: 0, Width: 8, Height: 6, Content: PersesIdentityGridContent{Ref: "#/spec/panels/keycloak-users"}},
			}}}},
			Duration: "5m",
		},
	}
}

// identityUnavailableEntry builds the explicit UNAVAILABLE entry for an identity
// dashboard with no verified current Keycloak metric signal. It carries no
// panel/dashboard; Reason names the missing verified signal.
func identityUnavailableEntry(reason string) PersesIdentityPanelCatalogEntry {
	return PersesIdentityPanelCatalogEntry{
		Key:         identityDashboardKey,
		Backend:     identityKeycloakBackend,
		Unavailable: true,
		Reason:      reason,
	}
}

// identityUnavailableReason maps a resolved, non-supported Keycloak metric
// signal status to the reason string reported on the UNAVAILABLE entry. It
// names the missing verified signal so the reason is authoritative, not
// fabricated.
func identityUnavailableReason(resolvedStatus capability.ObservabilitySupportStatus) string {
	if resolvedStatus == "" {
		return "Keycloak has no verified current metric signal in this manifest (the native openmetrics /metrics endpoint is not a verified current signal); the identity dashboard is explicitly unavailable and no exporter is installed, and no obsolete Keycloak SPI is assumed"
	}
	if resolvedStatus == capability.ObservabilityRequiresAdapter {
		return "Keycloak metrics require an adapter in this manifest and have no verified current native metric signal; the identity dashboard is explicitly unavailable and no exporter is installed"
	}
	return "Keycloak metric signal " + string(resolvedStatus) + " in this manifest is not the verified current supported native openmetrics signal; the identity dashboard is explicitly unavailable and no exporter is installed, and no obsolete Keycloak SPI is assumed"
}

// identityDashboardCatalog builds the shipped Perses identity dashboard
// catalog for the pinned release, keyed on the AUTHORITATIVE, resolved
// Keycloak metric signal status (never a caller-supplied bool). A signal
// resolved to ObservabilitySupported realizes the panels; every other resolved
// status (including the empty/absent status) is reported as explicitly
// UNAVAILABLE with a concrete reason. This is the unavailable-preserving
// realization mirroring the valkey precedent: catalog presence never becomes a
// fabricated panel.
func identityDashboardCatalog(resolvedKeycloakSignalStatus capability.ObservabilitySupportStatus) map[string]PersesIdentityPanelCatalogEntry {
	if resolvedKeycloakSignalStatus == capability.ObservabilitySupported {
		return map[string]PersesIdentityPanelCatalogEntry{
			identityDashboardKey: {
				Key:            identityDashboardKey,
				Backend:        identityKeycloakBackend,
				Datasource:     PersesIdentityDatasourcePrometheus,
				DatasourceName: PersesIdentityDatasourceName,
				Dashboard:      identityKeycloakDashboard(),
			},
		}
	}
	return map[string]PersesIdentityPanelCatalogEntry{
		identityDashboardKey: identityUnavailableEntry(
			identityUnavailableReason(resolvedKeycloakSignalStatus)),
	}
}

// identityDashboardJSON marshals one catalog entry to deterministic Perses-
// native JSON. For an unavailable entry it emits a minimal reason document,
// never a fabricated panel.
func identityDashboardJSON(entry PersesIdentityPanelCatalogEntry) ([]byte, error) {
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

// PersesIdentityDashboardAvailability returns the availability verdict of the
// identity dashboard, keyed by catalog key and resolved Keycloak metric signal
// status, for the pinned release. A signal resolved to ObservabilitySupported
// returns "available" with its supported Prometheus datasource kind; any other
// resolved status returns "unavailable" with the reason naming the missing
// verified signal. This is the deterministic witness that the identity
// dashboard remains explicitly unavailable rather than silently omitted when
// its verified signal is absent.
//
// resolvedKeycloakSignalStatus is the authoritative resolved-signal witness of
// the manifest (internal/capability/reference.go Keycloak metric signal). The
// identity dashboard is realized only when that signal resolves to
// ObservabilitySupported.
func PersesIdentityDashboardAvailability(resolvedKeycloakSignalStatus capability.ObservabilitySupportStatus) map[string]PersesIdentityDashboardAvailabilityEntry {
	catalog := identityDashboardCatalog(resolvedKeycloakSignalStatus)
	out := make(map[string]PersesIdentityDashboardAvailabilityEntry, len(catalog))
	for key, entry := range catalog {
		avail := PersesIdentityDashboardAvailable
		if entry.Unavailable {
			avail = PersesIdentityDashboardUnavailable
		}
		out[key] = PersesIdentityDashboardAvailabilityEntry{
			Key:        key,
			Backend:    entry.Backend,
			Available:  avail,
			Datasource: entry.Datasource,
			Reason:     entry.Reason,
		}
	}
	return out
}

// PersesIdentityDashboardAvailabilityEntry is the identity dashboard entry's
// availability witness surfaced on the CLI status JSON.
type PersesIdentityDashboardAvailabilityEntry struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// PersesIdentityDashboardAvailable / PersesIdentityDashboardUnavailable are the
// two verdict strings the identity dashboard reports on the CLI status JSON.
const (
	PersesIdentityDashboardAvailable   = "available"
	PersesIdentityDashboardUnavailable = "unavailable"
)

// identityPanelKeysSorted returns the identity dashboard panel keys in
// deterministic order for golden/witness stability.
func identityPanelKeysSorted(dashboard PersesIdentityDashboardResource) []string {
	keys := make([]string, 0, len(dashboard.Spec.Panels))
	for k := range dashboard.Spec.Panels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// mustIdentityJSON marshals v to compact JSON and panics on error. It is a
// build helper for Perses query specs from compile-time-constant inputs, so
// marshalling cannot fail at runtime.
func mustIdentityJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic("perses identity panel spec marshal failed: " + err.Error())
	}
	return json.RawMessage(data)
}

// identityDashboardJSONCompact marshals the identity dashboard to compact JSON
// for inline witnesses (determinism checks).
func identityDashboardJSONCompact(entry PersesIdentityPanelCatalogEntry) (string, error) {
	raw, err := identityDashboardJSON(entry)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return "", err
	}
	return b.String(), nil
}
