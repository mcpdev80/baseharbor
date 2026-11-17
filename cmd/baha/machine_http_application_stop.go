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
	resolved, err := resolveApplication(ctx, e.store, machineApplicationArgs(input.Name, input.Environment), "destroy")
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}
	ctx, cancel := machineLifecycleContext(ctx)
	defer cancel()
	reportHTTPProgress(report, "stop", "Reviewing the current application stop consequences.", 20)
	if err := authorizeApplicationOperation(ctx, "app.stop", resolved); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, applicationStopModeKey{}, true)
	args := machineApplicationArgs(input.Name, input.Environment)
	args = append(args, "--yes")
	if err := executeApplicationDestroyLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	destroyResult, err := applicationDestroyResult(resolved, false)
	if err != nil {
		return nil, err
	}
	result := applicationStopResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Target: resolved.Target.Name, State: "stopped", Verified: destroyResult.Verified, DataPreserved: true, ProviderChangeReceipt: destroyResult.ProviderChangeResult}
	reportHTTPProgress(report, "verify", "Application stop and retained recovery ownership verified; consumer consequences remain as recorded.", 100)
	return result, nil
}
