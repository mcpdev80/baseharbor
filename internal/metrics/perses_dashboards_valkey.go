package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses-native Valkey dashboard for BaseHarbor.
//
// This file realizes the Perses-native dashboard content for the single real
// BaseHarbor key-value backend, Valkey. It is a management realization, never an
// application capability: it never enables an exporter, declares
// metrics/logs/traces as an application capability, or ships a dashboard that
// assumes Grafana UI APIs.
//
// This is the 0428-081 fan-out (faechregel): it owns its own panel file and its
// own testdata directory (perses-valkey), so its write-set is disjoint from the
// #1597 database dashboards (perses_dashboards.go, keys
// prometheus-targets / loki-logs / tempo-traces / postgresql-targets /
// mongodb-targets) and the 0428-077 host/container dashboards
// (perses_dashboards_hosts.go). No line of that inventory is edited here; the
// only shared identifier is the Perses resource-kind constant, which is not an
// inventory key.
//
// The Perses JSON format is a Go-struct-first realization (the same pattern as
// internal/metrics/prometheus_files.go, realization.go and runtime_source.go):
// structs marshal to the Perses-native dashboard/document format. No go:embed
// of dashboard JSON lives in this package — the structs are the source of
// truth and the deterministic golden-file witnesses are their marshalling.
//
// Signal contract (0428-081 task): Valkey metrics are declared
// ObservabilityUnsupported in internal/capability/reference.go (Valkey has
// NO native metrics signal in BaseHarbor; 0428-182 / f5ce95af moved the
// declaration from the earlier ObservabilityRequiresAdapter to
// ObservabilityUnsupported — either way no real native signal, so the shipped
// entry stays EXPLICITLY UNAVAILABLE). The Perses seam
// (internal/capability/perses.go) forbids false-declaring a metric. This file
// therefore delivers the first Valkey dashboard CONDITIONAL on validated
// Valkey/compatible metrics: Valkey currently has no native signal, so the
// shipped entry is EXPLICITLY UNAVAILABLE — no panel, no installed exporter —
// mirroring the PostgreSQL/MongoDB handling in the #1597 inventory. This is the
// card contract's "deliver dashboards conditional on validated metrics; when a
// provider has NO native signal, mark it explicitly UNAVAILABLE like
// postgres/mongo". No fabricated Valkey panel ships and no exporter is
// installed; a future native Valkey signal would promote this entry.

// PersesValkeyDashboardKind is the Perses resource kind for a Valkey dashboard.
const PersesValkeyDashboardKind = "Dashboard"

// PersesValkeyUnavailableKind is the Perses resource kind an unavailable
// Valkey dashboard reports.
const PersesValkeyUnavailableKind = "DashboardUnavailable"

// PersesValkeyDashboardCatalogKey is the stable catalog identifier for the
// Valkey dashboard. It is disjoint from the #1597 and 0428-077 catalog keys.
const PersesValkeyDashboardCatalogKey = "valkey-targets"

// PersesValkeyPanel is a minimal Perses panel resource. Only the fields
// BaseHarbor uses are represented; the marshalling is deterministic and
// golden-tested. (Distinct by name from perses_dashboards.go's PersesPanel;
// the two files never share a type.)
type PersesValkeyPanel struct {
	Kind string                `json:"kind"`
	Spec PersesValkeyPanelSpec `json:"spec"`
}

// PersesValkeyPanelSpec is the spec section of a Valkey panel.
type PersesValkeyPanelSpec struct {
	Display PersesValkeyDisplay        `json:"display"`
	Plugin  PersesValkeyPanelPlugin    `json:"plugin"`
	Queries []PersesValkeyQueryWrapper `json:"queries"`
}

// PersesValkeyDisplay is the display section of a Valkey panel/dashboard.
type PersesValkeyDisplay struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PersesValkeyPanelPlugin is the plugin section of a Valkey panel.
type PersesValkeyPanelPlugin struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec,omitempty"`
}

// PersesValkeyQueryWrapper wraps a query plugin in its wrapper kind.
type PersesValkeyQueryWrapper struct {
	Kind string                  `json:"kind"`
	Spec PersesValkeyQueryPlugin `json:"spec"`
}

// PersesValkeyQueryPlugin is the spec section of a Valkey query.
type PersesValkeyQueryPlugin struct {
	Kind       string                     `json:"kind"`
	Spec       json.RawMessage            `json:"spec,omitempty"`
	Datasource *PersesValkeyDatasourceRef `json:"datasource,omitempty"`
}

