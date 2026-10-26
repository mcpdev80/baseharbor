package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
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
