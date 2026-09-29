package main

import (
	"context"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
	"github.com/mcpdev80/baseharbor/internal/development/nextjsadapter"
	"github.com/mcpdev80/baseharbor/internal/development/pythonadapter"
	"github.com/mcpdev80/baseharbor/internal/development/quarkusadapter"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func registerMCPReadTools(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("target", "Read-only inspection of the effective BaseHarbor target and repository-resolved deployment identity.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTargetInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		result, err := collectTargetInspection(ctx)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
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
			Target:          status.Target,
			Application:     status.Application,
			Environment:     status.Environment,
			Status:          status,
			Doctor:          doctor,
		}, nil
	})

	mcp.AddTool(server, machineMCPTool("evidence", "Export deterministic secret-safe lifecycle, policy, verification, recovery and audit evidence for the selected application.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		result, err := collectApplicationEvidence(ctx, store, machineApplicationArgs(input.Name, ""), strings.TrimSpace(input.Environment))
		if err != nil {
			return machineMCPFailure(err)
		}
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
}

func registerMCPLifecycleTools(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("app.new", "Create and validate a new ecosystem-native application from portable capability intent. This writes only the generated application files and exposes no shell or runtime escape hatch.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineAppNewInput) (*mcp.CallToolResult, any, error) {
		_ = ctx
		path := strings.TrimSpace(input.Path)
		if path == "" {
			path = "."
		}
		adapterID, err := developmentAdapterID(input.Stack)
		if err != nil {
			return machineMCPFailure(err)
		}
		capabilities, err := developmentCapabilityKinds(input.Capabilities)
		if err != nil {
			return machineMCPFailure(err)
		}
		registry, err := development.NewRegistry(goadapter.Adapter{}, nextjsadapter.Adapter{}, pythonadapter.Adapter{}, quarkusadapter.Adapter{})
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := development.CreateApplication(path, development.NewApplicationRequest{
			Name:               strings.TrimSpace(input.Name),
			Environment:        strings.TrimSpace(input.Environment),
			Adapter:            adapterID,
			Capabilities:       capabilities,
			Secrets:            append([]string(nil), input.Secrets...),
			EmitBackstage:      input.EmitBackstage,
			BackstageOwner:     input.BackstageOwner,
			BackstageLifecycle: input.BackstageLifecycle,
		}, registry)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, struct {
			ContractVersion string                      `json:"contract_version"`
			Application     string                      `json:"application"`
			Environment     string                      `json:"environment"`
			Profile         development.StackProfile    `json:"profile"`
			DevelopmentPlan development.DevelopmentPlan `json:"development_plan"`
			Files           []string                    `json:"files"`
			Validation      development.Validation      `json:"validation"`
		}{
			ContractVersion: machine.ContractVersion,
			Application:     result.Manifest.Name,
			Environment:     result.Manifest.Environment,
			Profile:         result.Profile,
			DevelopmentPlan: result.Plan,
			Files:           result.FilePaths,
			Validation:      result.Validation,
		}, nil
	})

	mcp.AddTool(server, machineMCPTool("apply", "Converge the complete selected BaseHarbor application lifecycle and return verified semantic status.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
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
			Result: applicationlifecycle.NewResult("apply", status.Application, status.Environment, status.State, true).WithTarget(status.Target),
			Status: status,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("update", "Fast-forward the current Git-backed application safely, preserving the existing backup/recovery policy and full post-update verification.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineUpdateInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
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
			Result: applicationlifecycle.NewResult("update", status.Application, status.Environment, status.State, status.Ready).WithTarget(status.Target),
			Status: status,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("repair", "Run the existing guarded drift/doctor repair path. Only BaseHarbor-owned findings classified as safely repairable are mutated.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplicationInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
		args := machineApplicationArgs(input.Name, input.Environment