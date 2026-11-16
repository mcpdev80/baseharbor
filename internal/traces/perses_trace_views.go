package traces

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Perses trace view layer (0428-085).
//
// This file realizes the native Perses trace VIEW that visualises the real
// BaseHarbor Tempo backend. Perses is a management/visualization surface, never
// an application capability: this view only binds the existing, Tempo-owned
// traces endpoint to a native Perses trace view. It declares NO Traces
// capability of its own (Perses only visualises the backend data owner that
// already ships the real traces signal) and names the real owner (Tempo) as an
// owner reference, never as a Perses-declared capability.
//
// This file follows the same struct-first, deterministic-JSON realization as
// internal/metrics/prometheus_files.go and the lane-091 Tempo datasource
// adapter (internal/metrics/perses_tempo_datasource.go): the Perses view
// document is a Go-struct-first realization and the golden-file witness is their
// marshalling. The lane-091 adapter lives in internal/metrics; that package
// imports internal/traces, so this view is the trace-side owner of the Perses
// view layer and builds from Tempo's OWN domain data (traces.ProviderEndpoint /
// traces.Placement / application.HasTraceSignal), mirroring the adapter's
// PersesTempoDatasource shape with its own struct. This view never writes or
// rewrites the lane-091 adapter.
//
// Fail-closed: the view layer never fabricates a view document for an unselected,
// unowned, missing, or unauthorized Tempo path. Its builder enforces Tempo
// selection (HasTraceSignal), Tempo ownership and presence
// (os.ErrNotExist), a materialized management endpoint over the runtime TLS
// boundary, and a managed Tempo path; a missing, denied, or externally-owned
// Tempo path therefore never yields a view document, and pin validation alone
// must never map into a "verified" view.
//
// Schema: the repo's own Perses-native view/document format (precedent files),
// never an external Grafana UI API or dashboard JSON.

// Perses trace view identity.
const (
	// PersesTraceViewName is the Perses trace view selector name.
	PersesTraceViewName = "traces"
	// PersesTracePanelKind is the Perses native Trace panel plugin kind. It is
	// the pinned native plugin, not a Grafana UI API.
	PersesTracePanelKind = "tracesPanel"
)

// PersesTraceViewError is returned by the fail-closed view builder when a native
// Perses trace view cannot be produced. It names the concrete reason so callers
// and tests can assert the exact fail-closed verdict rather than a fabricated
// view.
type PersesTraceViewError struct {
	Detail string
}

func (e *PersesTraceViewError) Error() string {
	return fmt.Sprintf("Perses trace view cannot be built: %s — the Perses trace view requires a selected, owned, materialized Tempo path, so this fails closed rather than fabricating a view document", e.Detail)
}

// PersesTracePanel is one Perses-native Trace panel in the trace view. It
// references the lane-091 Tempo datasource by its selector name and carries the
// trace query surface scoped by actual ownership. Only the fields BaseHarbor
// uses are represented; the marshalling is deterministic and golden-tested.
type PersesTracePanel struct {
	// Title is the human-readable panel title.
	Title string `json:"title"`
	// Kind is the Perses native panel plugin kind: the pinned native plugin,
	// never a Grafana UI API kind.
	Kind string `json:"kind"`
	// DatasourceName is the selector name of the Tempo datasource this panel
	// queries, mirroring the lane-091 adapter's Perses datasource selector.
	DatasourceName string `json:"datasourceName,omitempty"`
	// NoData is a Perses-native no-data replacement shown when the scoped
	// Tempo query returns no spans: a meaningful human message, never a bare
	// placeholder.
	NoData string `json:"noData,omitempty"`
	// Range is the default relative time range the panel renders (Perses
	// "range" field).
	Range string `json:"range,omitempty"`
}

// PersesTraceView is the native Perses trace VIEW document produced by this
// file. Only the fields BaseHarbor uses are represented; the marshalling is
// deterministic and golden-tested.
type PersesTraceView struct {
	// Kind is the Perses native view document kind.
	Kind string `json:"kind"`
	// ID is the stable view identifier.
	ID string `json:"id"`
	// Title is the human-readable view title.
	Title string `json:"title"`
	// Owner is the real observability capability owner Perses visualises. It is
	// the Tempo provider kind, named as an owner reference, NOT a Perses-declared
	// capability. It is not serialized: the view declares no capability of its
	// own.
	Owner string `json:"-"`
	// DatasourceName is the selector name of the Tempo datasource the view
	// references, mirroring the lane-091 adapter's Perses datasource selector.
	DatasourceName string `json:"datasourceName,omitempty"`
	// DatasourceURL is the reachable Tempo management endpoint the view queries,
	// taken from the materialized Tempo provider files over the runtime TLS
	// boundary. Empty when the endpoint is not yet materialized; the
	// fail-closed builder then still fails.
	DatasourceURL string `json:"datasourceURL,omitempty"`
	// Panels are the native Perses Trace panels. Exactly one panel is produced
	// for the scoped Tempo datasource; more panels are owned by downstream work.
	Panels []PersesTracePanel `json:"panels,omitempty"`
	// Reachability is exactly one of:
	//   - "unverified": the contract is defined and fail-closed, but no real
	//     Tempo query was observed (the only reachable state this Core produces
	//     for a selected, pinned surface).
	//   - "verified": only when a real Tempo query observed the source. That
	//     probe does not exist in this Core, so this state can never be produced
	//     here.
	Reachability string `json:"reachability,omitempty"`
}