// PersesValkeyDatasourceRef selects a datasource by kind + name.
type PersesValkeyDatasourceRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PersesValkeyLayout is a minimal Perses grid layout item.
type PersesValkeyLayout struct {
	Kind string                 `json:"kind"`
	Spec PersesValkeyLayoutSpec `json:"spec"`
}

// PersesValkeyLayoutSpec is the spec section of a Valkey grid layout.
type PersesValkeyLayoutSpec struct {
	Items []PersesValkeyGridItem `json:"items"`
}

// PersesValkeyGridItem positions one panel on the grid.
type PersesValkeyGridItem struct {
	X       int                     `json:"x"`
	Y       int                     `json:"y"`
	Width   int                     `json:"width"`
	Height  int                     `json:"height"`
	Content PersesValkeyGridContent `json:"content"`
}

// PersesValkeyGridContent references a panel by its $ref path.
type PersesValkeyGridContent struct {
	Ref string `json:"$ref"`
}

// PersesValkeyDashboard is the minimal Perses dashboard resource BaseHarbor
// ships for a real Valkey backend. When Valkey has a native signal this carries
// a real dashboard resource; today it is UNAVAILABLE and stays empty.
type PersesValkeyDashboard struct {
	Kind     string                        `json:"kind"`
	Metadata PersesValkeyDashboardMetadata `json:"metadata"`
	Spec     PersesValkeyDashboardSpec     `json:"spec"`
}

// PersesValkeyDashboardMetadata is the metadata section of a Valkey dashboard.
type PersesValkeyDashboardMetadata struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// PersesValkeyDashboardSpec is the spec section of a Valkey dashboard.
type PersesValkeyDashboardSpec struct {
	Display     PersesValkeyDisplay               `json:"display"`
	Datasources map[string]PersesValkeyDatasource `json:"datasources"`
	Panels      map[string]PersesValkeyPanel      `json:"panels"`
	Layouts     []PersesValkeyLayout              `json:"layouts"`
	Duration    string                            `json:"duration,omitempty"`
	Timezone    string                            `json:"timezone,omitempty"`
}

// PersesValkeyDatasource is a Perses datasource definition for the Valkey view.
type PersesValkeyDatasource struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// PersesValkeyPanelCatalogEntry is one shipped Perses-native Valkey dashboard.
// It is the deterministic source of truth; its marshalling is the golden
// witness.
type PersesValkeyPanelCatalogEntry struct {
	// Key is the stable catalog identifier.
	Key string
	// Backend is the real data owner (valkey).
	Backend string
	// Datasource is the Perses datasource kind the panels query (empty when
	// unavailable: no datasource has a native signal behind it).
	Datasource string
	// DatasourceName is the Perses datasource selector name (empty when
	// unavailable).
	DatasourceName string
	// Dashboard is the minimal Perses dashboard resource.
	Dashboard PersesValkeyDashboard
	// Unavailable is set when Valkey has no native metrics signal. When true,
	// Dashboard is empty and Reason names the missing signal.
	Unavailable bool
	// Reason explains an unavailable entry (e.g. "no native Valkey metrics").
	Reason string
}

// persesValkeyUnavailableReason is the concrete reason the Valkey dashboard
// reports today. It names the missing native signal exactly, per the card
// contract.
const persesValkeyUnavailableReason = "Valkey has no native metrics signal in BaseHarbor (observability status ObservabilityUnsupported); the dashboard is explicitly unavailable and no exporter is installed"

