package main

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineReleaseCheckInput struct {
	Channel string `json:"channel,omitempty"`
	Version string `json:"version,omitempty"`
}

func registerMCPReleaseTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("release.check", "Inspect a published release through shared selection without replacing the execution-host binary.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineReleaseCheckInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "release.check", "", "", ""); err != nil {
			return machineMCPFailure(err)
		}
		args := []string{"--check"}
		if input.Channel != "" {
			args = append(args, "--channel", input.Channel)
		}
		if input.Version != "" {
			args = append(args, "--version", input.Version)
		}
		opts, err := parseSelfUpdateOptions(args)
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := inspectSelfUpdate(ctx, version, opts)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
