package delivery

import (
	"fmt"
	"sort"
	"strings"
)

// HandoffActions are the delivery operations a handoff packet is opened for.
// A handoff never invents an operation: it packages exactly one of these four
// against the selected delivery mode.
const (
	HandoffPromote = "promote"
	HandoffApply   = "apply"
	HandoffRepair  = "repair"
	HandoffDestroy = "destroy"
	handoffNoop    = "noop"
	handoffRefused = "refused"
	handoffObserve = "observe"
)

var handoffActionSet = map[string]struct{}{
	HandoffPromote: {}, HandoffApply: {}, HandoffRepair: {}, HandoffDestroy: {},
}

// TrackingMetadata carries the external reconciler's own tracking annotations
// (e.g. Argo CD/Git app-of-apps references, Helm release ownership, Kustomize
// common labels) for a managed resource. In delegated delivery this metadata is
// owned by the external Git/Helm/Kustomize delivery: BaseHarbor reads, carries
// and displays it, and a handoff must preserve it verbatim. Dropping it would
// silently orphan the resource from the reconciler that tracks it, so loss of
// foreign tracking metadata fails closed. Empty keys and values are not
// metadata; they are malformed input.
type TrackingMetadata map[string]string

func (m TrackingMetadata) validate() error {
	for key, value := range m {
		if strings.TrimSpace(key) == "" || value == "" {
			return fmt.Errorf("tracking metadata entry %q is malformed: key and value are required", key)
		}
	}
	return nil
}

