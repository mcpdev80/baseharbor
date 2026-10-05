package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"strings"
	"time"
)

type machineControlPlaneUpInput struct {
	Target       string `json:"target,omitempty"`
	PostgresPort int    `json:"postgres_port,omitempty"`
	OpenBaoPort  int    `json:"openbao_port,omitempty"`
	RecoveryFile string `json:"recovery_file,omitempty"`
}
type machineControlPlaneDestroyInput struct {
	Target   string `json:"target,omitempty"`
	Approval bool   `json:"approval"`
}

func registerMCPControlPlaneTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("control-plane.status", "Inspect selected target control-plane readiness and HA satisfaction.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetInput) (*mcp.CallToolResult, any, error) {
		ctx, cancel := context.WithTimeout(withTargetOverride(ctx, input.Target), 30*time.Second)
		defer cancel()
		result, err := inspectControlPlane(ctx)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("control-plane.doctor", "Inspect control-plane prerequisites without returning runtime console or credentials.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetInput) (*mcp.CallToolResult, any, error) {
		result, err := inspectControlPlaneDoctor(withTargetOverride(ctx, input.Target))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("control-plane.up", "Initialize or converge the selected target control plane through shared lifecycle and memory preflight.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineControlPlaneUpInput) (*mcp.CallToolResult, any, error) {
		ctx, cancel := context.WithTimeout(machineNoninteractiveContext(withTargetOverride(ctx, input.Target)), 15*time.Minute)
		defer cancel()
		opts := runtimeUpOptions{Yes: true, ControlPlaneOnly: true, PostgresPort: input.PostgresPort, OpenBaoPort: input.OpenBaoPort, RecoveryFile: input.RecoveryFile}
		if err := runtimeUpGuided(ctx, strings.NewReader(""), io.Discard, opts); err != nil {
			return machineMCPFailure(err)
		}
		result, err := inspectControlPlane(ctx)
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := requireControlPlaneReady(result); err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	for _, action := range []string{"stop", "repair"} {
		mcp.AddTool(server, machineMCPTool("control-plane."+action, "Operate the selected owned control plane through existing lifecycle; preserve persistent data.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetInput) (*mcp.CallToolResult, any, error) {
			ctx, cancel := context.WithTimeout(withTargetOverride(ctx, input.Target), 3*time.Minute)
			defer cancel()
			var err error
			if action == "stop" {
				err = runtimeDown(ctx, io.Discard)
			} else {
				err = repairExistingControlPlaneRuntime(ctx, io.Discard)
			}
			if err != nil {
				return machineMCPFailure(err)
			}
			result, err := inspectControlPlane(ctx)
			if err != nil {
				return machineMCPFailure(err)
			}
			if action == "repair" {
				if err := requireControlPlaneReady(result); err != nil {
					return machineMCPFailure(err)
				}
			}
			return nil, result, nil
		})
	}
	mcp.AddTool(server, machineMCPTool("installation.destroy", "Remove all owned BaseHarbor installation resources after explicit approval and authorization of every registered deployment; preserve external application source/data.", true), func(ctx context.Context, req *mcp.CallToolRequest, input struct {
		Approval bool `json:"approval"`
	}) (*mcp.CallToolResult, any, error) { if err := authorizeCurrentMCPContext(ctx, "installation.destroy", "", "", ""); err != nil {
		return machineMCPFailure(err)
	}; if err := applicationlifecycle.RequireApproval("installation.destroy", input.Approval); err != nil {
		return machineMCPFailure(err)
	}; ctx, cancel := machineLifecycleContext(ctx); defer cancel(); if err := destroyInstallation(ctx, true, io.Discard, io.Discard); err != nil {
		return machineMCPFailure(err)
	}; return nil, map[string]any{"destroyed": true, "external_application_source_and_data_preserved": true}, nil })

	mcp.AddTool(server, machineMCPTool("control-plane.destroy", "Destroy selected owned control plane after approval and existing application-ownership preflight.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineControlPlaneDestroyInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		if err := authorizeCurrentMCPContext(ctx, "control-plane.destroy", "", "", ""); err != nil {
			return machineMCPFailure(err)
		}
		if err := applicationlifecycle.RequireApproval("control-plane.destroy", input.Approval); err != nil {
			return machineMCPFailure(err)
		}
		if err := destroyControlPlane(ctx, true, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		return nil, map[string]any{"destroyed": true, "application_data_preserved": true}, nil
	})
}