// PersesTraceViewResult is the builder's fail-closed verdict for the Tempo trace
// view.
type PersesTraceViewResult struct {
	// View is the produced Perses native trace view document.
	View PersesTraceView
	// Reachability is exactly PersesTraceViewUnverified for any view this Core
	// produces (a real Tempo query probe does not exist here).
	Reachability string
}

// PersesTraceViewUnverified is the only reachability state this Core reports for
// the optional Tempo trace view: the contract is defined and fail-closed, but no
// real Tempo query was observed, so reachability stays unverified.
const PersesTraceViewUnverified = "unverified"

// PersesTraceViewNow builds the native Perses trace view for the selected,
// pinned Tempo path. It is fail-closed:
//
//   - without a pinned release that validates -> no view (fail-closed),
//   - without a selected Traces signal (application.HasTraceSignal) -> no view
//     (fail-closed),
//   - with the Tempo provider not provisioned (os.ErrNotExist) -> no view
//     (fail-closed),
//   - with an externally-owned Tempo path (ScopeExternal) -> no view
//     (fail-closed),
//   - without a materialized Tempo management endpoint -> no view (fail-closed),
//   - with a selected, owned, materialized Tempo path -> a native Perses trace
//     view document with exactly one scoped Trace panel, reachability
//     "unverified" (no real Tempo query probe exists here).
//
// It never returns a fabricated view for an unselected, unowned, missing, or
// unauthorized Tempo path, and pin validation alone must never map into a
// "verified" view.
func PersesTraceViewNow(m application.Manifest) (PersesTraceViewResult, error) {
	// Fail-closed without a pinned release that validates.
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return PersesTraceViewResult{}, &PersesTraceViewError{Detail: err.Error()}
	}
	// Fail-closed on selection: Tempo is a provider-owned backend the Perses
	// surface only visualises. It is materialized only when the Traces signal is
	// selected (HasTraceSignal). An unselected Traces path must never yield a
	// view document, even when Perses is selected.
	if !application.HasTraceSignal(m) {
		return PersesTraceViewResult{}, &PersesTraceViewError{Detail: "the Traces signal is not selected"}
	}
	// Fail-closed on ownership and presence: the Tempo provider reports the
	// missing path as absent. A missing or externally-owned Tempo path must
	// never fabricate a view document.
	files, placement, err := ExistingProviderFiles(m)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PersesTraceViewResult{}, &PersesTraceViewError{Detail: "the Tempo provider is not provisioned"}
		}
		return PersesTraceViewResult{}, &PersesTraceViewError{Detail: err.Error()}
	}
	if placement.Scope == capability.ScopeExternal {
		return PersesTraceViewResult{}, &PersesTraceViewError{Detail: "the selected Tempo path is externally owned"}
	}
	// Fail-closed on a materialized endpoint: the Tempo management endpoint over
	// the runtime TLS boundary is required to build the document. A missing
	// endpoint means Tempo is not provisioned and the URL must not be fabricated.
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return PersesTraceViewResult{}, &PersesTraceViewError{Detail: fmt.Sprintf("the Tempo management endpoint is not materialized (%s)", err.Error())}
	}
	view := PersesTraceView{
		Kind:           "view",
		ID:             PersesTraceViewName,
		Title:          "BaseHarbor traces",
		Owner:          string(capability.ProviderTempo),
		DatasourceName: datasourceName,
		DatasourceURL:  endpoint,
		Reachability:   PersesTraceViewUnverified,
		Panels: []PersesTracePanel{{
			Title:          "BaseHarbor traces",
			Kind:           PersesTracePanelKind,
			DatasourceName: datasourceName,
			NoData:         "No traces for this owner in the selected range",
			Range:          "-15m",
		}},
	}
	return PersesTraceViewResult{
		View:         view,
		Reachability: PersesTraceViewUnverified,
	}, nil
}

// datasourceName is the single Tempo datasource selector name the trace view
// references. It is the lane-091 adapter's Perses datasource selector.
const datasourceName = "tempo"

// PersesTraceViewJSON marshals the produced view deterministically
// (json.MarshalIndent with trailing newline), mirroring the golden-file witness
// pattern used across the metrics package and this lane's adapter.
func PersesTraceViewJSON(v PersesTraceView) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// compile-time guard so the fail-closed view path stays reachable and the
// imports are used by downstream wiring.
var _ = errors.New
var _ = strings.TrimSpace
