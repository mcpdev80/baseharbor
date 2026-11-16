package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineStackShowInput struct {
	Name string `json:"name" jsonschema:"configured Stack Profile name"`
}
type machineStackCreateInput struct {
	Profile development.StackProfile `json:"profile" jsonschema:"complete reusable Stack Profile with canonical adapter and capability intent"`
	Scope   development.ProfileScope `json:"scope,omitempty" jsonschema:"user or repository; defaults to user"`
}

func registerMCPStackTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("stack.list", "List reusable Stack Profiles in deterministic name order.", false), func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		result, err := listStackProfiles(ctx)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("stack.show", "Resolve one reusable Stack Profile including inherited sources.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineStackShowInput) (*mcp.CallToolResult, any, error) {
		result, err := inspectStackProfile(ctx, input.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("stack.create", "Validate and save a reusable Stack Profile through the shared operation.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineStackCreateInput) (*mcp.CallToolResult, any, error) {
		result, err := createStackProfile(ctx, input.Profile, input.Scope)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
