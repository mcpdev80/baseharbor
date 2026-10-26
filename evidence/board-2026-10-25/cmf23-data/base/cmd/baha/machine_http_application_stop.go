package main

import (
	"context"
	"encoding/json"
	"io"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

// HTTP uses the same bounded input, selected deployment, staged consent and
// actual stop receipt as CLI/MCP. Generic approval cannot substitute consent.
func (e *bahaMachineExecutor) executeHTTPApplicationStop(ctx context.Context, operationContext machine.OperationContext, raw json.RawMessage, report machinehttp.ProgressReporter) (any, error) {
	var input machineApplicationStopInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationSelectors(operationContext, &input.Target, &input.Environment, &input.Name); err != nil {
		return nil, err
	}
	ctx = machineNoninteractiveContext(withProviderChangeConsent(withTargetOverride(ctx, input.Target), input.ProviderChangeConsent))
	resolved, err := resolveApplication(ctx, e.store, machineApplicationArgs(input.Name, input.Environment), "down")
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}
	ctx, cancel := machineLifecycleContext(ctx)
	defer cancel()
	reportHTTPProgress(report, "stop", "Reviewing the current application stop consequences.", 20)
	result, err := stopApplicationRuntimeWithResult(ctx, resolved, io.Discard, io.Discard)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "verify", "Application stop and retained recovery ownership verified; consumer consequences remain as recorded.", 100)
	return result, nil
}
