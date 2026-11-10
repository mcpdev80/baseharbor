package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestHTTPApplicationSecretSetRejectsScopeEscapeBeforeRuntimeAccess(t *testing.T) {
	input := machineHTTPApplicationSecretInput{
		Target: "foreign-target", Application: "demo", Environment: "dev", Key: "APP_SECRET", Value: []byte("secret-value"),
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&bahaMachineExecutor{}).executeHTTPApplicationSecretSet(context.Background(), machine.OperationContext{
		Target: "authorized-target", Application: "demo", Environment: "dev",
	}, raw)
	var problem *machine.Error
	if !errors.As(err, &problem) || problem.Code != machine.ErrorPolicyDenied {
		t.Fatalf("foreign secret target was accepted or reached runtime access: %v", err)
	}
}

func TestHTTPApplicationSecretSetRequiresCompleteScopeAndValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope machine.OperationContext
		input machineHTTPApplicationSecretInput
	}{
		{"missing-target", machine.OperationContext{Application: "demo", Environment: "dev"}, machineHTTPApplicationSecretInput{Application: "demo", Environment: "dev", Key: "APP_SECRET", Value: []byte("x")}},
		{"missing-application", machine.OperationContext{Target: "remote", Environment: "dev"}, machineHTTPApplicationSecretInput{Target: "remote", Environment: "dev", Key: "APP_SECRET", Value: []byte("x")}},
		{"missing-environment", machine.OperationContext{Target: "remote", Application: "demo"}, machineHTTPApplicationSecretInput{Target: "remote", Application: "demo", Key: "APP_SECRET", Value: []byte("x")}},
		{"empty-value", machine.OperationContext{Target: "remote", Application: "demo", Environment: "dev"}, machineHTTPApplicationSecretInput{Target: "remote", Application: "demo", Environment: "dev", Key: "APP_SECRET"}},
		{"oversized-value", machine.OperationContext{Target: "remote", Application: "demo", Environment: "dev"}, machineHTTPApplicationSecretInput{Target: "remote", Application: "demo", Environment: "dev", Key: "APP_SECRET", Value: make([]byte, maxRemoteApplicationSecretBytes+1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			_, err = (&bahaMachineExecutor{}).executeHTTPApplicationSecretSet(context.Background(), tc.scope, raw)
			var problem *machine.Error
			if !errors.As(err, &problem) || (problem.Code != machine.ErrorValidationFailed && problem.Code != machine.ErrorPolicyDenied) {
				t.Fatalf("invalid secret scope/input reached runtime access: %v", err)
			}
		})
	}
}

func TestHTTPApplicationSecretSetIsDiscoverable(t *testing.T) {
	for _, id := range (&bahaMachineExecutor{}).SupportedOperationIDs() {
		if id == "secret.set" {
			return
		}
	}
	t.Fatal("authorized secret.set is missing from Machine HTTP discovery")
}
