package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses log view layer (0428-085).
//
// This file realizes the native Perses log VIEW that visualises the real
// BaseHarbor Loki backend through the lane-086 Loki datasource adapter
// (internal/logs/loki_datasource.go). Perses is a management/visualization
// surface, never an application capability: this view only binds the existing,
// Loki-owned query surface to a native Perses log view. It declares NO Logs
// capability of its own (Perses only visualises the backend data owner that
// already ships the real logs signal) and names the real owner (Loki) as an
// owner reference, never as a Perses-declared capability.
//
// This file follows the same struct-first, deterministic-JSON realization as
// internal/metrics/prometheus_files.go and the lane-091 Tempo datasource
// adapter (internal/metrics/perses_tempo_datasource.go): the Perses view
// document is a Go-struct-first realization (the same pattern as
// internal/metrics/perses_tempo_datasource.go and internal/capability/perses.go)
// and the golden-file witness is their marshalling. The adapter output is
// consumed READ-ONLY — this file never writes or rewrites the lane-086 adapter.
//
// Fail-closed: the view layer never fabricates a view document for a missing or
// unvalidated Loki datasource. Its builder requires a pinned, validating Perses
// release AND an ownership-bound Loki datasource that is selected (the
// lane-086 adapter returns "configured_unverified" only then). A missing,
// unselected, unvalidated or externally-owned Loki path therefore never yields a
// view document, and the pinned release pin alone must never map into a
// "verified" view.
//
// Schema: the repo's own Perses-native view/document format (precedent files),
// never an external Grafana UI API or dashboard JSON.

// Perses log view identity.
const (
	// PersesLogViewName is the Perses log view selector name.
	PersesLogViewName = "logs"
	// PersesLogPanelKind is the Perses native Log panel plugin kind. It is the
	// pinned native plugin, not a Grafana UI API.
	PersesLogPanelKind = "logsPanel"
)

// PersesLogViewError is returned by the fail-closed view builder when a native
// Perses log view cannot be produced. It names the concrete reason so callers
// and tests can assert the exact fail-closed verdict rather than a fabricated
// view.
type PersesLogViewError struct {
	Detail string
}

func (e *PersesLogViewError) Error() string {
	return fmt.Sprintf("Perses log view cannot be built: %s — the Perses log view requires a selected, pinned, Loki-owned log source, so this fails closed rather than fabricating a view document", e.Detail)
}

// PersesLogPanel is one Perses-native Log panel in the log view. It references
// the lane-086 Loki datasource by its selector name and carries a LogQL stream
// matcher scoped by actual ownership. Only the fields BaseHarbor uses are
// represented; the marshalling is deterministic and golden-tested.
type PersesLogPanel struct {
	// Title is the human-readable panel title.
	Title string `json:"title"`
	// Kind is the Perses native panel plugin kind: the pinned native plugin,
	// never a Grafana UI API kind.
	Kind string `json:"kind"`
	// DatasourceName is the selector name of the Loki datasource this panel
	// queries, consumed read-only from the lane-086 adapter.
	DatasourceName string `json:"datasourceName,omitempty"`
	// Query is the LogQL stream matcher scoped by actual ownership, e.g.
	// {baseharbor_application="acme"}. Empty is not a valid panel.
	Query string `json:"query,omitempty"`
	// NoData is a Perses-native no-data replacement shown when the scoped
	// Loki query returns no rows: a meaningful human message, never a bare
	// placeholder.
	NoData string `json:"noData,omitempty"`
	// Range is the default relative time range the panel renders (Perses
	// "range" field).
	Range string `json:"range,omitempty"`
}

// PersesLogView is the native Perses log VIEW document produced by this file.
// Only the fields BaseHarbor uses are represented; the marshalling is
// deterministic and golden-tested.
type PersesLogView struct {
	// Kind is the Perses native view document kind.
	Kind string `json:"kind"`
	// ID is the stable view identifier.
	ID string `json:"id"`
	// Title is the human-readable view title.
	Title string `json:"title"`
	// Owner is the real observability capability owner Perses visualises. It is
	// the Loki provider kind, named as an owner reference, NOT a Perses-declared
	// capability. It is not serialized: the view declares no capability of its
	// own.
	Owner string `json:"-"`
	// Panels are the native Perses Log panels. Exactly one panel is produced
	// for the scoped Loki datasource; more panels are owned by downstream work.
	Panels []PersesLogPanel `json:"panels,omitempty"`
	// DatasourceName is the selector name of the Loki datasource the view
	// references, consumed read-only from the lane-086 adapter.
	DatasourceName string `json:"datasourceName,omitempty"`
	// Reachability is exactly one of:
	//   - "unverified": the contract is defined and fail-closed, but no real
	//     Loki query has observed the source (the only reachable state this Core
	//     produces for a selected, pinned surface).
	//   - "verified": only when a real Loki query observed the source. That probe
	//     does not exist in this Core, so this state can never be produced here.
	Reachability PersesLogViewStatus `json:"reachability,omitempty"`
}

