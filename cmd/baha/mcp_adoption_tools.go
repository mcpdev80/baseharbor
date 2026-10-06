package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
)

type machineApplicationCreateInput struct {
	Target string               `json:"target,omitempty"`
	Intent application.Manifest `json:"intent" jsonschema:"complete validated portable Application Intent with stable ApplicationID"`
}
type machineApplicationAdoptInput struct {
	Target         string                                     `json:"target,omitempty"`
	Repository     string                                     `json:"repository" jsonschema:"existing repository root to adopt; existing manifest is never overwritten"`
	Intent         application.Manifest                       `json:"intent"`
	WorkloadSource *repositoryinspect.WorkloadSourceCandidate `json:"workload_source,omitempty" jsonschema:"explicit inspected workload source choice when discovery is ambiguous"`
}
type machineApplicationConfigureInput struct {
	Target               string `json:"target,omitempty"`
	Name                 string `json:"name,omitempty"`
	Environment          string `json:"environment,omitempty"`
	Hostname             string `json:"hostname,omitempty"`
	TLSMode              string `json:"tls_mode,omitempty" jsonschema:"acme, existing or local"`
	CertificateDirectory string `json:"certificate_directory,omitempty"`
}

func registerMCPAdoptionTools(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.create", "Create validated target-managed Application Intent without starting runtime resources.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationCreateInput) (*mcp.CallToolResult, any, error) {
		path, err := createTargetManagedApplication(withTargetOverride(ctx, input.Target), input.Intent)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, applicationAdoptionResult{Application: input.Intent.Name, ApplicationID: input.Intent.ApplicationID, Environment: input.Intent.Environment, Manifest: path}, nil
	})
	mcp.AddTool(server, machineMCPTool("app.adopt", "Persist explicit Application Intent and inspected source selection into an existing repository without a wizard.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationAdoptInput) (*mcp.CallToolResult, any, error) {
		result, err := persistRepositoryApplication(withTargetOverride(ctx, input.Target), input.Repository, input.Intent, input.WorkloadSource)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("app.configure", "Resolve explicit deployment inputs through the shared noninteractive initialization service.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationConfigureInput) (*mcp.CallToolResult, any, error) {
		ctx = machineNoninteractiveContext(withTargetOverride(ctx, input.Target))
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "init")
		if err != nil {
			return machineMCPFailure(err)
		}
		if !resolved.FromRepository {
			return machineMCPFailure(usageError("application configuration requires repository intent", "Supply a repository application."))
		}
		options := repositoryInitOptions{Hostname: input.Hostname, TLSMode: input.TLSMode, CertDir: input.CertificateDirectory, Yes: true}
		if err := runRepositoryRuntimeInitResolved(ctx, resolved, resolved.repositoryRoot(), options, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		return nil, map[string]any{"application": resolved.Manifest.Name, "environment": resolved.Manifest.Environment, "configured": true}, nil
	})
}
