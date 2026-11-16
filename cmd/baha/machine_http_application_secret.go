package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

type machineHTTPApplicationSecretInput struct {
	Target      string `json:"target,omitempty"`
	Application string `json:"application,omitempty"`
	Environment string `json:"environment,omitempty"`
	Key         string `json:"key"`
	Value       []byte `json:"value"`
}

const maxRemoteApplicationSecretBytes = 700 << 10

func (e *bahaMachineExecutor) executeHTTPApplicationSecretSet(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
) (any, error) {
	var input machineHTTPApplicationSecretInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	defer zeroBytes(input.Value)
	var err error
	input.Target, err = bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return nil, err
	}
	input.Application, err = bindHTTPSelector("application", operationContext.Application, input.Application)
	if err != nil {
		return nil, err
	}
	input.Environment, err = bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return nil, err
	}
	input.Key = strings.TrimSpace(input.Key)
	if input.Target == "" || input.Application == "" || input.Environment == "" || input.Key == "" || len(input.Value) == 0 || len(input.Value) > maxRemoteApplicationSecretBytes {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Application secret input is incomplete or outside the remote request size limit.", "Provide an authorized Target, application, environment, key, and secret value of at most 716800 bytes.", false)
	}

	ctx = withTargetOverride(ctx, input.Target)
	resolved, err := resolveApplication(ctx, e.store, machineApplicationArgs(input.Application, input.Environment), "secret.set")
	if err != nil {
		return nil, err
	}
	if resolved.Target.Name != input.Target || resolved.Manifest.Name != input.Application || resolved.Manifest.Environment != input.Environment {
		return nil, machine.NewError(machine.ErrorPolicyDenied, "The resolved application secret scope does not match the authorized request.", "Use the registered application and environment in the machine operation context.", false)
	}
	result, err := setApplicationSecret(ctx, resolved, input.Key, func() ([]byte, error) {
		return append([]byte(nil), input.Value...), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
