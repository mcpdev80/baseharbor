package deployment

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Reference dimension kinds. Exactly these three keys may appear in
// DeploymentRecord.References. They are independent dimensions; a mutation to one
// never rewrites another.
const (
	ReferenceIntended = "intended"
	ReferenceApplied  = "applied"
	ReferenceObserved = "observed"
)

func validReferenceKind(kind string) bool {
	switch kind {
	case ReferenceIntended, ReferenceApplied, ReferenceObserved:
		return true
	default:
		return false
	}
}

// validateReferences rejects unknown kinds and duplicate kinds within a single
// record. The isolation contract is enforced by callers writing only their own
// dimension, not by this structural check.
func validateReferences(references map[string]*ReferenceDimension) error {
	for kind, dim := range references {
		if !validReferenceKind(kind) {
			return fmt.Errorf("invalid reference kind %q", kind)
		}
		if dim == nil {
			return fmt.Errorf("reference %q is nil", kind)
		}
	}
	return nil
}

// ReferenceDimension is the release/component reference recorded for one point
// in a deployment's lifecycle. Staging isolates the three dimensions so a change
// to one never rewrites another: an intent edit keeps the last-applied reference,
// and a fresh observation keeps both the intended and the last-applied
// references.
type ReferenceDimension struct {
	// Kind distinguishes the dimension: "intended" (desired), "applied"
	// (last-applied), and "observed" (what the runtime currently reports).
	Kind string `json:"kind"`
	// Release is the resolved release reference (image tag/digest, source
	// revision, or a scoped fingerprint). Git revision may be unknown for
	// image-only inputs; a digest is preferred when evidenced.
	Release string `json:"release,omitempty"`
	// Component maps a workload/ service name to its reference, when the
	// deployment realizes more than one component.
	Component map[string]string `json:"component,omitempty"`
	// At is when the reference was recorded.
	At time.Time `json:"at,omitempty"`
}

// ReferenceDimensions indexes reference dimensions by their kind.
type ReferenceDimensions map[string]*ReferenceDimension

// AppliedReference returns the last-applied release reference, or "" when none
// has been recorded.
func (refs ReferenceDimensions) AppliedReference() string {
	if refs == nil {
		return ""
	}
	if dim, ok := refs[ReferenceApplied]; ok && dim != nil {
		return strings.TrimSpace(dim.Release)
	}
	return ""
}

// Stale reports whether the observed dimension diverges from the last-applied
// dimension. It is an explicit, fail-closed staleness operation: an unrecorded
// observed reference (no observation yet) is reported as stale so callers cannot
// treat "no observation" as "up to date".
func (r DeploymentRecord) Stale() error {
	observed, ok := r.References[ReferenceObserved]
	if !ok || observed == nil || strings.TrimSpace(observed.Release) == "" {
		return &DeploymentRecordStateError{Identity: r.Identity, Kind: "stale", Err: errors.New("no observed reference recorded; cannot confirm up-to-date")}
	}
	applied := r.References[ReferenceApplied]
	if applied == nil || strings.TrimSpace(applied.Release) == "" {
		// Without a last-applied reference there is nothing to be stale against;
		// the observed reference stands on its own.
		return nil
	}
	if strings.TrimSpace(observed.Release) != strings.TrimSpace(applied.Release) {
		return &DeploymentRecordStateError{Identity: r.Identity, Kind: "stale", Err: errors.New("observed reference diverges from last-applied reference")}
	}
	return nil
}

// SetReference writes exactly one reference dimension in isolation: it rewrites
// only the named dimension and leaves the intended, applied and observed
// dimensions untouched. This is the isolation contract that makes staging a real
// extension of the deployment state rather than a presenter — one mutation never
// rewrites another dimension.
func SetReference(id DeploymentIdentity, kind string, dimension ReferenceDimension) error {
	if !validReferenceKind(kind) {
		return fmt.Errorf("invalid reference kind %q", kind)
	}
	dimension.Kind = kind
	dimension.At = time.Now().UTC()
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		return err
	}
	if record.References == nil {
		record.References = map[string]*ReferenceDimension{}
	}
	// Writing one dimension never clears or copies another.
	record.References[kind] = &dimension
	return SaveDeploymentRecord(record)
}

// ClearReference removes one dimension while preserving the other two.
func ClearReference(id DeploymentIdentity, kind string) error {
	if !validReferenceKind(kind) {
		return fmt.Errorf("invalid reference kind %q", kind)
	}
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		return err
	}
	if _, ok := record.References[kind]; !ok {
		// Nothing to clear; still valid.
		return nil
	}
	delete(record.References, kind)
	if len(record.References) == 0 {
		record.References = nil
	}
	return SaveDeploymentRecord(record)
}
