package main

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
)

type machineConnectivityInput struct {
	Target      string                    `json:"target,omitempty"`
	Source      connectivityEndpointInput `json:"source"`
	Destination connectivityEndpointInput `json:"destination"`
}

func registerMCPConnectivityTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("connectivity.list", "List directional deny-by-default connectivity exceptions.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTrustInput) (*mcp.CallToolResult, any, error) {
		rules, err := listConnectivityRules(withTargetOverride(ctx, input.Target))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, map[string]any{"rules": rules}, nil
	})
	mcp.AddTool(server, machineMCPTool("connectivity.connect", "Converge and verify one directional connectivity exception after authorization of both environments.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineConnectivityInput) (*mcp.CallToolResult, any, error) {
		source, err := parseConnectivityEndpointInput(formatConnectivityInput(input.Source))
		if err != nil {
			return machineMCPFailure(err)
		}
		target, err := parseConnectivityEndpointInput(formatConnectivityInput(input.Destination))
		if err != nil {
			return machineMCPFailure(err)
		}
		if source.Port != 0 {
			return machineMCPFailure(usageError("source must not include a port", "Provide a source service identity."))
		}
		result, err := connectApplications(withTargetOverride(ctx, input.Target), source, target, io.Discard)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("connectivity.disconnect", "Remove one exact directional connectivity exception through shared suspension and policy cleanup.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineConnectivityInput) (*mcp.CallToolResult, any, error) {
		source, err := parseConnectivityEndpointInput(formatConnectivityInput(input.Source))
		if err != nil {
			return machineMCPFailure(err)
		}
		target, err := parseConnectivityEndpointInput(formatConnectivityInput(input.Destination))
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := disconnectApplications(withTargetOverride(ctx, input.Target), source, target)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, map[string]any{"removed": true, "rule": result}, nil
	})
}
