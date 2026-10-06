package main

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineTargetDeleteInput struct {
	Name string `json:"name" jsonschema:"configured Target with no owned deployments or runtime state"`
}

func registerMCPTargetMutationTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("target.create", "Create explicit runtime/access Target configuration through the shared operation.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetCreateInput) (*mcp.CallToolResult, any, error) {
		result, err := createTargetDefinition(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("target.delete", "Delete empty Target configuration; owned deployments or runtime state block removal.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetDeleteInput) (*mcp.CallToolResult, any, error) {
		result, err := deleteTargetDefinition(ctx, input.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
