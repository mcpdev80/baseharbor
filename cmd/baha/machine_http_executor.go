package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type bahaMachineExecutor struct {
	store             application.Store
	connectorSessions *targetsession.Pool
}

// Bound once during Core startup, before accepting any machine request.
func (e *bahaMachineExecutor) BindConnectorSessions(pool *targetsession.Pool) {
	e.connectorSessions = pool
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
	ctx = targetsession.WithPool(ctx, e.connectorSessions)
	ctx = withOrganizationEnvironment(ctx, operationContext.Environment)
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
	switch httpOperationKinds[operation.ID] {
	case httpRead:
		result, err = e.executeHTTPRead(ctx, operation.ID, operationContext, input)
	case httpRuntimeMutation:
		result, err = e.executeHTTPRuntimeMutation(ctx, operation.ID, operationContext, input)
	case httpPlatformMutation:
		result, err = e.executeHTTPPlatformMutation(ctx, operation.ID, operationContext, input, report)
	case httpLifecycle:
		result, err = e.executeHTTPLifecycle(ctx, operation.ID, operationContext, input, report)
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Machine operation is not implemented by the HTTP semantic executor.", "Use machine discovery and select an implemented operation.", false)
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
	invalid := func() error {
		return machine.NewError(machine.ErrorValidationFailed, "Invalid machine operation input.", "Send valid JSON input for the selected operation.", false)
	}
	if machine.ValidateJSONObject(input, 1<<20) != nil {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return invalid()
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return invalid()
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
