package metrics

import (
	"encoding/json"
	"fmt"
	"strings"
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
// ObservabilityRequiresAdapter in internal/capability/reference.go (Valkey has
// NO native metrics signal in BaseHarbor) and the Perses seam
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
const persesValkeyUnavailableReason = "Valkey has no native metrics signal in BaseHarbor (observability status RequiresAdapter); the dashboard is explicitly unavailable and no exporter is installed"

// persesValkeyDashboardCatalog builds the shipped Perses Valkey catalog for the
// pinned release. It contains exactly one entry (valkey-targets). Valkey has no
// native metrics signal in BaseHarbor, so the entry is explicitly UNAVAILABLE:
// no panel ships and no exporter is installed. This is the conditional
// dashboard: a native Valkey signal would promote this entry to a real
// dashboard, but that signal does not yet exist.
func persesValkeyDashboardCatalog() map[string]PersesValkeyPanelCatalogEntry {
	return map[string]PersesValkeyPanelCatalogEntry{
		PersesValkeyDashboardCatalogKey: {
			Key:         PersesValkeyDashboardCatalogKey,
			Backend:     "valkey",
			Unavailable: true,
			Reason:      persesValkeyUnavailableReason,
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
// would emit the full dashboard resource.
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
// Valkey catalog entry, keyed by catalog key, for the pinned release. A real
// backend returns "available" with its Perses datasource kind; Valkey today
// returns "unavailable" with the reason naming the missing signal. This is the
// deterministic CLI witness that Valkey remains explicitly unavailable rather
// than silently fabricated — the mirror image of the PostgreSQL/MongoDB
// handling in the #1597 inventory.
func PersesValkeyDashboardAvailability() map[string]PersesValkeyDashboardAvailabilityEntry {
	catalog := persesValkeyDashboardCatalog()
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
