package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type machineTargetInput struct {
	Target string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses BASEHARBOR_TARGET or configured default-target"`
}

type machineInspectInput struct {
	Path string `json:"path,omitempty" jsonschema:"local repository path or Git URL; defaults to the current directory"`
}

type machineApplicationInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses BASEHARBOR_TARGET or configured default-target"`
	Name        string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
}

type machineUpdateInput struct {
	Target             string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	Environment        string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	BackupPasswordFile string `json:"backup_password_file,omitempty" jsonschema:"owner-only local file containing the backup password used for the pre-update recovery point"`
	NoBackup           bool   `json:"no_backup,omitempty" jsonschema:"explicitly acknowledge updating durable state without a pre-update recovery point"`
}

type machineBackupInput struct {
	Target       string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	Name         string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment  string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	OutputPath   string `json:"output_path,omitempty" jsonschema:"optional local path for the encrypted BaseHarbor recovery archive"`
	PasswordFile string `json:"password_file,omitempty" jsonschema:"owner-only local file containing the backup password; secret values are never accepted directly"`
}

type machineRestoreInput struct {
	Target       string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	BackupPath   string `json:"backup_path,omitempty" jsonschema:"local encrypted BaseHarbor recovery archive to restore"`
	Name         string `json:"name,omitempty" jsonschema:"optional expected application identity"`
	Environment  string `json:"environment,omitempty" jsonschema:"optional expected deployment environment"`
	PasswordFile string `json:"password_file" jsonschema:"owner-only local file containing the backup password; secret values are never accepted directly"`
}

type machineDestroyInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	Name        string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	Approval    bool   `json:"approval,omitempty" jsonschema:"explicit operator approval required before destructive mutation"`
	FullReset   bool   `json:"full_reset,omitempty" jsonschema:"also remove BaseHarbor-owned repository deployment and normalized TLS state"`
}

type machineToolError struct {
	ContractVersion string         `json:"contract_version"`
	Error           *machine.Error `json:"error"`
}

type machineLifecycleStatusResult struct {
	applicationlifecycle.Result
	Status applicationStatusResult `json:"status"`
}

type machineLifecycleDoctorResult struct {
	applicationlifecycle.Result
	Doctor applicationDoctorResult `json:"doctor"`
}

type machineObserveResult struct {
	ContractVersion string                  `json:"contract_version"`
	Target          string                  `json:"target"`
	Application     string                  `json:"application"`
	Environment     string                  `json:"environment"`
	Status          applicationStatusResult `json:"status"`
	Doctor          applicationDoctorResult `json:"doctor"`
}

type machineBackupResult struct {
	applicationlifecycle.Result
	Backup application.BackupMetadata `json:"backup"`
}

func mcpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "mcp",
		Summary: "Expose BaseHarbor semantic operations over MCP",
		Usage:   "baha mcp serve",
		Long:    "Runs a local stdio Model Context Protocol server exposing a deliberately small BaseHarbor semantic tool surface. It never exposes generic shell, Docker, Compose or Podman execution.",
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
		Capabilities:              &mcp.ServerCapabilities{},
	})

	registerMCPReadTools(server, store)
	registerMCPLifecycleTools(server, store)
	return server
}

func machineMCPTool(operationID, description string, openWorld bool) *mcp.Tool {
	operation, ok := machine.OperationByID(operationID)
	if !ok || operation.MCPTool == "" {
		panic("BaseHarbor machine operation is not registered for MCP: " + operationID)
	}
	readOnly := operation.Safety == machine.SafetyReadOnly
	destructive := operation.Safety == machine.SafetyDestructive
	return &mcp.Tool{
		Name:        operation.MCPTool,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readOnly,
			DestructiveHint: boolPointer(destructive),
			OpenWorldHint:   boolPointer(openWorld),
		},
	}
}

func machineApplicationArgs(name, environment string) []string {
	args := make([]string, 0, 3)
	if name = strings.TrimSpace(name); name != "" {
		args = append(args, name)
	}
	if environment = strings.TrimSpace(environment); environment != "" {
		args = append(args, "--environment", environment)
	}
	return args
}

func machineLifecycleContext(ctx context.Context) context.Context {
	// Once a mutating MCP lifecycle operation has been accepted, its
	// convergence must not be truncated merely because the client request
	// times out or disconnects. Preserve request values but detach cancellation;
	// lifecycle-specific verification timeouts still bound individual phases.
	ctx = context.WithoutCancel(ctx)
	opts := cli.OutputOptionsFromContext(ctx)
	opts.NonInteractive = true
	opts.Quiet = true
	opts.Plain = true
	return cli.WithOutputOptions(ctx, opts)
}

func machineMCPFailure(err error) (*mcp.CallToolResult, any, error) {
	classified := machine.Classify(classifyMachineCLIError(err))
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

func boolPointer(value bool) *bool {
	return &value
}
