package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

type machineRuntimeMutationInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses the effective target"`
	Environment string `json:"environment,omitempty" jsonschema:"explicit environment used for authorization; defaults to dev"`
	Provider    string `json:"provider,omitempty" jsonschema:"optional runtime provider identity; defaults to the target runtime provider"`
	Kind        string `json:"kind" jsonschema:"provider-neutral runtime resource kind"`
	ResourceID  string `json:"resource_id" jsonschema:"stable runtime resource id"`
}

func registerMCPRuntimeExplorerMutationTools(server *mcp.Server) {
	registerMCPRuntimeMutation(server, "runtime.start", runtimeexplorer.OperationStart)
	registerMCPRuntimeMutation(server, "runtime.stop", runtimeexplorer.OperationStop)
	registerMCPRuntimeMutation(server, "runtime.restart", runtimeexplorer.OperationRestart)
}

func registerMCPRuntimeMutation(server *mcp.Server, operationID string, operation runtimeexplorer.Operation) {
	mcp.AddTool(server, machineMCPTool(operationID, "Perform one bounded Runtime Explorer lifecycle operation on an authorized concrete resource.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRuntimeMutationInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, operationID, input.Target, input.Environment, ""); err != nil {
			return machineMCPFailure(err)
		}
		result, err := executeRuntimeMutation(ctx, input, operation)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}

func executeRuntimeMutation(ctx context.Context, input machineRuntimeMutationInput, operation runtimeexplorer.Operation) (runtimeexplorer.OperationResult, error) {
	explorer, target, err := runtimeExplorerForTarget(ctx, input.Target)
	if err != nil {
		return runtimeexplorer.OperationResult{}, err
	}
	capabilities, err := explorer.Capabilities(ctx, target)
	if err != nil {
		return runtimeexplorer.OperationResult{}, err
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = capabilities.Provider
	}
	ref := runtimeexplorer.ResourceRef{
		Provider:   provider,
		Target:     target,
		Kind:       runtimeexplorer.ResourceKind(strings.TrimSpace(input.Kind)),
		ResourceID: strings.TrimSpace(input.ResourceID),
	}
	resource, err := explorer.Inspect(ctx, ref)
	if err != nil {
		return runtimeexplorer.OperationResult{}, err
	}
	environment := strings.TrimSpace(input.Environment)
	if environment != "" && resource.Relationship.Environment != "" && resource.Relationship.Environment != environment {
		return runtimeexplorer.OperationResult{}, machine.NewError(
			machine.ErrorPolicyDenied,
			"Runtime resource is outside the authorized environment.",
			"Retry with the resource environment after obtaining the required operator authorization.",
			false,
		)
	}
	return explorer.Operate(ctx, runtimeexplorer.OperationRequest{Resource: ref, Operation: operation})
}

func (e *bahaMachineExecutor) executeHTTPRuntimeMutation(
	ctx context.Context,
	operationID string,
	operationContext machine.OperationContext,
	raw json.RawMessage,
) (any, error) {
	var input machineRuntimeMutationInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	var err error
	input.Target, err = bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return nil, err
	}
	input.Environment, err = bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return nil, err
	}
	if operationContext.Resource != "" {
		input.ResourceID, err = bindHTTPSelector("resource", operationContext.Resource, input.ResourceID)
		if err != nil {
			return nil, err
		}
	}
	switch operationID {
	case "runtime.start":
		return executeRuntimeMutation(ctx, input, runtimeexplorer.OperationStart)
	case "runtime.stop":
		return executeRuntimeMutation(ctx, input, runtimeexplorer.OperationStop)
	case "runtime.restart":
		return executeRuntimeMutation(ctx, input, runtimeexplorer.OperationRestart)
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Unsupported Runtime Explorer mutation.", "Use machine discovery.", false)
	}
}
