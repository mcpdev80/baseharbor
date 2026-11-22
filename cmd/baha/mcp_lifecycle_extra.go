package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"encoding/json"
	"bytes"
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
	// The optional Perses management surface (0428-084): typed operations share
	// the exact CLI mutation/read seams, so CLI -o json and the MCP structured
	// result stay equal. Machine transports run the shared apply lifecycle
	// immediately; the deferred-apply notice is the human CLI presentation only.
	mcp.AddTool(server, machineMCPTool("app.perses.status", "Report the optional Perses management surface three-state contract without mutation or a health probe.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = machineNoninteractiveContext(withTargetOverride(ctx, input.Target))
		result, err := reportPersesSurfaceStatus(ctx, store, machineApplicationArgs(input.Name, input.Environment))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("app.perses.enable", "Select the optional Perses management surface in the repository manifest and converge runtime through the shared apply lifecycle. Explicit confirmation is mandatory and ownership verification remains fail-closed.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machinePersesLifecycleInput) (*mcp.CallToolResult, any, error) {
		return callPersesLifecycleTool(ctx, store, input, application.PersesLifecycleEnable, true)
	})
	mcp.AddTool(server, machineMCPTool("app.perses.disable", "Deselect the optional Perses management surface in the repository manifest, preserving the backend observability owner, and converge runtime through the shared apply lifecycle. Explicit confirmation is mandatory.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machinePersesLifecycleInput) (*mcp.CallToolResult, any, error) {
		return callPersesLifecycleTool(ctx, store, input, application.PersesLifecycleDisable, false)
	})
	mcp.AddTool(server, machineMCPTool("app.perses.remove", "Retire only BaseHarbor-owned Perses runtime resources and disable the surface; Prometheus, Loki, Tempo and foreign data are never removed. Explicit confirmation is mandatory and ownership verification remains fail-closed.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machinePersesLifecycleInput) (*mcp.CallToolResult, any, error) {
		return callPersesLifecycleTool(ctx, store, input, application.PersesLifecycleRemove, false)
	})
	registerMCPAppPullTool(server, store)
}

// registerMCPAppPullTool exposes the guided source pull through the typed MCP
// tool boundary. It mirrors the CLI 'baha app pull' exactly: the current MCP
// context is authorized for app.pull, then the shared runAppPull implementation
// is reused (no logic rebuilt here) and the JSON machine result the CLI emits is
// returned unchanged so structured clients see the same pull plan or result.
func registerMCPAppPullTool(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.pull", "Safely pull selected repository sources as a strict fast-forward to their upstream, inspecting working tree, branch, upstream and divergence first; with --up converges the application via apply only after every selected source is pulled.", true),
		func(ctx context.Context, _ *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
			ctx = withTargetOverride(ctx, input.Target)
			if err := authorizeCurrentMCPContext(ctx, "app.pull", input.Target, input.Environment, ""); err != nil {
				return machineMCPFailure(err)
			}
			opts := appPullOptions{
				OutputFormat: outputJSON,
			}
			outBuf, errOutBuf := &bytes.Buffer{}, &bytes.Buffer{}
			if err := runAppPull(ctx, store, opts, outBuf, errOutBuf); err != nil {
				return machineMCPFailure(err)
			}
			var machineResult any
			if err := json.Unmarshal(outBuf.Bytes(), &machineResult); err != nil {
				return machineMCPFailure(err)
			}
			return nil, machineResult, nil
		})
}

type machinePersesLifecycleInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses BASEHARBOR_TARGET or configured default-target"`
	Name        string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	Approval    bool   `json:"approval,omitempty" jsonschema:"explicit confirmation for this Perses lifecycle mutation; ordinary approval does not imply backend changes"`
}

func callPersesLifecycleTool(ctx context.Context, store application.Store, input machinePersesLifecycleInput, action application.PersesLifecycleAction, setPerses bool) (*mcp.CallToolResult, any, error) {
	ctx = machineNoninteractiveContext(withTargetOverride(ctx, input.Target))
	if err := applicationlifecycle.RequireApproval("perses."+string(action), input.Approval); err != nil {
		return machineMCPFailure(err)
	}
	result, err := executePersesLifecycle(ctx, store, machineApplicationArgs(input.Name, input.Environment), action, setPerses, io.Discard, io.Discard)
	if err != nil {
		return machineMCPFailure(err)
	}
	return nil, result, nil
}
