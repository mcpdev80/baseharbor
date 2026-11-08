package accesswizard

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
	"github.com/mcpdev80/baseharbor/internal/policy"
)

// ErrDenied is the terminal failure the wizard reports when the single
// production policy authority Denies an assignment or removal. It is never a
// bare "access denied"; it carries the resolved reason so the operator can act
// on it.
var ErrDenied = errDenied

var errDenied = &wizardError{message: "access denied by the production policy authority"}

// wizardError is a minimal presentation marker so the CLI can render the reason
// once and exit without a second generic error block. The optional wrap keeps
// the sentinel ErrDenied on the error chain so callers can errors.Is it even
// when a human-readable reason is attached.
type wizardError struct {
	message string
	wrap    error
}

func (e *wizardError) Error() string { return e.message }

func (e *wizardError) Unwrap() error { return e.wrap }

// ResolveOperation maps the wizard's selected operation verb to the concrete
// machine operation the production policy path resolves against. It returns the
// orgconfig.Operation the operator selected, validated against the fixed
// operation domain, so an unknown verb cannot widen the surface.
func ResolveOperation(operation string) (orgconfig.Operation, error) {
	op := strings.TrimSpace(operation)
	for _, valid := range orgconfig.ValidOperations {
		if string(valid) == op {
			return valid, nil
		}
	}
	return "", ErrDenied
}

// ResolveAssignment folds one confirmed profile assignment into the single
// production policy authority. It builds the assignment through the accepted
// orgconfig Manager (so the definition is validated against the authority),
// then reports the membership of the resolved operation through the accepted
// policy Adapter and folds it into Aggregate. The wizard never grants on its
// own: an unassigned operation, an unknown/revoked profile, or a foreign tenant
// all fold to a Deny, exactly as the accepted predecessor does.
//
// ResolveAssignment mutates the in-memory Manager and validates the decision but
// does NOT persist: the batch caller validates every selection first, then
// commits the whole set atomically through CommitActive (so one failure leaves
// nothing written). A nil manager yields a Deny (fail closed): a Manager that
// was never built from an active state is not a valid assignment surface.
func ResolveAssignment(manager *orgconfig.Manager, person, target, operation string, replace bool) (policy.Decision, error) {
	if manager == nil {
		return policy.Deny, ErrDenied
	}
	op, err := ResolveOperation(operation)
	if err != nil {
		return policy.Deny, err
	}
	profile := orgconfig.Profile{
		Name:       strings.TrimSpace(person),
		Target:     strings.TrimSpace(target),
		Operations: []orgconfig.Operation{op},
	}
	if _, err := manager.Create(profile, replace); err != nil {
		// A duplicate with a differing definition, an undeclared scope or a
		// policy violation all fail closed through the accepted Manager.
		return policy.Deny, &wizardError{message: err.Error()}
	}
	adapter := policy.NewAdapter(manager)
	finding, err := adapter.Resolve(profile.Name, profile.Target, manager.Owner, op)
	if err != nil {
		return policy.Deny, &wizardError{message: err.Error()}
	}
	return policy.ResolveFinding(nil, finding), nil
}

// RevokeAssignment removes the requested person's profile on the reviewed Target
// through the accepted
// orgconfig Manager and reports the result through the single production policy
// authority. A revoked or unknown profile fails closed through the accepted
// policy Adapter. The removal mutates only the owned profile set; it never
// touches a foreign grant.
//
// RevokeAssignment mutates the in-memory Manager and validates the removal but
// does NOT persist: the batch caller validates every selection first, then
// commits the whole set atomically through CommitActive (so one failure leaves
// nothing written). The manager indexes profiles by (name, target); the profile
// on a different Target for the same Name is a distinct profile and is never
// touched by this revoke.
//
// kind selects which removal the caller intends. Only the managed-user revoke is
// implemented on this card: RevokeAssignment fails closed for every other
// RemoveKind (connection, device, owned-grant) so an unimplemented removal never
// mutates a managed-user profile by accident — an unimplemented kind must never
// be silently served by the first implemented kind. The full production routes
// for those kinds belong on their own cards (managed-users, device-lifecycle);
// this wizard only ever serves RemoveManagedUser and fails closed otherwise.
func RevokeAssignment(manager *orgconfig.Manager, person, target string, kind RemoveKind) error {
	if kind == RemoveManagedUser {
		if manager == nil || strings.TrimSpace(person) == "" || strings.TrimSpace(target) == "" {
			return ErrDenied
		}
		// The wizard resolves the profile on the exact reviewed (person, target):
		// a profile the same Name holds on another Target is a distinct profile
		// and must not be revoked. GetByKey fails closed when no owned profile
		// exists on this Target.
		if _, err := manager.GetByKey(strings.TrimSpace(person), strings.TrimSpace(target)); err != nil {
			return &wizardError{message: err.Error()}
		}
		if _, err := manager.Revoke(person, target); err != nil {
			return &wizardError{message: err.Error()}
		}
		return nil
	}
	// Any non-managed-user removal kind has no production route on this card yet:
	// fail closed with a clear rejection rather than serving it silently as the
	// managed-user revoke (finding #4: no silent first-kind change).
	return &wizardError{
		message: fmt.Errorf("%w: %q removal is not implemented by this wizard; it is served on its own card", ErrDenied, kind).Error(),
		wrap:    ErrDenied,
	}
}

// CommitActive folds the Manager's owned profile set back into the loaded active
// state and writes it through the production active-state writer. The batch CLI
// caller calls it once after every selection has validated in memory, so the
// whole set persists atomically: a write failure fails closed and nothing is
// written, leaving the store byte-identical.
func CommitActive(manager *orgconfig.Manager, resolveLoad func() (orgconfig.ActiveState, error)) error {
	if resolveLoad == nil {
		return fmt.Errorf("%w: no active-state writer bound", ErrDenied)
	}
	state, loadErr := resolveLoad()
	if loadErr != nil {
		return &wizardError{message: loadErr.Error()}
	}
	if persistErr := orgconfig.PersistActive(manager, state); persistErr != nil {
		return &wizardError{message: persistErr.Error()}
	}
	return nil
}
