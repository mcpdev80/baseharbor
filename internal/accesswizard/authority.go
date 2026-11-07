package accesswizard

import (
	"strings"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
	"github.com/mcpdev80/baseharbor/internal/policy"
)

// ErrDenied is the terminal failure the wizard reports when the single
// production policy authority Denies an assignment or removal. It is never a
// bare "access denied"; it carries the resolved reason so the operator can act
// on it.
var ErrDenied = errDenied

var errDenied = &wizardError{"access denied by the production policy authority"}

// wizardError is a minimal presentation marker so the CLI can render the reason
// once and exit without a second generic error block.
type wizardError struct{ message string }

func (e *wizardError) Error() string { return e.message }

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
// resolveLoad supplies the active configuration so the production path runs with
// a real, owned profile rather than a hardcoded name. A nil manager yields a
// Deny (fail closed): a Manager that was never built from an active state is not
// a valid assignment surface.
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
	finding, err := adapter.Resolve(profile.Name, manager.Owner, op)
	if err != nil {
		return policy.Deny, &wizardError{message: err.Error()}
	}
	return policy.ResolveFinding(nil, finding), nil
}

// RevokeAssignment removes the requested person's profile on the reviewed Target\n// through the accepted
// orgconfig Manager and reports the result through the single production policy
// authority. A revoked or unknown profile fails closed through the accepted
// policy Adapter. The removal mutates only the owned profile set; it never
// touches a foreign grant.
func RevokeAssignment(manager *orgconfig.Manager, person, target string) error {
	if manager == nil || strings.TrimSpace(person) == "" || strings.TrimSpace(target) == "" {
		return ErrDenied
	}
	// The CLI selects a Target; Manager.Revoke accepts a profile name.
	// The current manager indexes profiles by person. Never use a selected
	// Target as a profile key or remove a profile belonging to another Target.
	profile, err := manager.Get(strings.TrimSpace(person))
	if err != nil {
		return &wizardError{message: err.Error()}
	}
	if profile.Name != person || profile.Target != target {
		return ErrDenied
	}
	if _, err := manager.Revoke(profile.Name); err != nil {
		return &wizardError{message: err.Error()}
	}
	return nil
}