// mustJSONPersesValkey marshals v to compact JSON without HTML escaping, and
// panics on error. It is a build helper for building Valkey Perses plugin specs
// from raw compile-time constants, so marshalling cannot fail at runtime. The
// & character is emitted literally (not \u0026) so the golden witnesses are
// human-readable Perses-native JSON. It is disjoint from the host/container
// mustJSONHosts helper.
func mustJSONPersesValkey(v any) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic("perses valkey panel spec marshal failed: " + err.Error())
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// persesValkeyResolvedSignalStatus is the real producer of the Valkey dashboard
// availability verdict — the authoritative, non-forgeable resolution of the
// shipped reference provider catalog. It answers the Valkey backend's OWN
// native metrics signal only, never inferred from another backend's health or
// from any manifest declaration the caller supplies.
//
// It mirrors hostProviderMetricsNativeVerified() (internal/capability/perses.go)
// and ResolveHostDashboardSignalStatus(), which gate the host/container
// dashboards the same way: it iterates the reference catalog (the same
// source host/provider metrics read) and returns ObservabilitySupported only
// when Valkey's metrics signal is declared real, native and backend-verified.
// A RequiresAdapter signal (as Valkey has today) resolves to
// ObservabilityRequiresAdapter and stays unavailable-preserving. Manifest
// declaration alone (perses selected, backend running) is NOT availability —
// only this catalog resolution promotes the entry.
func persesValkeyResolvedSignalStatus() capability.ObservabilitySupportStatus {
	for _, descriptor := range capability.ReferenceIntegrations() {
		if descriptor.ID != "baseharbor/valkey" {
			continue
		}
		for _, signal := range descriptor.Observability.Signals {
			if signal.Name != "metrics" {
				continue
			}
			switch signal.Status {
			case capability.ObservabilitySupported:
				return capability.ObservabilitySupported
			case capability.ObservabilityRequiresAdapter:
				return capability.ObservabilityRequiresAdapter
			default:
				return signal.Status
			}
		}
	}
	return ""
}

// persesValkeyPanelValkeyUp is the one verified Valkey panel: the Valkey
// provider up-signal, queried through the pinned Prometheus datasource. It is
// the sole panel of the verified dashboard resource.
func persesValkeyPanelValkeyUp() PersesValkeyPanel {
	return PersesValkeyPanel{
		Kind: PersesValkeyPanelKind,
		Spec: PersesValkeyPanelSpec{
			Display: PersesValkeyDisplay{Name: "Valkey provider up"},
			Plugin: PersesValkeyPanelPlugin{
				Kind: PersesValkeyPanelPluginGraph,
			},
			Queries: []PersesValkeyQueryWrapper{{
				Kind: PersesValkeyQueryWrapperKind,
				Spec: PersesValkeyQueryPlugin{
					Kind: PersesValkeyQueryPluginGraph,
					// The verified panel carries a real Perses query: a Prometheus
					// expression selecting the BaseHarbor Prometheus scrape of the
					// valkey provider container. It scopes on baseharbor_provider —
					// the real emitted label (see
					// internal/metrics/prometheus_files.go targetGroupPayload: job,
					// baseharbor_provider, baseharbor_source, baseharbor_source_class,
					// baseharbor_metrics_path; there is NO `backend` label emitted
					// anywhere). A query keying on `backend="valkey"` can never
					// resolve against the emitted series (0428-183 round 3 Finding 1);
					// the source-scoped baseharbor_provider label is the one that
					// matches, mirroring the host/container working pattern
					// (perses_dashboards_hosts.go:234/:249-251). A graph panel without
					// this data basis would be a fabricated panel (0428-183 P2: every
					// exported panel must carry a runnable query).
					Spec:       mustJSONPersesValkey(map[string]string{"query": `up{baseharbor_provider="valkey"}`}),
					Datasource: &PersesValkeyDatasourceRef{Kind: PersesValkeyDatasourcePrometheus, Name: "valkey"},
				},
			}},
		},
	}
}

// persesValkeyDashboardCatalogWithStatus is the catalog builder over the
// authoritative resolved Valkey backend-signal status and its verification
// result. A signal resolved to ObservabilitySupported AND backend-verified
// (verification == ObservabilityVerifyBackend) realizes the real dashboard
// panels; any other resolved status (the empty/absent status, a
// RequiresAdapter signal that only requires an adapter, or a supported-but-not
// verified signal) reports the entry as explicitly UNAVAILABLE with a concrete
// reason naming the missing verified signal. Catalog presence never becomes a
// fabricated panel.
//
// verification is an injectable test anchor, NOT a finished enum the caller
// pre-resolves: the CLI and every production caller pass their real
// backend-verification verdict (persesValkeyBackendVerified()), so the
// verified-panel code only runs when a real verified signal is present. A test
// drives the verified branch by supplying a backend-verified anchor
// (ObservabilityVerifyBackend), never by setting the supported status alone —
// that is what keeps the positive witness real (0428-183 round 4) and not a
// caller-asserted enum value.
func persesValkeyDashboardCatalogWithStatus(status capability.ObservabilitySupportStatus, verification capability.ObservabilityVerificationMode) map[string]PersesValkeyPanelCatalogEntry {
	if status == capability.ObservabilitySupported && verification == capability.ObservabilityVerifyBackend {
		return map[string]PersesValkeyPanelCatalogEntry{
			PersesValkeyDashboardCatalogKey: {
				Key:            PersesValkeyDashboardCatalogKey,
				Backend:        "valkey",
				Datasource:     "prometheus",
				DatasourceName: "valkey",
				Dashboard:      persesValkeyDashboard(),
			},
		}
	}
	return map[string]PersesValkeyPanelCatalogEntry{
		PersesValkeyDashboardCatalogKey: {
			Key:         PersesValkeyDashboardCatalogKey,
			Backend:     "valkey",
			Unavailable: true,
			Reason:      persesValkeyUnavailableReason,
		},
	}
}

