package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineOperatorIdentityInput struct {
	Target      string `json:"target,omitempty"`
	Environment string `json:"environment"`
}
type machineProviderInitInput struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}
type machineProviderTestInput struct {
	Path string `json:"path"`
}
type machineWorkspaceShowInput struct {
	Manifest string `json:"manifest,omitempty"`
}

func registerMCPAdditionalReadTools(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("operator.identity", "Inspect verified operator identity without exposing local session tokens.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineOperatorIdentityInput) (*mcp.CallToolResult, any, error) {
		result, err := inspectOperatorIdentity(withTargetOverride(ctx, input.Target), input.Environment)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("provider.init", "Create a provider authoring scaffold without interactive input.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineProviderInitInput) (*mcp.CallToolResult, any, error) {
		result, err := initializeProviderScaffold(ctx, input.Path, input.ID)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("provider.test", "Inspect provider descriptor conformance; executable lifecycle acceptance uses the separate public conformance profile.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineProviderTestInput) (*mcp.CallToolResult, any, error) {
		result, err := inspectProviderContract(ctx, input.Path)
		if err != nil {
			return machineMCPFailure(err)
		}
		return &mcp.CallToolResult{IsError: result.Status != capability.ConformancePass}, result, nil
	})
	mcp.AddTool(server, machineMCPTool("workspace.show", "Inspect canonical source identity and this application's local checkout mapping.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineWorkspaceShowInput) (*mcp.CallToolResult, any, error) {
		result, err := inspectApplicationWorkspace(ctx, input.Manifest)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("app.show", "Inspect the shared secret-safe application overview and recorded recovery metadata.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "show")
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := inspectApplicationOverview(ctx, resolved)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
