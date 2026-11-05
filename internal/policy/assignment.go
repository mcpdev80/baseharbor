// Package policy holds the BaseHarbor policy decision model: findings, rules
// and the aggregate decision used by the effective policy for an application
// environment.
//
// assignment.go adds the bounded adapter this card OWNS: it turns an accepted,
// validated orgconfig team access profile into a policy decision for a single
// machine operation. The adapter does not grant: it only reports whether an
// operation falls inside the profile's assigned set. Anything the profile does
// not assign is denied, never warned, so a missing assignment cannot silently
// widen access.
package policy

import (
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// ErrAssignmentUnknown fails closed when the adapter is asked to resolve a
// profile that the bound manager does not own or that was never created, or an
// empty manager.
var ErrAssignmentUnknown = errAssignmentUnknown

var errAssignmentUnknown = policyAssignmentError("policy: unknown profile assignment")

// policyAssignmentError is a minimal sentinel error so the assignment layer's
// sentinels are private and cannot be reconstructed by an external package.
type policyAssignmentError string

func (e policyAssignmentError) Error() string { return string(e) }

// Adapter resolves machine operations against an accepted orgconfig team access
// profile. It holds a reference to the owning Manager so it can look up the
// profile by name; it never mutates it.
type Adapter struct {
	manager *orgconfig.Manager
}

// NewAdapter builds an assignment Adapter over one owning Manager. A nil
// manager yields an Adapter that always fails closed on Resolve.
func NewAdapter(m *orgconfig.Manager) *Adapter {
	return &Adapter{manager: m}
}

// Resolve reports whether the given operation falls inside the profile's
// assigned operation set. It returns the aggregated policy decision for the
// operation: Allow when the operation is explicitly assigned, Deny when the
// operation is outside the assignment (a missing assignment is Deny, never a
// warning). An unknown profile or empty manager fails closed on
// ErrAssignmentUnknown.
//
// Resolve is read-only: it never mutates the profile or the manager and it
// confers no right; the server rechecks the actor and the selected Target at
// commit time.
func (a *Adapter) Resolve(profileName string, operation orgconfig.Operation) (Decision, error) {
	if a.manager == nil {
		return Deny, errAssignmentUnknown
	}
	profile, err := a.manager.Get(profileName)
	if err != nil {
		return Deny, err
	}
	for _, op := range profile.Operations {
		if op == operation {
			return Allow, nil
		}
	}
	return Deny, nil
}
