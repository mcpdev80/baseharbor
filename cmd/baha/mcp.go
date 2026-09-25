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

type machineInspectInput struct {
	Path string `json:"path,omitempty" jsonschema:"local repository path or Git URL; defaults to the current directory"`
}

type machineApplicationInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses BASEHARBOR_TARGET or configured default-target"`
	Name        string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
}

type machineUpdateInput struct {
	Target              string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
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

	mcp.AddTool(server, machineMCPTool("inspect", "Read-only repository inspection. Returns deterministic, secret-safe evidence and capability findings without changing repository or runtime state.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineInspectInput) (*mcp.CallToolResult, any, error) {
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

	mcp.AddTool(server, machineMCPTool("plan", "Read-only deterministic desired-state plan for the current repository or named application.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "plan")
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := application.BuildPlan(resolved.Manifest)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("apply", "Converge the complete selected BaseHarbor application lifecycle and return verified semantic status.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx = machineLifecycleContext(ctx)
		args := machineApplicationArgs(input.Name, input.Environment)
		if err := executeApplicationApplyLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		status, err := collectApplicationStatusResult(ctx, store, args)
		if err != nil {
			return machineMCPFailure(err)
		}
		if !status.Ready {
			return machineMCPFailure(&machine.Error{
				Code:        machine.ErrorVerificationFailed,
				CauseCode:   "post_apply_status_not_ready",
				Message:     "Application apply completed but the verified application status is not READY.",
				Resource:    status.Application,
				Remediation: "manual/admin action required",
				Next:        "Run baseharbor.doctor and resolve the reported degraded checks before retrying.",
			})
		}
		result := machineLifecycleStatusResult{
			Result: applicationlifecycle.NewResult("apply", status.Application, status.Environment, status.State, true),
			Status: status,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("status", "Read-only runtime and readiness observation for the current repository or named application.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		result, err := collectApplicationStatusResult(ctx, store, machineApplicationArgs(input.Name, input.Environment))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("doctor", "Read-only diagnostic verification for the current repository or named application.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		result, err := collectApplicationDoctor(ctx, store, machineApplicationArgs(input.Name, input.Environment))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("observe", "Return one secret-safe diagnostics view combining application readiness and doctor verification, including observability checks already supported by BaseHarbor.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		args := machineApplicationArgs(input.Name, input.Environment)
		status, err := collectApplicationStatusResult(ctx, store, args)
		if err != nil {
			return machineMCPFailure(err)
		}
		doctor, err := collectApplicationDoctor(ctx, store, args)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, machineObserveResult{
			ContractVersion: machine.ContractVersion,
			Application:     status.Application,
			Environment:     status.Environment,
			Status:          status,
			Doctor:          doctor,
		}, nil
	})

	mcp.AddTool(server, machineMCPTool("update", "Fast-forward the current Git-backed application safely, preserving the existing backup/recovery policy and full post-update verification.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineUpdateInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx = machineLifecycleContext(ctx)
		environment := strings.TrimSpace(input.Environment)
		resolved, err := resolveApplicationEnvironment(ctx, store, nil, "update", environment)
		if err != nil {
			return machineMCPFailure(err)
		}
		if applicationUpdateHasDurableState(resolved.Manifest) && strings.TrimSpace(input.BackupPasswordFile) == "" && !input.NoBackup {
			return machineMCPFailure(&machine.Error{
				Code:        machine.ErrorApprovalRequired,
				CauseCode:   "recovery_choice_required",
				Message:     "Updating an application with durable managed state requires an explicit recovery choice.",
				Resource:    resolved.Manifest.Name,
				Remediation: "requires operator approval",
				Next:        "Provide backup_password_file for an encrypted pre-update recovery point, or set no_backup=true to explicitly accept the risk.",
			})
		}
		args := make([]string, 0, 4)
		if environment != "" {
			args = append(args, "--environment", environment)
		}
		if passwordFile := strings.TrimSpace(input.BackupPasswordFile); passwordFile != "" {
			args = append(args, "--backup-password-file", passwordFile)
		}
		if input.NoBackup {
			args = append(args, "--no-backup")
		}
		if err := executeApplicationUpdateLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		status, err := collectApplicationStatusResult(ctx, store, machineApplicationArgs("", environment))
		if err != nil {
			return machineMCPFailure(err)
		}
		result := machineLifecycleStatusResult{
			Result: applicationlifecycle.NewResult("update", status.Application, status.Environment, status.State, status.Ready),
			Status: status,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("repair", "Run the existing guarded drift/doctor repair path. Only BaseHarbor-owned findings classified as safely repairable are mutated.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx = machineLifecycleContext(ctx)
		args := machineApplicationArgs(input.Name, input.Environment)
		args = append(args, "--fix")
		if err := executeApplicationRepairLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		doctorArgs := machineApplicationArgs(input.Name, input.Environment)
		doctor, err := collectApplicationDoctor(ctx, store, doctorArgs)
		if err != nil {
			return machineMCPFailure(err)
		}
		result := machineLifecycleDoctorResult{
			Result: applicationlifecycle.NewResult("repair", doctor.Application, doctor.Environment, doctor.State, doctor.Healthy),
			Doctor: doctor,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("backup", "Create the currently supported encrypted application recovery unit. Passwords are accepted only through an owner-only local file reference.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineBackupInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx = machineLifecycleContext(ctx)
		passwordFile := strings.TrimSpace(input.PasswordFile)
		if passwordFile == "" {
			return machineMCPFailure(machine.NewError(machine.ErrorValidationFailed, "password_file is required.", "Provide an owner-only local password file; plaintext backup passwords are never accepted through MCP.", false))
		}
		args := machineApplicationArgs(input.Name, input.Environment)
		args = append(args, "--password-file", passwordFile)
		if output := strings.TrimSpace(input.OutputPath); output != "" {
			args = append(args, "--output", output)
		}
		if err := executeApplicationBackupWithMetadataLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "backup")
		if err != nil {
			return machineMCPFailure(err)
		}
		metadata, err := resolved.Store.LastBackup(resolved.Manifest.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		result := machineBackupResult{
			Result: applicationlifecycle.NewResult("backup", resolved.Manifest.Name, resolved.Manifest.Environment, "backed_up", true),
			Backup: metadata,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("restore", "Restore and verify the currently supported encrypted application recovery unit. Passwords are accepted only through an owner-only local file reference.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRestoreInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx = machineLifecycleContext(ctx)
		backupPath := strings.TrimSpace(input.BackupPath)
		passwordFile := strings.TrimSpace(input.PasswordFile)
		if backupPath == "" || passwordFile == "" {
			return machineMCPFailure(machine.NewError(machine.ErrorValidationFailed, "backup_path and password_file are required.", "Provide the encrypted recovery archive and an owner-only local password file.", false))
		}
		args := []string{backupPath}
		if name := strings.TrimSpace(input.Name); name != "" {
			args = append(args, name)
		}
		args = append(args, "--password-file", passwordFile)
		if environment := strings.TrimSpace(input.Environment); environment != "" {
			args = append(args, "--environment", environment)
		}
		if err := executeApplicationRestoreLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		statusArgs := machineApplicationArgs(input.Name, input.Environment)
		status, err := collectApplicationStatusResult(ctx, store, statusArgs)
		if err == nil {
			result := machineLifecycleStatusResult{
				Result: applicationlifecycle.NewResult("restore", status.Application, status.Environment, status.State, status.Ready),
				Status: status,
			}
			return nil, result, nil
		}
		result := applicationlifecycle.NewResult("restore", strings.TrimSpace(input.Name), strings.TrimSpace(input.Environment), "restored", true)
		result.Detail = "Recovery unit restored and verified by the restore lifecycle; application identity can be discovered with baseharbor.status in repository context."
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("destroy", "Permanently remove BaseHarbor-owned application runtime resources and state. Explicit approval is mandatory and ownership verification remains fail-closed.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineDestroyInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		if err := applicationlifecycle.RequireApproval("destroy", input.Approval); err != nil {
			return machineMCPFailure(err)
		}
		ctx = machineLifecycleContext(ctx)
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "destroy")
		if err != nil {
			return machineMCPFailure(err)
		}
		args := machineApplicationArgs(input.Name, input.Environment)
		args = append(args, "--yes")
		if input.FullReset {
			args = append(args, "--full-reset")
		}
		if err := executeApplicationDestroyLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		result := applicationlifecycle.NewResult("destroy", resolved.Manifest.Name, resolved.Manifest.Environment, "destroyed", true)
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("policy.check", "Read-only typed policy evaluation for the selected application environment. Returns allow, warn or deny with secret-safe findings.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		result, err := collectApplicationPolicy(ctx, store, machineApplicationArgs(input.Name, ""), strings.TrimSpace(input.Environment))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("policy.explain", "Read-only explanation of effective environment policy defaults, rules and bounded operator overrides.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		result, err := explainApplicationPolicy(ctx, store, machineApplicationArgs(input.Name, ""), strings.TrimSpace(input.Environment))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

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
