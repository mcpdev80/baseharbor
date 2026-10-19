package main

import (
	"errors"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// fixtureCleanupDiagnostic is the only structured surface the authorized
// fixture-cleanup path publishes when destroyInstallation refuses. It records
// exactly three coarse, non-material fields: who called, what ownership owner
// was asserted, and the failure classification. It never includes a hostname,
// path, revision hash, plan digest, deployment ID or any guarded inventory
// material, and it never weakens the ownership/consent guard it observes.
type fixtureCleanupDiagnostic struct {
	Caller       string `json:"caller"`
	Owner        string `json:"owner,omitempty"`
	Classification string `json:"classification"`
}

// nativeFixtureCleanupDiagnostic extracts a bounded, redacted diagnostic from a
// failed destroyInstallation. It surfaces only the caller, the asserted owner
// subject and a coarse failure classification; it never surfaces the
// destruction impact's owned paths, revisions, plan IDs or target/host names.
func nativeFixtureCleanupDiagnostic(err error) *fixtureCleanupDiagnostic {
	if err == nil {
		return nil
	}
	diagnostic := &fixtureCleanupDiagnostic{
		Caller:       "native-fixture-cleanup",
		Classification: classificationOfDestroyRefusal(err),
		Owner:        ownershipSubject(err),
	}
	if diagnostic.Caller == "" {
		diagnostic.Caller = "native-fixture-cleanup"
	}
	if diagnostic.Classification == "" {
		diagnostic.Classification = "destroy-refused"
	}
	return diagnostic
}

// classificationOfDestroyRefusal maps a guarded refusal to a coarse,
// non-material category such as ownership or consent. It never echoes the
// destruction impact contents, only the machine error classification.
func classificationOfDestroyRefusal(err error) string {
	var failure *machine.Error
	if !errors.As(err, &failure) {
		return ""
	}
	switch failure.Code {
	case machine.ErrorOwnershipAmbiguous:
		return "ownership-ambiguous"
	case machine.ErrorPolicyDenied:
		return "policy-denied"
	case machine.ErrorAuthenticationFailed:
		return "authentication-required"
	case machine.ErrorValidationFailed:
		return "validation-failed"
	default:
		return "destroy-refused"
	}
}

// ownershipSubject reports only the coarse owner subject of the destroy scope
// when it is available on the failure details; it never returns a path,
// revision, plan digest, deployment ID or target/host name.
func ownershipSubject(err error) string {
	var failure *machine.Error
	if !errors.As(err, &failure) {
		return ""
	}
	switch current := failure.Details.(type) {
	case runtimeDestroyImpact:
		if current.Scope.Owner.Subject != "" {
			return current.Scope.Owner.Subject
		}
		if current.Scope.Owner.Owner != "" {
			return current.Scope.Owner.Owner
		}
	case applicationDestructionPlan:
		if current.Scope.Owner.Subject != "" {
			return current.Scope.Owner.Subject
		}
		if current.Scope.Owner.Owner != "" {
			return current.Scope.Owner.Owner
		}
	}
	return ""
}
