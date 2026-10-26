package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineTargetDeleteInput struct {
	Name                  string                             `json:"name" jsonschema:"configured Target whose original definition, data and runtime owners are retained"`
	ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
}

func registerMCPTargetMutationTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("target.create", "Create explicit runtime/access Target configuration through the shared operation.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetCreateInput) (*mcp.CallToolResult, any, error) {
		ctx = withProviderChangeConsent(machineNoninteractiveContext(ctx), input.ProviderChangeConsent)
		result, err := createTargetDefinition(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("target.delete", "Detach Target configuration after two current-plan acknowledgements; preserve original definition recovery, runtime, bindings and data.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetDeleteInput) (*mcp.CallToolResult, any, error) {
		ctx = withProviderChangeConsent(machineNoninteractiveContext(ctx), input.ProviderChangeConsent)
		result, err := deleteTargetDefinition(ctx, input.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
