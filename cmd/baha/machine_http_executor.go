package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

type bahaMachineExecutor struct {
	store application.Store
}

func newBahaMachineExecutor(store application.Store) machinehttp.Executor {
	return &bahaMachineExecutor{store: store}
}

func (e *bahaMachineExecutor) Execute(
	ctx context.Context,
	operation machine.Operation,
	operationContext machine.OperationContext,
	input json.RawMessage,
	report machinehttp.ProgressReporter,
) (json.RawMessage, error) {
	opts := cli.OutputOptionsFromContext(ctx)
	opts.NonInteractive = true
	opts.Quiet = true
	opts.Plain = true
	ctx = cli.WithOutputOptions(ctx, opts)

	if report != nil {
		report(machine.OperationProgress{Stage: "accepted", Message: "Operation accepted by BaseHarbor Core."})
	}

	var (
		result any
		err    error
	)
	switch operation.ID {
	case "target", "inspect", "workspace.resolve", "workspace.status",
		"runtime.capabilities", "runtime.list", "runtime.inspect",
		"plan", "status", "doctor", "observe", "evidence",
		"provider.list", "provider.inspect", "provider.verify",
		"organization.inspect", "organization.check", "policy.check", "policy.explain":
		result, err = e.executeHTTPRead(ctx, operation.ID, operationContext, input)
	case "runtime.start", "runtime.stop", "runtime.restart":
		result, err = e.executeHTTPRuntimeMutation(ctx, operation.ID, operationContext, input)
	case "workspace.update", "app.new", "provider.add", "provider.remove",
		"organization.set", "organization.update", "runtime.operate":
		result, err = e.executeHTTPPlatformMutation(ctx, operation.ID, operationContext, input, report)
	case "apply", "update", "repair", "backup", "restore", "destroy":
		result, err = e.executeHTTPLifecycle(ctx, operation.ID, operationContext, input, report)
	default:
		return nil, machine.NewError(
			machine.ErrorUnsupported,
			"Machine operation is not implemented by the HTTP semantic executor.",
			"Use machine discovery and select an implemented operation.",
			false,
		)
	}
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, machine.Wrap(machine.ErrorInternal, err, "Retry the operation.", false)
	}
	return data, nil
}

func decodeHTTPInput(input json.RawMessage, target any) error {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	if err := json.Unmarshal(input, target); err != nil {
		return machine.Wrap(machine.ErrorValidationFailed, err, "Send valid JSON input for the selected operation.", false)
	}
	return nil
}

func bindHTTPSelector(label, authorized, supplied string) (string, error) {
	authorized = strings.TrimSpace(authorized)
	supplied = strings.TrimSpace(supplied)
	if authorized != "" && supplied != "" && supplied != authorized {
		return "", &machine.Error{
			Code:      machine.ErrorPolicyDenied,
			CauseCode: "operation_context_mismatch",
			Message:   "Machine input attempts to escape the authorized operation context.",
			Resource:  label,
			Next:      "Use selectors that match the authorized execution context.",
		}
	}
	if authorized != "" {
		return authorized, nil
	}
	return supplied, nil
}

func bindHTTPApplicationInput(operationContext machine.OperationContext, input *machineApplicationInput) error {
	var err error
	input.Target, err = bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return err
	}
	input.Environment, err = bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return err
	}
	input.Name, err = bindHTTPSelector("application", operationContext.Application, input.Name)
	return err
}

func bindHTTPApplicationSelectors(operationContext machine.OperationContext, target, environment, name *string) error {
	var err error
	*target, err = bindHTTPSelector("target", operationContext.Target, *target)
	if err != nil {
		return err
	}
	*environment, err = bindHTTPSelector("environment", operationContext.Environment, *environment)
	if err != nil {
		return err
	}
	*name, err = bindHTTPSelector("application", operationContext.Application, *name)
	return err
}
