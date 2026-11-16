package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineConnectionInput struct {
	Target   string `json:"target,omitempty"`
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind" jsonschema:"postgres or valkey"`
	Instance string `json:"instance,omitempty"`
}

func registerMCPEnvironmentTools(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.environment", "Inspect masked application environment contract; credentials are never revealed through MCP.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "env")
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := inspectApplicationEnvironment(ctx, resolved, false)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("app.connection", "Inspect typed service connection metadata without passwords or credential-bearing URIs.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineConnectionInput) (*mcp.CallToolResult, any, error) {
		if input.Kind != "postgres" && input.Kind != "valkey" {
			return machineMCPFailure(usageError("unsupported service connection kind", "Choose postgres or valkey."))
		}
		_, binding, err := resolveAccessBinding(withTargetOverride(ctx, input.Target), store, input.Name, input.Kind, input.Instance)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, publicApplicationConnection(input.Kind, binding), nil
	})
}
