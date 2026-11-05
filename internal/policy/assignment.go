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
	"fmt"
	"strconv"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// ErrAssignmentUnknown fails closed when the adapter is asked to resolve a
// profile that the bound manager does not own or that was never created, or an
// empty manager.
var ErrAssignmentUnknown = errAssignmentUnknown

var errAssignmentUnknown = policyAssignmentError("policy: unknown profile assignment")

// errForeignTenant fails closed when the adapter is asked to resolve a profile
// from a tenant (installation identity) other than the one that owns the bound
// Manager. A foreign installation cannot resolve or exercise another
// installation's profile.
var errForeignTenant = policyAssignmentError("policy: foreign tenant access profile")

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

// Resolve reports whether the named profile — as seen through the one owning
// orgconfig.Manager (the single authority) — assigns the given operation, and
// attributes the call to the caller's installation identity (tenant). It does
// not grant: it only reports membership in the assigned set. Anything the
// profile does not assign is denied, never warned, so a missing assignment
// cannot silently widen access.
//
// Resolve is read-only and holds no decision authority of its own. It returns a
// Finding for the caller to fold into the single production decision path
// (policy.Aggregate); it never returns a bare Allow/Deny authority. A call from
// a foreign installation identity (one whose Manager does not own the profile)
// fails closed. A revoked or unknown profile fails closed on
// ErrAssignmentUnknown. The server rechecks the actor and the selected Target
// at commit time.
//
// Resolve must not be used as a second decision surface: the result feeds one
// authority, never decides alone.
func (a *Adapter) Resolve(profileName, tenant string, operation orgconfig.Operation) (Finding, error) {
	if a.manager == nil {
		return Finding{}, errAssignmentUnknown
	}
	// Tenant/actor binding: the profile must exist on THIS manager (this
	// installation identity). A call from a foreign owner cannot resolve a
	// profile that belongs to another installation.
	if tenant != "" && a.manager.Owner != tenant {
		return Finding{}, errForeignTenant
	}
	profile, err := a.manager.Get(profileName)
	if err != nil {
		// Revoked (removed) or unknown profile fails closed through this adapter.
		return Finding{}, errAssignmentUnknown
	}
	finding := Finding{
		Code:     "access-profile",
		Subject:  profileName,
		Scope:    "profile",
		Message:  fmt.Sprintf("access profile %q by tenant %q", profileName, tenant),
		Source:   SourceDefault,
		Revision: strconv.FormatUint(profile.Revision, 10),
	}
	for _, op := range profile.Operations {
		if op == operation {
			finding.Decision = Allow
			return finding, nil
		}
	}
	finding.Decision = Deny
	return finding, nil
}

// ResolveFinding folds one adapter resolution into the single production policy
// authority: it aggregates the resolved Finding with the other mandatory
// findings and returns the terminal decision. A Deny finding (foreign tenant,
// revoked/unknown profile, unassigned operation) forces the aggregate to Deny;
// an assignment that is Allow does not by itself widen any other mandatory
// deny. This is the only production surface the adapter owns: it contributes a
// constrained finding to the one authority, it never decides alone.
func ResolveFinding(other []Finding, finding Finding) Decision {
	return Aggregate(append(other, finding))
}
