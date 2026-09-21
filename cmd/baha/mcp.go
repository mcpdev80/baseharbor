package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

type machineInspectInput struct {
	Path string `json:"path,omitempty" jsonschema:"local repository path or Git URL; defaults to the current directory"`
}

type machineApplicationInput struct {
	Name string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside a repository containing baseharbor.yaml"`
}

type machineToolError struct {
	ContractVersion string         `json:"contract_version"`
	Error           *machine.Error `json:"error"`
}

func mcpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "mcp",
		Summary: "Expose BaseHarbor semantic operations over MCP",
		Usage:   "baha mcp serve",
		Long:    "Runs a local stdio Model Context Protocol server exposing a deliberately small BaseHarbor semantic tool surface. It never exposes generic shell, Docker or Compose execution.",
		Children: []*cli.Command{
			{
				Name:    "serve",
				Summary: "Run the local stdio MCP server",
				Usage:   "baha mcp serve",
				Long:    "Uses stdin/stdout only. stdout is reserved exclusively for MCP protocol messages; diagnostics are returned as structured tool errors or written to stderr by the process entrypoint.",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) != 0 {
						return usageError("baha mcp serve does not accept arguments", "Run 'baha mcp serve'.")
					}
					return runMCPServer(ctx, store)
				},
			},
		},
	}
}

func runMCPServer(ctx context.Context, store application.Store) error {
	return newMCPServer(store).Run(ctx, &mcp.StdioTransport{})
}

func newMCPServer(store application.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "baseharbor",
		Version: version,
	}, &mcp.ServerOptions{
		SupportedProtocolVersions: []string{"2026-07-28", "2025-11-25"},
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "baseharbor.inspect",
		Description: "Read-only repository inspection. Returns deterministic, secret-safe evidence and capability findings without changing repository or runtime state.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: false, OpenWorldHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input machineInspectInput) (*mcp.CallToolResult, any, error) {
		path := strings.TrimSpace(input.Path)
		if path == "" {
			path = "."
		}
		result, err := inspectRepositorySource(ctx, path)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "baseharbor.plan",
		Description: "Read-only deterministic desired-state plan for the current repository or named application.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: false, OpenWorldHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		resolved, err := resolveMachineApplication(store, strings.TrimSpace(input.Name), "plan")
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := application.BuildPlan(resolved.Manifest)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "baseharbor.status",
		Description: "Read-only runtime and readiness observation for the current repository or named application.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: false, OpenWorldHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		var args []string
		if name := strings.TrimSpace(input.Name); name != "" {
			args = []string{name}
		}
		result, err := collectApplicationStatusResult(ctx, store, args)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "baseharbor.doctor",
		Description: "Read-only diagnostic verification for the current repository or named application.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: false, OpenWorldHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		var args []string
		if name := strings.TrimSpace(input.Name); name != "" {
			args = []string{name}
		}
		result, err := collectApplicationDoctor(ctx, store, args)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	return server
}

func machineMCPFailure(err error) (*mcp.CallToolResult, any, error) {
	classified := machine.Classify(err)
	payload := machineToolError{ContractVersion: machine.ContractVersion, Error: classified}
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, payload, nil
}

