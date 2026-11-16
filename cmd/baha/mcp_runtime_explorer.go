package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerMCPRuntimeExplorerReadTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("runtime.capabilities", "Inspect provider-neutral Runtime Explorer capabilities for the effective Target.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRuntimeTargetInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "runtime.capabilities", input.Target, input.Environment, ""); err != nil {
			return machineMCPFailure(err)
		}
		result, err := collectRuntimeCapabilities(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("runtime.list", "List provider-neutral runtime resources with stable identity, ownership and BaseHarbor relationships.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRuntimeListInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "runtime.list", input.Target, input.Environment, ""); err != nil {
			return machineMCPFailure(err)
		}
		result, err := collectRuntimeResources(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("runtime.inspect", "Inspect one stable provider-neutral runtime resource without mutation.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRuntimeInspectInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "runtime.inspect", input.Target, input.Environment, ""); err != nil {
			return machineMCPFailure(err)
		}
		result, err := collectRuntimeResource(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("runtime.metrics", "Resolve the provider-neutral metrics handle for one stable runtime resource.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRuntimeInspectInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "runtime.metrics", input.Target, input.Environment, ""); err != nil {
			return machineMCPFailure(err)
		}
		result, err := collectRuntimeMetrics(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