func (m TrackingMetadata) clone() TrackingMetadata {
	if len(m) == 0 {
		return nil
	}
	out := make(TrackingMetadata, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

// DriftObservation is the honest external-evidence record for one resource set:
// what the external desired state says versus what was observed. It is the
// drift input for a handoff; a zero-value observation means no external drift.
type DriftObservation struct {
	// DesiredDigest is the digest the external desired state (Git/Helm/
	// Kustomize) pins. Required: a handoff that claims drift cannot be decided
	// without the external digest.
	DesiredDigest string
	// ObservedDigest is the digest the runtime observation actually reports.
	// Required alongside DesiredDigest.
	ObservedDigest string
	// Evidence is the external proof the drift was read from (a Git revision,
	// an Argo CD sync revision, a Helm release revision). Empty means the
	// digests are asserted without external evidence — never acceptable.
	Evidence string
	// Synced and Healthy are external statuses and must only be set from
	// actual external evidence; a file existing or CI being green is neither.
	Synced  bool
	Healthy bool
}

func (d DriftObservation) validate() error {
	if d.DesiredDigest == "" || d.ObservedDigest == "" {
		return fmt.Errorf("drift observation requires both the desired and observed digests")
	}
	if d.Evidence == "" {
		return fmt.Errorf("drift observation requires external evidence for its digests")
	}
	return nil
}

func (d DriftObservation) drifted() bool {
	return d.DesiredDigest != d.ObservedDigest
}

// HandoffRequest opens a bounded handoff for one action in one delivery mode.
// Mode/Owner/Provider consistency is re-validated here through Selection so a
// handoff can never widen the accepted contract.
type HandoffRequest struct {
	Mode      Mode
	Operation string
	Selection Selection
	// Tracking is the foreign reconciliation metadata currently observed for
	// the resource set. A delegated handoff preserves it; a direct handoff
	// reports it and must never silently drop it.
	Tracking TrackingMetadata
	// Drift is the external desired/observed comparison for the resource set.
	Drift DriftObservation
}

func (r HandoffRequest) Normalize() HandoffRequest {
	out := r
	out.Selection = r.Selection.Normalize()
	out.Mode = Mode(strings.ToLower(strings.TrimSpace(string(r.Mode))))
	if out.Mode == "" {
		out.Mode = out.Selection.Mode
	}
	out.Operation = strings.ToLower(strings.TrimSpace(r.Operation))
	return out
}

func (r HandoffRequest) Validate() error {
	r = r.Normalize()
	if _, ok := handoffActionSet[r.Operation]; !ok {
		return fmt.Errorf("unsupported handoff operation %q", r.Operation)
	}
	if r.Mode != ModeDirect && r.Mode != ModeDelegated {
		return fmt.Errorf("unsupported handoff mode %q", r.Mode)
	}
	if err := r.Selection.Validate(); err != nil {
		return fmt.Errorf("handoff selection invalid: %w", err)
	}
	if r.Mode != r.Selection.Mode {
		return fmt.Errorf("handoff mode %q contradicts selection mode %q", r.Mode, r.Selection.Mode)
	}
	if err := r.Tracking.validate(); err != nil {
		return err
	}
	return nil
}

// Handoff is the rendered outcome of a handoff request: the bounded
// read-model a surface (CLI/JSON/MCP) displays. Action is one of
// observe|noop|refused; it is never a hidden mutation.
type Handoff struct {
	Mode       Mode   `json:"mode"`
	Operation  string `json:"operation"`
	Provider   string `json:"provider"`
	Owner      string `json:"owner"`
	Action     string `json:"action"`
	Drifted    bool   `json:"drifted"`
	Synced     bool   `json:"synced,omitempty"`
	Healthy    bool   `json:"healthy,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
	Provenance string `json:"provenance,omitempty"`
	Revision   string `json:"revision,omitempty"`
	// Tracking is the foreign reconciliation metadata exactly as it was
	// received. Preservation means byte-equal contents on a completed
	// delegated handoff; a refused handoff also carries it unchanged so the
	// operator sees what is at stake.
	Tracking TrackingMetadata `json:"tracking_metadata,omitempty"`
	Reason   string           `json:"reason,omitempty"`
}

// HandOff packages one delivery operation for the selected mode.
//
// Direct mode: BaseHarbor owns reconciliation, the handoff is an apply-plan
// pass-through (observe) and no external drift concept gates it.
//
// Delegated mode: the external reconciler owns reconciliation. The handoff
// preserves the external tracking metadata verbatim, never mutates external
// desired state here, and fails closed on drift: a drifted external resource
// set is refused with an explicit reason. It NEVER silently falls back to a
// direct apply — a fallback would race the external reconciler and contradict
// the one-writer boundary.
func HandOff(req HandoffRequest) (Handoff, error) {
	if err := req.Validate(); err != nil {
		return Handoff{}, err
	}
	req = req.Normalize()
	preserved := req.Tracking.clone()
	out := Handoff{
		Mode:       req.Mode,
		Operation:  req.Operation,
		Provider:   req.Selection.Provider,
		Owner:      string(req.Selection.Owner),
		Provenance: req.Selection.Provenance,
		Revision:   req.Selection.Revision,
		Tracking:   preserved,
		Action:     handoffObserve,
	}
	if req.Mode == ModeDirect {
		return out, nil
	}

	// Delegated: a drift claim must be backed by external evidence. Digests
	// without an external evidence reference are an integrity error, not a
	// "no drift" default.
	drift := req.Drift
	if (drift.DesiredDigest != "" || drift.ObservedDigest != "" || drift.Evidence != "") && drift.Evidence == "" {
		return Handoff{}, fmt.Errorf("delegated handoff cannot assert drift without external evidence")
	}
	if drift.drifted() {
		out.Action = handoffRefused
		out.Drifted = true
		out.Synced = drift.Synced
		out.Healthy = drift.Healthy
		out.Evidence = drift.Evidence
		out.Reason = "external desired state drifted; no silent fallback to direct delivery"
		return out, nil
	}
	out.Synced = drift.Synced
	out.Healthy = drift.Healthy
	out.Evidence = drift.Evidence
	return out, nil
}

// DiffTracking compares two tracking-metadata maps and reports the lost,
// added and changed keys. An empty diff means byte-equal preservation.
func DiffTracking(before, after TrackingMetadata) TrackingDiff {
	var d TrackingDiff
	for key, value := range before {
		next, ok := after[key]
		switch {
		case !ok:
			d.Lost = append(d.Lost, key)
		case next != value:
			d.Changed = append(d.Changed, key)
		}
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			d.Added = append(d.Added, key)
		}
	}
	sort.Strings(d.Lost)
	sort.Strings(d.Added)
	sort.Strings(d.Changed)
	return d
}

// TrackingDiff is the bounded read-model of a metadata preservation check.
type TrackingDiff struct {
	Lost    []string `json:"lost,omitempty"`
	Added   []string `json:"added,omitempty"`
	Changed []string `json:"changed,omitempty"`
}

func (d TrackingDiff) Empty() bool {
	return len(d.Lost) == 0 && len(d.Added) == 0 && len(d.Changed) == 0
}
