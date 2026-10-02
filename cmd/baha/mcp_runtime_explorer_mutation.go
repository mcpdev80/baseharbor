package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerMCPRuntimeExplorerMutationTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("runtime.operate", "Perform an ownership-gated start, stop or restart on one stable Runtime Explorer resource.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRuntimeOperateInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "runtime.operate", input.Target, input.Environment, ""); err != nil {
			return machineMCPFailure(err)
		}
		result, err := executeRuntimeOperation(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
