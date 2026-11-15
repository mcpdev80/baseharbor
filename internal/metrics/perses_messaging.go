package metrics

import (
	"encoding/json"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// PersesDefaultRelease mirrors the seam pin (internal/capability/perses.go,
// lane-0428-075-perses-seam) so the messaging catalog stays keyed on the same
// pinned release without importing the seam file. It MUST stay in lock-step
// with the seam pin.
var persesDefaultRelease = struct {
	Version                 string
	Image                   string
	PrometheusPluginVersion string
	AuthenticationClass     string
}{
	Version:                 "v0.55.1",
	Image:                   "persesdev/perses:v0.55.1",
	PrometheusPluginVersion: "v0.6.0",
	AuthenticationClass:     "unsupported",
}

// Perses-native messaging dashboards.
//
// This file implements the messaging side of the Perses-native dashboard
// catalog (see the selection catalog in capability/perses_selection.go). It
// answers the next question after the seam pins the release: for a backend
// that has been selected, which Perses-native dashboards are ELIGIBLE and how
// are they realized.
//
// Eligibility is derived from manifest truth + the real provider signal
// inventory, never from an assumed exporter. A backend that has no real
// Perses-native signal of the pinned release is reported as NOT_ELIGIBLE with
// a concrete reason and never provisioned, never as a false Grafana-parity
// claim.
//
// Messaging (RabbitMQ) ships NO native observability signal on its integration
// descriptor (internal/capability/reference.go: RabbitMQIntegration declares
// only the amqp interface and an optional management-ui; no metrics/logs/traces
// signal). The task scope therefore muterts messaging on the same pattern as
// PostgreSQL/Valkey: a messaging dashboard has no native Perses signal and is
// reported UNAVAILABLE, never provisioned. This file encodes that contract.
//
// Perses-native dashboards are a management realization, never an application
// capability, so this code never enables an exporter or declares
// metrics/logs/traces as an application capability.

// PersesMessagingDashboard is the stable catalog key for the single messaging
// dashboard BaseHarbor could ship for RabbitMQ. It never exists at runtime
// because RabbitMQ has no native Perses signal.
const PersesMessagingDashboardKey = "rabbitmq-messaging"

// PersesMessagingDashboardStatus is the selection verdict for the messaging
// dashboard. It is exactly one of two values so consumers can distinguish a
// genuine "not available" from a provisioning error.
type PersesMessagingDashboardStatus string

const (
	// PersesMessagingDashboardUnavailable means the messaging backend ships
	// no native Perses signal of the pinned release, so no dashboard can be
	// provisioned. This is the only status the messaging dashboard ever
	// reports; it mirrors the PostgreSQL/Valkey requires-adapter case.
	PersesMessagingDashboardUnavailable PersesMessagingDashboardStatus = "unavailable"
)

// PersesMessagingDashboardEligibility is the selection verdict for the single
// messaging dashboard.
type PersesMessagingDashboardEligibility struct {
	// Dashboard is the stable catalog key ("rabbitmq-messaging").
	Dashboard string
	// Eligible is always false: messaging ships no native Perses signal.
	Eligible bool
	// Reason explains the verdict. It is empty only when Eligible.
	Reason string
	// Status is the machine-readable verdict. For messaging it is always
	// PersesMessagingDashboardUnavailable.
	Status PersesMessagingDashboardStatus
	// Backend is the backend data owner the dashboard would realize.
	Backend string
	// VersionPinned records the accepted Perses release this verdict keys on.
	VersionPinned string
}

// Perses panel/dashboard JSON types.
//
// Perses is a native Dashboard-as-Code tool: dashboards are authored in Go (or
// CUE) via percli and emitted as JSON/YAML, then applied with percli. The
// native dashboard JSON shape (perses.dev, perses.dev/v1alpha1) is:
//
//	apiVersion: perses.dev/v1alpha1
//	kind: PersesDashboard
//	metadata: {name: <key>}
//	spec: {duration, display:{name}, layouts:[{kind:Grid,...}],
//	      datasources:{<id>:{default,plugin:{spec:{directUrl}}}},
//	      panels:{<id>:{kind:"Panel",spec:{display:{name},
//	            plugin:{kind:"TimeSeriesChart"},
//	            queries:[{kind:"TimeSeriesQuery",
//	                   spec:{plugin:{kind:"PrometheusTimeSeriesQuery",
//	                             spec:{query,seriesNameFormat}}}}]}}}}
//
// No Perses Go SDK module is vendored in BaseHarbor (go.mod carries no perses
// dependency), so the structs below are hand-written Go mirrors of that schema,
// serialized via encoding/json. They render the panel templates the catalog
// would ship; the messaging catalog itself is UNAVAILABLE and ships no live
// panels, so these structs encode the contract, not a running surface.

// PersesDashboardVersion is the perses.dev API version the emitted JSON targets.
const PersesDashboardVersion = "perses.dev/v1alpha1"

// PersesDashboard is the top-level native Perses dashboard object.
type PersesDashboard struct {
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Metadata   PersesDashboardMeta `json:"metadata"`
	Spec       PersesDashboardSpec `json:"spec"`
}

// PersesDashboardMeta is the object metadata of a native Perses dashboard.
type PersesDashboardMeta struct {
	Name string `json:"name"`
}

// PersesDashboardSpec is the spec of a native Perses dashboard.
type PersesDashboardSpec struct {
	Duration    string                      `json:"duration"`
	Display     PersesDashboardDisplay      `json:"display"`
	Layouts     []PersesLayout              `json:"layouts,omitempty"`
	Datasources map[string]PersesDatasource `json:"datasources,omitempty"`
	Panels      map[string]PersesPanel      `json:"panels,omitempty"`
}

// PersesDashboardDisplay carries the human-facing dashboard title.
type PersesDashboardDisplay struct {
	Name string `json:"name"`
}

// PersesLayout is a single grid layout of a native Perses dashboard.
type PersesLayout struct {
	Kind string           `json:"kind"`
	Spec PersesLayoutSpec `json:"spec"`
}

// PersesLayoutSpec is the grid layout spec of a native Perses dashboard.
type PersesLayoutSpec struct {
	Items []PersesLayoutItem `json:"items"`
}

// PersesLayoutItem places a panel in a grid row.
type PersesLayoutItem struct {
	Content string `json:"content"`
	Height  int    `json:"height"`
	Width   int    `json:"width"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

// PersesDatasource is a single datasource of a native Perses dashboard.
type PersesDatasource struct {
	Default bool                   `json:"default,omitempty"`
	Plugin  PersesDatasourcePlugin `json:"plugin"`
}

// PersesDatasourcePlugin wraps the datasource plugin spec.
type PersesDatasourcePlugin struct {
	Spec PersesDatasourcePluginSpec `json:"spec"`
}

// PersesDatasourcePluginSpec is the Prometheus datasource plugin spec.
type PersesDatasourcePluginSpec struct {
	DirectURL string `json:"directUrl"`
}

// PersesPanel is a single panel of a native Perses dashboard.
type PersesPanel struct {
	Kind string          `json:"kind"`
	Spec PersesPanelSpec `json:"spec"`
}

// PersesPanelSpec is the spec of a native Perses panel.
type PersesPanelSpec struct {
	Display PersesPanelDisplay `json:"display"`
	Plugin  PersesPanelPlugin  `json:"plugin"`
	Queries []PersesQuery      `json:"queries"`
}

// PersesPanelDisplay carries the human-facing panel title.
type PersesPanelDisplay struct {
	Name string `json:"name"`
}

// PersesPanelPlugin wraps the panel plugin spec.
type PersesPanelPlugin struct {
	Kind string `json:"kind"`
}

// PersesQuery is a single query of a native Perses panel.
type PersesQuery struct {
	Kind string          `json:"kind"`
	Spec PersesQuerySpec `json:"spec"`
}

// PersesQuerySpec is the spec of a native Perses query.
type PersesQuerySpec struct {
	Plugin           PersesQueryPlugin `json:"plugin"`
	SeriesNameFormat string            `json:"seriesNameFormat,omitempty"`
}

// PersesQueryPlugin wraps the PromQL query plugin spec.
type PersesQueryPlugin struct {
	Kind string                `json:"kind"`
	Spec PersesQueryPluginSpec `json:"spec"`
}

// PersesQueryPluginSpec is the Prometheus time-series query plugin spec.
type PersesQueryPluginSpec struct {
	Query string `json:"query"`
}

// RenderPersesMessagingDashboard serializes the messaging dashboard contract to
// native Perses JSON. Because messaging ships no native Perses signal, the
// rendered dashboard carries an explicit UNAVAILABLE annotation rather than any
// live panel or datasource: it documents the contract, not a running surface.
//
// The returned bytes are deterministic (sorted keys) so golden-file tests are
// stable.
func RenderPersesMessagingDashboard() ([]byte, error) {
	dashboard := PersesDashboard{
		APIVersion: PersesDashboardVersion,
		Kind:       "PersesDashboard",
		Metadata:   PersesDashboardMeta{Name: PersesMessagingDashboardKey},
		Spec: PersesDashboardSpec{
			Duration:    "24h",
			Display:     PersesDashboardDisplay{Name: "Messaging (RabbitMQ) — not available"},
			Datasources: map[string]PersesDatasource{},
			Panels:      map[string]PersesPanel{},
		},
	}
	data, err := json.MarshalIndent(dashboard, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, nil
}

// one dashboard key, keyed on the pinned release and the real signal inventory.
// A consumer iterates this instead of a partial list.
type PersesMessagingCatalog struct {
	release string
}

// DefaultMessagingDashboardCatalog builds the shipped messaging catalog for the
// pinned release. A catalog entry exists only where a real backend signal of
// that release exists; messaging declares none, so the catalog reports an
// explicit UNAVAILABLE verdict rather than a missing entry. It stays in
// lock-step with the seam pin (v0.55.1).
func DefaultMessagingDashboardCatalog() PersesMessagingCatalog {
	return PersesMessagingCatalog{
		release: persesDefaultRelease.Version,
	}
}

// Release returns the pinned release the messaging catalog keys on. It stays in
// lock-step with the seam pin (v0.55.1).
func (c PersesMessagingCatalog) Release() string {
	return c.release
}

// EligibleFor evaluates the selection verdict for the messaging dashboard. It is
// fail-closed: a missing Perses selection or an unpinned/unsupported release
// always fails closed, as does any backend that ships no real Perses-native
// signal. Messaging has no such signal, so the verdict is always UNAVAILABLE.
//
// backendSignals is the real signal inventory of the manifest (see the
// observability signal registry): a dashboard is eligible only when its backend
// actually produces the signal the dashboard queries. Passing a fabricated or
// empty inventory where a backend is expected is not eligibility.
func (c PersesMessagingCatalog) EligibleFor(persesSelected bool, backendSignals map[string]bool) (PersesMessagingDashboardEligibility, error) {
	rabbitMQ := string(capability.ProviderRabbitMQ)
	if !persesSelected {
		return PersesMessagingDashboardEligibility{
			Dashboard:     PersesMessagingDashboardKey,
			Eligible:      false,
			Reason:        "perses observability surface is not selected",
			Status:        PersesMessagingDashboardUnavailable,
			Backend:       rabbitMQ,
			VersionPinned: c.release,
		}, nil
	}
	if persesDefaultRelease.Version == "" || persesDefaultRelease.Image == "" {
		return PersesMessagingDashboardEligibility{
			Dashboard:     PersesMessagingDashboardKey,
			Eligible:      false,
			Reason:        "perses release is not pinned or is invalid (fail-closed)",
			Status:        PersesMessagingDashboardUnavailable,
			Backend:       string(capability.ProviderRabbitMQ),
			VersionPinned: c.release,
		}, nil
	}
	// Messaging ships no native Perses signal on its integration descriptor
	// (internal/capability/reference.go). No real metric family is ever
	// produced, so no dashboard can be provisioned for any inventory.
	if !backendSignals[rabbitMQ] {
		return PersesMessagingDashboardEligibility{
			Dashboard:     PersesMessagingDashboardKey,
			Eligible:      false,
			Reason:        "messaging (RabbitMQ) ships no native Perses signal of " + c.release + " in this manifest",
			Status:        PersesMessagingDashboardUnavailable,
			Backend:       rabbitMQ,
			VersionPinned: c.release,
		}, nil
	}
	// Even when a (fabricated) signal is asserted, messaging still has no
	// real native metric family, so the verdict stays UNAVAILABLE.
	return PersesMessagingDashboardEligibility{
		Dashboard:     PersesMessagingDashboardKey,
		Eligible:      false,
		Reason:        "messaging (RabbitMQ) ships no native Perses metric family; no dashboard can be provisioned",
		Status:        PersesMessagingDashboardUnavailable,
		Backend:       rabbitMQ,
		VersionPinned: c.release,
	}, nil
}

// VerifyMessagingDashboardCatalogBuilds pins the shipped messaging catalog: it
// reports exactly one dashboard with a real backend and a concrete status. A
// catalog with zero entries would silently provision an empty dashboard.
func VerifyMessagingDashboardCatalogBuilds() error {
	catalog := DefaultMessagingDashboardCatalog()
	elig, err := catalog.EligibleFor(true, map[string]bool{
		string(capability.ProviderRabbitMQ): true,
	})
	if err != nil {
		return err
	}
	if elig.Dashboard == "" {
		return fmt.Errorf("perses messaging dashboard has an empty key")
	}
	if elig.Status == "" {
		return fmt.Errorf("perses messaging dashboard has no status")
	}
	if elig.Eligible {
		return fmt.Errorf("perses messaging dashboard must never be eligible: messaging has no native Perses signal")
	}
	return nil
}
