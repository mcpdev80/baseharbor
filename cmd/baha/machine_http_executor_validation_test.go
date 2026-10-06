package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestHTTPAdvertisedReadOperationsReachInputValidation(t *testing.T) {
	executor := &bahaMachineExecutor{}
	for _, id := range executor.SupportedOperationIDs() {
		t.Run(id, func(t *testing.T) {
			_, err := executor.Execute(context.Background(), machine.Operation{ID: id}, machine.OperationContext{}, json.RawMessage(`{"SECRET-CREDENTIAL":"must-not-echo"}`), nil)
			if err == nil {
				t.Fatal("unexpected input accepted")
			}
			typed, ok := err.(*machine.Error)
			if !ok || typed.Code != machine.ErrorValidationFailed {
				t.Fatalf("advertised HTTP operation did not reach input validation: %v", err)
			}
			if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "must-not-echo") {
				t.Fatal("input credential echoed")
			}
		})
	}
}

func TestHTTPInputRejectsUnknownNullTrailingAndOversizedData(t *testing.T) {
	for _, raw := range []string{`{"target":"safe","SECRET-CREDENTIAL":"hidden"}`, "null", `{"target":"safe"}{"target":"foreign"}`, strings.Repeat(" ", (1<<20)+1)} {
		var input machineRuntimeTargetInput
		if err := decodeHTTPInput(json.RawMessage(raw), &input); err == nil {
			t.Fatal("invalid input accepted")
		} else if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "hidden") {
			t.Fatal("secret echoed")
		}
	}
	var input machineRuntimeTargetInput
	if err := decodeHTTPInput(json.RawMessage(`{"target":"safe","environment":"dev"}`), &input); err != nil || input.Target != "safe" || input.Environment != "dev" {
		t.Fatalf("valid input rejected: %+v %v", input, err)
	}
}
