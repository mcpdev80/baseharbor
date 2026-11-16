package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestCoreHTTPApplyRejectsUnboundRemoteSourceBeforeMutation(t *testing.T) {
	scope := machine.OperationContext{Target: "remote-docker", Application: "demo", Environment: "dev"}
	for _, tc := range []struct {
		name        string
		application string
		ref         string
		want        machine.ErrorCode
	}{
		{"valid-immutable-without-registered-Core-target", "demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ""},
		{"mutable-branch", "demo", "main", machine.ErrorValidationFailed},
		{"foreign-application", "foreign", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", machine.ErrorValidationFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := development.SourceModel{
				SchemaVersion: development.SourceModelVersion, Application: tc.application,
				Sources: []development.SourceDefinition{{
					ID: "app", Type: development.SourceRepository,
					Repository: "https://git.example/demo.git", Ref: tc.ref,
				}},
				Components: []development.ComponentSource{{Component: "app", Source: "app"}},
			}
			raw, err := json.Marshal(machineApplyInput{
				Target: "remote-docker", Name: "demo", Environment: "dev", SourceModel: &model,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = (&bahaMachineExecutor{}).executeHTTPApply(context.Background(), scope, raw, nil)
			if err == nil || (tc.want != "" && machine.Classify(err).Code != tc.want) {
				t.Fatalf("remote source guard = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestCoreHTTPApplyStageErrorPreservesMachineCodeAndStage(t *testing.T) {
	original := &machine.Error{
		Code:      machine.ErrorCapabilityMissing,
		CauseCode: "application_initialization_required",
		Message:   "Application identity has not been initialized.",
		Next:      "Initialize the application first.",
	}
	got := machine.Classify(coreHTTPApplyStageError("execute lifecycle", original))
	if got.Code != original.Code || got.CauseCode != original.CauseCode || got.Next != original.Next ||
		got.Message != "Core remote apply execute lifecycle: "+original.Message {
		t.Fatalf("Core apply stage and error authority were not retained: %+v", got)
	}
}
