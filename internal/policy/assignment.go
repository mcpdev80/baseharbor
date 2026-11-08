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
	"errors"
	"fmt"
	"strconv"
	"strings"

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

// Resolve reports whether the named profile on the given Target — as seen
// through the one owning orgconfig.Manager (the single authority) — assigns the
// given operation, and attributes the call to the caller's installation
// identity (tenant). It does not grant: it only reports membership in the
// assigned set. Anything the profile does not assign is denied, never warned,
// so a missing assignment cannot silently widen access.
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
func (a *Adapter) Resolve(profileName, target, tenant string, operation orgconfig.Operation) (Finding, error) {
	if a.manager == nil {
		return Finding{}, errAssignmentUnknown
	}
	// Tenant/actor binding: the profile must exist on THIS manager (this
	// installation identity). A call from a foreign owner cannot resolve a
	// profile that belongs to another installation.
	if tenant != "" && a.manager.Owner != tenant {
		return Finding{}, errForeignTenant
	}
	profile, err := a.manager.GetByKey(profileName, target)
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

// ResolveAll folds every adapter resolution over the owned profile set into the
// single production policy authority. It drives the production path with a
// real, owned profile and the actual operation rather than a hardcoded verb:
// for each incoming finding it resolves the owning Manager (built from the
// active configuration's owned profiles) against the finding's operation and
// folds the result. A Deny — foreign tenant, revoked/unknown profile, or
// unassigned operation — forces the aggregate to Deny; an Allow does not widen
// any other mandatory deny. This is the only production surface the adapter
// owns: it contributes constrained findings to the one authority, it never
// decides alone, and it never silently drops a resolution.
//
// resolveOperation maps the call to a machine operation via a real owning
// Manager; a nil operation means no real signal was available and the caller
// must fail closed, which ResolveAll does by folding that fold to a Deny
// rather than a default Allow. resolveLoad supplies the active configuration
// so the contract holds where no disk-backed LoadActive is present.
func ResolveAll(other []Finding, resolveOperation func(Finding) (orgconfig.Operation, error), resolveLoad func() (orgconfig.ActiveState, error)) Decision {
	return Aggregate(append(other, profileFold(resolveOperation, resolveLoad)...))
}

// ErrNoActiveOrganization fails closed when the active organization configuration
// has no organization to resolve an owned profile for.
var ErrNoActiveOrganization = errors.New("policy: no active organization to resolve owned profile")

// ErrNoConcreteOperation fails closed when no concrete machine operation is
// bound for the production policy path, so the aggregate cannot default to a
// hardcoded verb.
var ErrNoConcreteOperation = errors.New("policy: no concrete machine operation bound; fail closed on profile resolution")

// profileFold resolves the owning Manager against the active configuration's
// owned profiles once and returns the single constrained Finding it contributes
// to the aggregate. It fails closed: an adapter error, a foreign tenant, an
// unknown/revoked profile or an unassigned operation all fold to a Deny finding
// so the mandatory aggregate cannot default to Allow.
func profileFold(resolveOperation func(Finding) (orgconfig.Operation, error), resolveLoad func() (orgconfig.ActiveState, error)) []Finding {
	// The resolver's error is the contract's fail-closed signal: no concrete
	// operation bound means the caller cannot positively assign and the
	// aggregate must fail closed. Check it before any disk-backed LoadActive
	// so the contract holds even where no active configuration is present.
	operation, opErr := resolveOperation(Finding{})
	if opErr != nil || operation == "" {
		return []Finding{profileDeny("no concrete machine operation bound: fail closed on profile resolution")}
	}
	state, err := resolveLoad()
	if err != nil {
		return []Finding{profileDeny("no active organization: cannot resolve owned profile")}
	}
	org := strings.TrimSpace(state.Config.Organization)
	if org == "" {
		return nil
	}
	manager := orgconfig.ManagerFromActive(state)
	adapter := NewAdapter(manager)
	// An active organization with no owned profile that positively assigns the
	// operation fails closed: holding an unassigned profile confers no right,
	// so no positive assignment folds Allow.
	if manager.Count() == 0 {
		return []Finding{profileDeny("no owned profile assigns the resolved operation")}
	}
	// Resolve the operation against every owned profile and fold the results:
	// an assigned operation folds Allow, an unassigned/unknown/revoked
	// operation folds Deny. The aggregate takes the worst case, so a single
	// unassigned profile forces Deny while a positive assignment never widens
	// a mandatory deny. This drives the production path with real owned
	// profiles rather than a single hardcoded name.
	var fold []Finding
	for _, profile := range manager.List() {
		finding, rErr := adapter.Resolve(profile.Name, profile.Target, org, operation)
		if rErr != nil {
			return []Finding{profileDeny(rErr.Error())}
		}
		fold = append(fold, finding)
	}
	return fold
}

func profileDeny(message string) Finding {
	return Finding{
		Code:     "access-profile",
		Scope:    "profile",
		Decision: Deny,
		Source:   SourceDefault,
		Message:  message,
	}
}