// persesValkeyBackendVerified is the real backend-verification verdict the
// production callers feed the availability gate. It is the authoritative
// resolution of whether Valkey's metrics signal (if any) has actually been
// backend-verified — the second axis the verified dashboard requires on top of
// a supported signal. Today, with the reference catalog resolving Valkey
// metrics to ObservabilityUnsupported, this is false: no verified signal
// exists, so the shipped entry stays UNAVAILABLE. A future native,
// backend-verified Valkey signal promotes this to true and realizes the
// dashboard panels.
func persesValkeyBackendVerified() capability.ObservabilityVerificationMode {
	return capability.ObservabilityVerifyNone
}

// persesValkeyDashboardCatalog builds the shipped Perses Valkey catalog for the
// pinned release. It contains exactly one entry (valkey-targets). Valkey has no
// native metrics signal in BaseHarbor today, so the entry is explicitly
// UNAVAILABLE: no panel ships and no exporter is installed. This is the
// conditional dashboard: a native Valkey signal would promote this entry to a
// real dashboard, but that signal does not yet exist. It resolves the status
// from the authoritative reference catalog (persesValkeyResolvedSignalStatus).
func persesValkeyDashboardCatalog() map[string]PersesValkeyPanelCatalogEntry {
	return persesValkeyDashboardCatalogWithStatus(
		persesValkeyResolvedSignalStatus(),
		persesValkeyBackendVerified(),
	)
}

// persesValkeyDashboard builds the BaseHarbor Valkey dashboard resource for the
// verified current signal. It ships real panels queried through the pinned
// Prometheus datasource and no fabricated panel. This is the verified-branch
// realization: a real Valkey metrics signal of this manifest promotes the entry
// from the explicitly-unavailable reason document to this dashboard resource.
func persesValkeyDashboard() PersesValkeyDashboard {
	return PersesValkeyDashboard{
		Kind:     PersesValkeyDashboardKind,
		Metadata: PersesValkeyDashboardMetadata{Name: "valkey-targets", Version: 0},
		Spec: PersesValkeyDashboardSpec{
			Display:     PersesValkeyDisplay{Name: "BaseHarbor Valkey target", Description: "Valkey provider container up-signal scraped by BaseHarbor Prometheus."},
			Datasources: map[string]PersesValkeyDatasource{"prometheus": {Kind: PersesValkeyDatasourcePrometheus, Name: "valkey", Default: true}},
			Panels: map[string]PersesValkeyPanel{
				"valkey-up": persesValkeyPanelValkeyUp(),
			},
			Layouts: []PersesValkeyLayout{{Kind: PersesValkeyGridLayoutKind, Spec: PersesValkeyLayoutSpec{Items: []PersesValkeyGridItem{
				{X: 0, Y: 0, Width: 24, Height: 6, Content: PersesValkeyGridContent{Ref: "#/spec/panels/valkey-up"}},
			}}}},
			Duration: "5m",
		},
	}
}

// persesValkeyDashboardCatalogJSON marshals the catalog to deterministic
// Perses-native JSON. The single unavailable entry is represented by a minimal
// reason document, never a fabricated panel.
func persesValkeyDashboardCatalogJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\n")
	for key, entry := range persesValkeyDashboardCatalog() {
		encoded, err := persesValkeyPanelJSON(entry)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "  %q: %s,\n", key, encoded)
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

