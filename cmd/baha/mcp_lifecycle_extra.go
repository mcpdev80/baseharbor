package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
)

func machineNoninteractiveContext(ctx context.Context) context.Context {
	options := cli.OutputOptionsFromContext(ctx)
	options.NonInteractive = true
	return cli.WithOutputOptions(ctx, options)
}
func registerMCPAdditionalLifecycleTools(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.stop", "Stop owned application runtime and verify removal while preserving persistent data.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = machineNoninteractiveContext(withTargetOverride(ctx, input.Target))
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "down")
		if err != nil {
			return machineMCPFailure(err)
		}
		ctx, cancel := machineLifecycleContext(ctx)
		defer cancel()
		if err := stopApplicationRuntime(ctx, resolved, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		return nil, applicationStopResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, State: "stopped", DataPreserved: true}, nil
	})
	mcp.AddTool(server, machineMCPTool("app.preflight", "Run shared application preflight without mutation or interactive memory overrides.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = machineNoninteractiveContext(withTargetOverride(ctx, input.Target))
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "preflight")
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := collectApplicationPreflight(ctx, resolved, io.Discard, io.Discard)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
