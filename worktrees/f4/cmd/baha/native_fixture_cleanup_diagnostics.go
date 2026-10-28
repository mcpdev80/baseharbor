package main

import (
	"errors"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Cleanup diagnostics contain only fixed caller, owner-category and failure
// classifications. They never include identities or impact material.
type fixtureCleanupDiagnostic struct {
	Caller         string `json:"caller"`
	Owner          string `json:"owner,omitempty"`
	Classification string `json:"classification"`
}

func nativeFixtureCleanupDiagnostic(err error) *fixtureCleanupDiagnostic {
	if err == nil {
		return nil
	}
	classification := classificationOfDestroyRefusal(err)
	if classification == "" {
		classification = "destroy-refused"
	}
	return &fixtureCleanupDiagnostic{
		Caller:         "native-fixture-cleanup",
		Owner:          fixtureCleanupOwnerCategory(err),
		Classification: classification,
	}
}

func classificationOfDestroyRefusal(err error) string {
	var failure *machine.Error
	if !errors.As(err, &failure) || failure == nil {
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

// The plan type identifies a category; its owner fields remain private.
func fixtureCleanupOwnerCategory(err error) string {
	var failure *machine.Error
	if !errors.As(err, &failure) || failure == nil {
		return ""
	}
	switch failure.Details.(type) {
	case runtimeDestroyImpact:
		return "installation"
	case applicationDestructionPlan:
		return "application-deployment"
	default:
		return ""
	}
}
