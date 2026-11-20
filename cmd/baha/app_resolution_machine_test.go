package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestWrappedResolutionErrorsShareExistingMachineContracts(t *testing.T) {
	for _, test := range []struct {
		code     string
		expected machine.ErrorCode
	}{
		{"missing", machine.ErrorNotFound}, {"missing-dependency", machine.ErrorNotFound},
		{"incompatible", machine.ErrorValidationFailed}, {"dependency-cycle", machine.ErrorValidationFailed},
		{"ambiguous", machine.ErrorConflict}, {"binding-conflict", machine.ErrorConflict},
		{"foreign", machine.ErrorOwnershipAmbiguous}, {"foreign-target", machine.ErrorOwnershipAmbiguous},
		{"unrecognized", machine.ErrorInternal},
	} {
		t.Run(test.code, func(t *testing.T) {
			domain := &application.ResolutionError{Code: test.code, InstanceID: "test-provider"}
			mapped := classifyApplicationResolutionError(fmt.Errorf("existing provider plan: %w", domain))
			for _, surface := range []error{mapped, classifyMachineCLIError(mapped)} {
				failure := machine.Classify(surface)
				if failure.Code != test.expected || failure.Resource != "test-provider" || failure.CauseCode == "" || failure.Next == "" {
					t.Fatalf("machine drift: %+v", failure)
				}
				var cause *application.ResolutionError
				if !errors.As(surface, &cause) || cause != domain {
					t.Fatal("domain cause lost")
				}
				if cli.ExitCode(surface) != cli.ExitCode(domain) {
					t.Fatal("existing exit mapping changed")
				}
			}
		})
	}
	existing := &machine.Error{Code: machine.ErrorPolicyDenied, Cause: &application.ResolutionError{Code: "missing"}}
	if classifyApplicationResolutionError(existing) != existing {
		t.Fatal("existing envelope overridden")
	}
	unknown := errors.New("unexpected native adapter error")
	if classifyApplicationResolutionError(unknown) != unknown || machine.Classify(unknown).Code != machine.ErrorInternal {
		t.Fatal("unexpected error semantics changed")
	}
}

func TestResolutionPlanFailureReportsActualConsumerAndEdge(t *testing.T) {
	domain := &application.ResolutionError{
		Code: "binding-conflict", Consumer: "application/app-id/dev", InstanceID: "postgresql/application/app-id/dev/default",
		Requirement: capability.Requirement{Kind: capability.SQL, Name: "default"},
	}
	mapped := classifyApplicationResolutionError(fmt.Errorf("existing provider plan: %w", domain))
	failure := machine.Classify(mapped)
	if failure.Code != machine.ErrorConflict || failure.CauseCode != "provider_resolution_binding_conflict" {
		t.Fatalf("binding conflict was mislabeled as stale consumer dependency: %+v", failure)
	}
	diagnostic, ok := failure.Details.(applicationResolutionDiagnostic)
	if !ok || diagnostic.Code != domain.Code || diagnostic.Consumer != domain.Consumer || diagnostic.InstanceID != domain.InstanceID || diagnostic.Requirement != "database.sql/default" {
		t.Fatalf("resolution edge details lost: %#v", failure.Details)
	}
	proposal := appMachineProposalAuthorityFailure("plan", appMachineProposalStage("resolve-provider-plan", mapped))
	if !strings.Contains(proposal.Error(), "resolution=binding-conflict") || !strings.Contains(proposal.Error(), domain.Consumer) || !strings.Contains(proposal.Error(), domain.InstanceID) {
		t.Fatalf("safe resolver diagnostic absent from proposal error: %v", proposal)
	}
}
