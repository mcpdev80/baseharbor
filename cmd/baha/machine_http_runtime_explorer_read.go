package main

import (
	"context"
	"encoding/json"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func executeHTTPRuntimeExplorerRead(
	ctx context.Context,
	operationID string,
	operationContext machine.OperationContext,
	raw json.RawMessage,
) (any, error) {
	switch operationID {
	case "runtime.capabilities":
		var input machineRuntimeTargetInput
		if err := decodeHTTPInput(raw, &input); err != nil {
			return nil, err
		}
		target, err := bindHTTPSelector("target", operationContext.Target, input.Target)
		if err != nil {
			return nil, err
		}
		input.Target = target
		return collectRuntimeCapabilities(ctx, input)

	case "runtime.list":
		var input machineRuntimeListInput
		if err := decodeHTTPInput(raw, &input); err != nil {
			return nil, err
		}
		target, err := bindHTTPSelector("target", operationContext.Target, input.Target)
		if err != nil {
			return nil, err
		}
		input.Target = target
		return collectRuntimeResources(ctx, input)

	case "runtime.inspect":
		var input machineRuntimeInspectInput
		if err := decodeHTTPInput(raw, &input); err != nil {
			return nil, err
		}
		target, err := bindHTTPSelector("target", operationContext.Target, input.Target)
		if err != nil {
			return nil, err
		}
		input.Target = target
		if operationContext.Resource != "" {
			resourceID, err := bindHTTPSelector("resource", operationContext.Resource, input.ResourceID)
			if err != nil {
				return nil, err
			}
			input.ResourceID = resourceID
		}
		return collectRuntimeResource(ctx, input)

	default:
		return nil, machine.NewError(
			machine.ErrorUnsupported,
			"Unsupported Runtime Explorer read operation.",
			"Use machine discovery to negotiate Runtime Explorer operations.",
			false,
		)
	}
}
