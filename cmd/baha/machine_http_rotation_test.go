package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestHTTPManagedRotationBindsInstallationAndRequiresApproval(t *testing.T) {
	selected := machine.OperationContext{Target: "installation-a", Environment: "dev"}
	target, err := bindHTTPManagedRotation(selected, json.RawMessage(`{"approval":true}`))
	if err != nil || target != selected.Target {
		t.Fatal("exact approved installation rejected", target, err)
	}
	for _, test := range []struct {
		context machine.OperationContext
		input   string
		code    machine.ErrorCode
	}{
		{selected, `{}`, machine.ErrorPolicyDenied},
		{selected, `{"approval":false}`, machine.ErrorPolicyDenied},
		{selected, `{"approval":true,"target":"foreign"}`, machine.ErrorPolicyDenied},
		{selected, `{"approval":true,"recovery_file":"PRIVATE-RECOVERY"}`, machine.ErrorValidationFailed},
		{machine.OperationContext{Environment: "dev"}, `{"approval":true}`, machine.ErrorValidationFailed},
		{machine.OperationContext{Target: "installation-a"}, `{"approval":true}`, machine.ErrorValidationFailed},
		{machine.OperationContext{Target: "installation-a", Environment: "dev", Application: "foreign-app"}, `{"approval":true}`, machine.ErrorValidationFailed},
		{machine.OperationContext{Target: "installation-a", Environment: "dev", Workspace: "foreign-workspace"}, `{"approval":true}`, machine.ErrorValidationFailed},
	} {
		_, err := bindHTTPManagedRotation(test.context, json.RawMessage(test.input))
		var problem *machine.Error
		if !errors.As(err, &problem) || problem.Code != test.code || strings.Contains(err.Error(), "PRIVATE-RECOVERY") {
			t.Fatal("rotation escaped context/approval validation", err)
		}
	}
}
