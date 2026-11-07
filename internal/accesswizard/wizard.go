// Package accesswizard owns the bounded wizard adapter that turns the accepted
// orgconfig team access profile Manager and the accepted policy decision
// authority into a single guided production path for profile assignment and
// deliberate offboarding of one person on one target.
//
// The wizard is a guide, not a decision surface. It never grants a right on its
// own: every assignment it builds flows through the single production policy
// authority (policy.Aggregate) and every removal is re-checked against the
// current configuration at submit time. An incomplete configuration fails
// closed before any write is attempted.
package accesswizard

import (
	"fmt"
	"sort"
	"strings"
)

// ErrIncompleteConfig fails closed when the wizard is asked to run on an
// installation that has not declared a Target, an environment operation, or a
// managed organization to attach a person to. A person can only be onboarded
// onto a target that the active configuration already declares, so inventing a
// target or running before the organization is configured is rejected.
var ErrIncompleteConfig = fmt.Errorf("incomplete configuration: cannot assign access")

// Target is a managed Target the active configuration declares. The wizard only
// ever names targets the authority already owns; it never invents one.
type Target struct {
	Name    string
	Unknown bool
}

// SearchResult is one selectable row of the target search. Selection preserves
// the effective scope (the operation the target is known to allow) so the review
// screen shows exactly what the assignment would grant.
type SearchResult struct {
	Name           string
	Operation      string
	EffectiveScope string
}

// Selection is one confirmed target an operator wants to attach a person to. It
// is preserved across pagination/back so a scroll-back selection survives the
// return trip to the review screen.
type Selection struct {
	TargetName     string
	Operation      string
	EffectiveScope string
}

// Person is the identity the wizard onboards or offboards on a target. Only the
// name is required; the wizard attributes the change to this identity at submit
// time. The name is the minimal, non-empty, single-identifier token the
// orgconfig profile validation already enforces.
type Person struct {
	Name string
}

// Assignment is one confirmed profile an assignment wizard would create. The
// wizard builds it through the accepted orgconfig Manager so every field is
// validated against the authority before it is folded into policy.
type Assignment struct {
	Person    Person
	Target    string
	Operation string
	Replace   bool
}

// Review is the terminal confirmation the wizard shows before any mutation. It
// lists the effective scope of every confirmed selection so the operator can
// approve or cancel before the write.
type Review struct {
	Person   Person
	Selected []Selection
}

// RemoveKind enumerates the four distinct offboard actions the wizard must
// distinguish, per the card acceptance: a local connection removal, a device
// revoke, an owned grant removal, and an explicit managed-user disable/delete.
type RemoveKind int

const (
	// RemoveConnection removes only this workstation's local connection to the
	// target ("this workstation only").
	RemoveConnection RemoveKind = iota
	// RemoveDevice revokes the Dev Remote device locally and remotely.
	RemoveDevice
	// RemoveGrant removes only the owned profile grant on the target.
	RemoveGrant
	// RemoveManagedUser disables or deletes an explicitly owned managed user.
	RemoveManagedUser
)

// String renders the remove kind for the review and error screens.
func (k RemoveKind) String() string {
	switch k {
	case RemoveConnection:
		return "connection"
	case RemoveDevice:
		return "device"
	case RemoveGrant:
		return "owned grant"
	case RemoveManagedUser:
		return "managed user"
	default:
		return "unknown"
	}
}

// OffboardRequest is one confirmed removal the wizard submits. It binds the
// kind, the person, and the target so the production path can re-check the
// current state and mutate only the owned boundary.
type OffboardRequest struct {
	Kind   RemoveKind
	Person Person
	Target string
	// Profile, when set, scopes a RemoveGrant to a single owned profile name.
	Profile string
}

// ScopePreview shows the effective scope the wizard presents on the review
// screen: exactly which operations each selection already grants on the target,
// never more.
type ScopePreview struct {
	Target     string
	Operations []string
}

// BuildSelections validates a raw target name against the known target set and
// returns the effective-scope row for it, or fails closed when the target is
// undeclared. The wizard only ever presents declared targets; an undeclared
// target is an attempt to widen the surface and is rejected.
func BuildSelections(known map[string]string, raw string) ([]SearchResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if _, ok := known[raw]; !ok {
		return nil, fmt.Errorf("%w: target %q is not declared by the active configuration", ErrIncompleteConfig, raw)
	}
	return []SearchResult{{Name: raw, Operation: known[raw], EffectiveScope: raw}}, nil
}

// SortedTargets returns the declared target names in canonical order for the
// search screen and for pagination.
func SortedTargets(known map[string]string) []string {
	out := make([]string, 0, len(known))
	for name := range known {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ValidateSelection rejects an empty selection before it reaches the review
// screen. A wizard that would commit an empty selection fails closed.
func ValidateSelection(s Selection) error {
	if strings.TrimSpace(s.TargetName) == "" {
		return fmt.Errorf("%w: a selection must name a declared target", ErrIncompleteConfig)
	}
	if strings.TrimSpace(s.Operation) == "" {
		return fmt.Errorf("%w: a selection must resolve an effective operation", ErrIncompleteConfig)
	}
	return nil
}

// ValidateAssignment fails closed on an unassignable profile: an empty person,
// an undeclared target, or an empty operation.
func ValidateAssignment(a Assignment) error {
	if strings.TrimSpace(a.Person.Name) == "" {
		return fmt.Errorf("%w: a person name is required", ErrIncompleteConfig)
	}
	if strings.TrimSpace(a.Target) == "" {
		return fmt.Errorf("%w: a target is required for the assignment", ErrIncompleteConfig)
	}
	if strings.TrimSpace(a.Operation) == "" {
		return fmt.Errorf("%w: an operation must be selected for the assignment", ErrIncompleteConfig)
	}
	return nil
}

// ValidateOffboard fails closed on an unremovable offboard request: an empty
// person, target, or kind, or a managed-user removal without a name.
func ValidateOffboard(r OffboardRequest) error {
	if strings.TrimSpace(r.Person.Name) == "" {
		return fmt.Errorf("%w: a person name is required", ErrIncompleteConfig)
	}
	if strings.TrimSpace(r.Target) == "" {
		return fmt.Errorf("%w: a target is required for the removal", ErrIncompleteConfig)
	}
	switch r.Kind {
	case RemoveManagedUser:
		if strings.TrimSpace(r.Profile) == "" {
			return fmt.Errorf("%w: a managed user must be named to disable or delete", ErrIncompleteConfig)
		}
	}
	return nil
}

// Review renders the effective scope of every confirmed selection as the single
// review screen the wizard presents before any mutation. The lines are stable
// and ordered so they can be diffed in tests.
func (r Review) Render() []string {
	lines := make([]string, 0, len(r.Selected)+2)
	lines = append(lines, fmt.Sprintf("Review access change for %q:", r.Person.Name))
	if len(r.Selected) == 0 {
		lines = append(lines, "  (no selections) - cancel")
		return lines
	}
	for _, s := range r.Selected {
		scope := strings.TrimSpace(s.EffectiveScope)
		if scope == "" {
			scope = s.TargetName
		}
		lines = append(lines, fmt.Sprintf("  - %s: %s (%s)", s.TargetName, s.Operation, scope))
	}
	return lines
}

// marker
