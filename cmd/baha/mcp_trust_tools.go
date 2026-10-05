package main

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineTrustInput struct {
	Target string `json:"target,omitempty"`
}
type machineTrustExportInput struct {
	Target string `json:"target,omitempty"`
	Path   string `json:"path" jsonschema:"new destination for public CA certificate; existing files are never overwritten"`
}
type machineTrustInstallInput struct {
	Target   string `json:"target,omitempty"`
	Approval bool   `json:"approval" jsonschema:"explicit consent to mutate the MCP execution host trust store"`
}

func registerMCPTrustTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("trust.status", "Inspect managed public CA trust on the execution host without mutation.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTrustInput) (*mcp.CallToolResult, any, error) {
		result, err := inspectManagedTrust(withTargetOverride(ctx, input.Target))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("trust.export", "Export public managed CA material to a new execution-host file; private issuer keys never leave the provider.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTrustExportInput) (*mcp.CallToolResult, any, error) {
		result, err := exportManagedTrust(withTargetOverride(ctx, input.Target), input.Path)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("trust.install", "Install public managed CA into this execution host's trust store after explicit approval.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTrustInstallInput) (*mcp.CallToolResult, any, error) {
		result, err := installManagedTrust(withTargetOverride(ctx, input.Target), input.Approval)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
