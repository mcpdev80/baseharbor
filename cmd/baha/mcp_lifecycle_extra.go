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
	mcp.AddTool(server, machineMCPTool("app.stop", "Stop owned application runtime and verify removal while preserving persistent data.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationStopInput) (*mcp.CallToolResult, any, error) {
		ctx = machineNoninteractiveContext(withProviderChangeConsent(withTargetOverride(ctx, input.Target), input.ProviderChangeConsent))
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "destroy")
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := authorizeApplicationOperation(ctx, "app.stop", resolved); err != nil {
			return machineMCPFailure(err)
		}
		ctx, cancel := machineLifecycleContext(ctx)
		defer cancel()
		ctx = context.WithValue(ctx, applicationStopModeKey{}, true)
		args := machineApplicationArgs(input.Name, input.Environment)
		args = append(args, "--yes")
		if err := executeApplicationDestroyLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		destroyResult, err := applicationDestroyResult(resolved, false)
		if err != nil {
			return machineMCPFailure(err)
		}
		result := applicationStopResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Target: resolved.Target.Name, State: "stopped", Verified: destroyResult.Verified, DataPreserved: true, ProviderChangeReceipt: destroyResult.ProviderChangeResult}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("app.preflight", "Run shared application preflight without mutation or interactive memory overrides.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = machineNoninteractiveContext(withTargetOverride(ctx, input.Target))
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "preflight")
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := collectApplicationPreflight(ctx, resolved, io.Discard, io.Discard)
		if err != nil {
			if len(result.Checks) == 0 {
				return machineMCPFailure(err)
			}
			return &mcp.CallToolResult{IsError: true}, result, nil
		}
		return nil, result, nil
	})
}
