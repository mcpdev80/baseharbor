package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type machineWorkspaceInitInput struct {
	Target     string                         `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment Target for shared authorization context"`
	Manifest   string                         `json:"manifest,omitempty" jsonschema:"repository manifest path; defaults to the current application repository"`
	Sources    []development.SourceDefinition `json:"sources" jsonschema:"canonical repository or immutable OCI source identities"`
	Components []development.ComponentSource  `json:"components" jsonschema:"application component to source identity mappings"`
}

type machineWorkspaceMapInput struct {
	Target   string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment Target for shared authorization context"`
	Manifest string `json:"manifest,omitempty" jsonschema:"repository manifest path; defaults to the current application repository"`
	Source   string `json:"source" jsonschema:"canonical repository source identity"`
	Path     string `json:"path" jsonschema:"existing local source checkout path"`
}

func registerMCPWorkspaceMutationTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("workspace.init", "Configure canonical component/source identities without interactive prompts.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineWorkspaceInitInput) (*mcp.CallToolResult, any, error) {
		result, err := initializeApplicationWorkspace(ctx, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("workspace.map", "Map a canonical source to a developer-local checkout through the shared workspace operation.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineWorkspaceMapInput) (*mcp.CallToolResult, any, error) {
		result, err := mapApplicationWorkspace(withTargetOverride(ctx, input.Target), input.Manifest, input.Source, input.Path)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
}