// persesValkeyPanelJSON marshals one catalog entry to deterministic Perses-native
// JSON. For the unavailable entry it emits the reason document; a real dashboard
// emits the full dashboard resource. The available/unavailable branch is the
// catalog's own claim, never a self-confirmation of the caller.
func persesValkeyPanelJSON(entry PersesValkeyPanelCatalogEntry) ([]byte, error) {
	if entry.Unavailable {
		return marshalIndentNoEscape(struct {
			Kind        string `json:"kind"`
			Key         string `json:"key"`
			Backend     string `json:"backend"`
			Unavailable bool   `json:"unavailable"`
			Reason      string `json:"reason"`
		}{
			Kind:        PersesValkeyUnavailableKind,
			Key:         entry.Key,
			Backend:     entry.Backend,
			Unavailable: true,
			Reason:      entry.Reason,
		})
	}
	return marshalIndentNoEscape(entry.Dashboard)
}

// PersesValkeyDashboardAvailability returns the availability verdict of the
// Valkey catalog entry, keyed by catalog key, for the pinned release, over the
// authoritative resolved Valkey backend-signal status. A real, backend-verified
// Valkey native metrics signal (resolved to ObservabilitySupported) returns
// "available" with its Perses datasource kind; any other status (the empty
// status of the CLI face, a RequiresAdapter signal, or any other resolved
// status) returns "unavailable" with the reason naming the missing verified
// signal. This is the deterministic CLI witness that Valkey is reported as
// available ONLY when its own verified signal proves it, never self-confirmed
// by the manifest declaration it is meant to gate — the mirror image of the
// host/container dashboard status (perses_dashboards_hosts.go) and the
// verified-only predicate the identity dashboard hardened (0428-078).
//
// status is the verdict produced by persesValkeyResolvedSignalStatus() (the
// real reference-catalog producer); a caller may pass it through, but the CLI
// face must NOT fabricate a status — the CLI face reads the real producer.
//
// verification is the real backend-verification verdict the CLI face reads
// (persesValkeyBackendVerified()); a caller may pass it through. Only a
// supported AND backend-verified signal promotes the entry.
func PersesValkeyDashboardAvailability(status capability.ObservabilitySupportStatus, verification capability.ObservabilityVerificationMode) map[string]PersesValkeyDashboardAvailabilityEntry {
	catalog := persesValkeyDashboardCatalogWithStatus(status, verification)
	out := make(map[string]PersesValkeyDashboardAvailabilityEntry, len(catalog))
	for key, entry := range catalog {
		avail := PersesValkeyDashboardAvailable
		if entry.Unavailable {
			avail = PersesValkeyDashboardUnavailable
		}
		out[key] = PersesValkeyDashboardAvailabilityEntry{
			Key:        key,
			Backend:    entry.Backend,
			Available:  avail,
			Datasource: entry.Datasource,
			Reason:     entry.Reason,
		}
	}
	return out
}

// PersesValkeyDashboardAvailabilityResolved is the canonical production caller:
// it reads BOTH the real reference-catalog producer (persesValkeyResolvedSignalStatus)
// and the real backend-verification verdict (persesValkeyBackendVerified) and
// surfaces the resulting availability verdict on the CLI status JSON. The CLI
// face wires exactly this function so it never fabricates a status or a
// verification verdict.
func PersesValkeyDashboardAvailabilityResolved() map[string]PersesValkeyDashboardAvailabilityEntry {
	return PersesValkeyDashboardAvailability(
		persesValkeyResolvedSignalStatus(),
		persesValkeyBackendVerified(),
	)
}

// PersesValkeyDashboardAvailabilityEntry is the Valkey catalog entry's
// availability verdict surfaced on the CLI status JSON.
type PersesValkeyDashboardAvailabilityEntry struct {
	Key        string `json:"key"`
	Backend    string `json:"backend"`
	Available  string `json:"available"`
	Datasource string `json:"datasource,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// PersesValkeyDashboardAvailable / PersesValkeyDashboardUnavailable are the two
// verdict strings a Valkey dashboard reports on the CLI status JSON.
const (
	PersesValkeyDashboardAvailable   = "available"
	PersesValkeyDashboardUnavailable = "unavailable"
)

// PersesValkey datasource / panel / query plugin kinds. These are the Perses
// native kinds the verified Valkey dashboard resource declares; they are only
// emitted by the verified branch, never by the unavailable reason document.
const (
	PersesValkeyDatasourcePrometheus = "prometheus"
	PersesValkeyPanelKind            = "Panel"
	PersesValkeyPanelPluginGraph     = "graph"
	PersesValkeyQueryWrapperKind     = "query"
	PersesValkeyQueryPluginGraph     = "graph"
	PersesValkeyGridLayoutKind       = "grid"
)