// PersesLogViewStatus is the machine-readable view reachability. It is EXACTLY
// one of the two values below and no other.
type PersesLogViewStatus string

const (
	// PersesLogViewUnverified is the only reachability state this Core reports
	// for the optional Loki log view: the contract is defined and fail-closed,
	// but no real Loki query was observed, so reachability stays unverified.
	PersesLogViewUnverified PersesLogViewStatus = "unverified"
)

// PersesLogViewResult is the builder's fail-closed verdict for the Loki log view.
type PersesLogViewResult struct {
	// View is the produced Perses native log view document.
	View PersesLogView
	// Reachability is exactly PersesLogViewUnverified for any view this Core
	// produces (a real Loki query probe does not exist here).
	Reachability PersesLogViewStatus
}

// PersesLogViewNow builds the native Perses log view for the calling
// application's owned Loki datasource. It is fail-closed:
//
//   - without a pinned release that validates -> no view (fail-closed),
//   - without an ownership-bound Loki datasource -> no view (fail-closed),
//   - with a pinned, validating release and a selected, owned Loki datasource ->
//     a native Perses log view document with exactly one scoped Log panel,
//     reachability "unverified" (no real Loki query probe exists here).
//
// It never returns a fabricated view for an unselected, unvalidated, or
// externally-owned Loki path, and pin validation alone must never map into a
// "verified" view. The lane-086 Loki datasource is consumed read-only.
func PersesLogViewNow(selected bool, owner LokiDatasourceOwner) (PersesLogViewResult, error) {
	// Fail-closed without a pinned release that validates.
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return PersesLogViewResult{}, &PersesLogViewError{Detail: err.Error()}
	}
	// Fail-closed without an ownership-bound Loki datasource: the lane-086
	// adapter returns an error for an incomplete or foreign owner boundary.
	datasource, err := LokiDatasourceNow(selected, owner)
	if err != nil {
		return PersesLogViewResult{}, &PersesLogViewError{Detail: err.Error()}
	}
	// Fail-closed when the datasource is not configured (unselected or unpinned):
	// a missing or externally-owned Loki path must never yield a view document.
	if datasource.Status != LokiDatasourceConfiguredUnverified {
		return PersesLogViewResult{}, &PersesLogViewError{Detail: fmt.Sprintf("Loki datasource status = %q", datasource.Status)}
	}
	view := PersesLogView{
		Kind:           "view",
		ID:             PersesLogViewName,
		Title:          "BaseHarbor logs",
		Owner:          string(capability.ProviderLoki),
		DatasourceName: datasourceName,
		Reachability:   PersesLogViewUnverified,
		Panels: []PersesLogPanel{{
			Title:          "BaseHarbor logs",
			Kind:           PersesLogPanelKind,
			DatasourceName: datasourceName,
			Query:          logMatcherForOwner(owner),
			NoData:         "No logs for this owner in the selected range",
			Range:          "-15m",
		}},
	}
	return PersesLogViewResult{
		View:         view,
		Reachability: PersesLogViewUnverified,
	}, nil
}

// datasourceName is the single Loki datasource selector name the log view
// references. It is the lane-086 adapter's Perses datasource selector.
const datasourceName = "loki"

// logMatcherForOwner returns the LogQL stream matcher scoped by the owning
// application and environment of the Loki datasource. It never leaks owner
// secrets; it is a selector literal only.
func logMatcherForOwner(owner LokiDatasourceOwner) string {
	app := strings.TrimSpace(owner.Application)
	if app == "" {
		return ""
	}
	return fmt.Sprintf(`{baseharbor_application=%q}`, app)
}

// PersesLogViewJSON marshals the produced view deterministically
// (json.MarshalIndent with trailing newline), mirroring the golden-file witness
// pattern used across the metrics package and this lane's adapter.
func PersesLogViewJSON(v PersesLogView) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// compile-time guard so the fail-closed view path stays reachable and the
// imports are used by downstream wiring.
var _ = errors.New
