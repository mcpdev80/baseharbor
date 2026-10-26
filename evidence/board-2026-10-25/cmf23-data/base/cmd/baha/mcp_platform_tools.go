package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"time"
)

type machineOpenBaoInput struct {
	Target       string `json:"target,omitempty"`
	RecoveryFile string `json:"recovery_file"`
}
type machineDevelopmentInput struct {
	Target string `json:"target,omitempty"`
	Domain string `json:"domain,omitempty"`
}
type machineDevelopmentCredentialsInput struct {
	Target       string `json:"target,omitempty"`
	Reset        bool   `json:"reset,omitempty"`
	Username     string `json:"username,omitempty"`
	PasswordFile string `json:"password_file,omitempty"`
}

func registerMCPPlatformTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("openbao.status", "Inspect managed trust-plane state without credentials.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetInput) (*mcp.CallToolResult, any, error) {
		ctx, cancel := context.WithTimeout(withTargetOverride(ctx, input.Target), 30*time.Second)
		defer cancel()
		result, err := inspectManagedOpenBao(ctx)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	for _, action := range []string{"bootstrap", "unseal", "rotate"} {
		mcp.AddTool(server, machineMCPTool("openbao."+action, "Operate managed trust plane using operator-held protected recovery file; secret material is never returned.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineOpenBaoInput) (*mcp.CallToolResult, any, error) {
			ctx, cancel := context.WithTimeout(withTargetOverride(ctx, input.Target), 15*time.Minute)
			defer cancel()
			if input.RecoveryFile == "" {
				return machineMCPFailure(usageError("recovery file is required", "Supply an operator-held protected recovery file outside BaseHarbor state."))
			}
			var err error
			switch action {
			case "bootstrap":
				err = bootstrapManagedOpenBao(ctx, input.RecoveryFile)
			case "unseal":
				err = unsealManagedOpenBao(ctx, input.RecoveryFile)
			case "rotate":
				err = rotateManagedOpenBao(ctx, input.RecoveryFile)
			}
			if err != nil {
				return machineMCPFailure(err)
			}
			result, err := inspectManagedOpenBao(ctx)
			if err != nil {
				return machineMCPFailure(err)
			}
			return nil, result, nil
		})
	}
	mcp.AddTool(server, machineMCPTool("dev.domain", "Ensure or configure canonical target development domain.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineDevelopmentInput) (*mcp.CallToolResult, any, error) {
		result, err := configureDevelopmentDomain(withTargetOverride(ctx, input.Target), input.Domain)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("dev.credentials", "Ensure, configure or reset developer access through the shared managed authority; return only username and protected file reference.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineDevelopmentCredentialsInput) (*mcp.CallToolResult, any, error) {
		credentials, target, err := configureDevelopmentCredentials(withTargetOverride(ctx, input.Target), input.Reset, input.Username, input.PasswordFile)
		if err != nil {
			return machineMCPFailure(err)
		}
		path, err := devaccess.Path(target, "dev")
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, map[string]any{"target": target, "environment": "dev", "username": credentials.Username, "protected_file": path}, nil
	})
}
