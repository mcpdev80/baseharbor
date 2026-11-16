package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
)

type machineRuntimeIdentityInput struct {
	Target      string `json:"target,omitempty"`
	Name        string `json:"name,omitempty"`
	Environment string `json:"environment,omitempty"`
	Approval    bool   `json:"approval"`
}
type machineTLSUpdateInput struct {
	Target      string `json:"target,omitempty"`
	Name        string `json:"name,omitempty"`
	Environment string `json:"environment,omitempty"`
	CheckOnly   bool   `json:"check_only,omitempty"`
}

func registerMCPIdentityTLSTools(server *mcp.Server, store application.Store) {
	for _, action := range []string{"rotate", "revoke"} {
		action := action
		mcp.AddTool(server, machineMCPTool("runtime-identity."+action, "Manage the app-scoped runtime credential after approval; never return credential bytes.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineRuntimeIdentityInput) (*mcp.CallToolResult, any, error) {
			ctx = withTargetOverride(ctx, input.Target)
			resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "runtime-identity "+action)
			if err != nil {
				return machineMCPFailure(err)
			}
			if err := authorizeApplicationOperation(ctx, "runtime-identity."+action, resolved); err != nil {
				return machineMCPFailure(err)
			}
			if err := applicationlifecycle.RequireApproval("runtime-identity."+action, input.Approval); err != nil {
				return machineMCPFailure(err)
			}
			if err := mutateApplicationRuntimeIdentity(ctx, resolved, action); err != nil {
				return machineMCPFailure(err)
			}
			return nil, map[string]any{"application": resolved.Manifest.Name, "environment": resolved.Manifest.Environment, "action": action, "completed": true}, nil
		})
	}
	mcp.AddTool(server, machineMCPTool("tls.update", "Check or safely install a newer existing TLS certificate and verify the shared restart path.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTLSUpdateInput) (*mcp.CallToolResult, any, error) {
		ctx = machineNoninteractiveContext(withTargetOverride(ctx, input.Target))
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "tls update")
		if err != nil {
			return machineMCPFailure(err)
		}
		ctx, cancel := machineLifecycleContext(ctx)
		defer cancel()
		result, err := updateApplicationTLS(ctx, resolved, input.CheckOnly, io.Discard)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
