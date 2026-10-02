package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
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

	mcp.AddTool(server, machineMCPTool("workspace.resolve", "Resolve canonical multi-repository component/source identity to the local XDG workspace mapping without changing source or runtime state.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineWorkspaceResolveInput) (*mcp.CallToolResult, any, error) {
		manifestPath, manifest, err := resolveWorkspaceManifest(input.Manifest)
		if err != nil {
			return machineMCPFailure(err)
		}
		model, _, err := development.LoadSourceModel(manifestPath)
		if err != nil {
			return machineMCPFailure(err)
		}
		mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		resolved, err := development.ResolveWorkspace(manifestPath, model, mapping)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, resolved, nil
	})

	mcp.AddTool(server, machineMCPTool("workspace.status", "Inspect Git state for every mapped repository source without changing checked-out revisions.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineWorkspaceStatusInput) (*mcp.CallToolResult, any, error) {
		manifestPath, manifest, err := resolveWorkspaceManifest(input.Manifest)
		if err != nil {
			return machineMCPFailure(err)
		}
		model, _, err := development.LoadSourceModel(manifestPath)
		if err != nil {
			return machineMCPFailure(err)
		}
		mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		status, err := development.InspectWorkspaceGit(ctx, model, mapping, input.Fetch)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, status, nil
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

	mcp.AddTool(server, machineMCPTool("provider.list", "List registered externally owned capability providers using the shared secret-safe provider state.", false), func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		result, err := application.ListExternalProviders()
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("provider.inspect", "Inspect one external provider registration without revealing credential or private-key material.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineProviderIDInput) (*mcp.CallToolResult, any, error) {
		result, err := application.InspectExternalProvider(strings.TrimSpace(input.ID))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("provider.verify", "Verify external provider reachability and configured TLS trust without mutation.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineProviderIDInput) (*mcp.CallToolResult, any, error) {
		result, err := application.VerifyExternalProvider(ctx, strings.TrimSpace(input.ID))
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("organization.inspect", "Inspect the active organization/platform source, immutable resolution and effective defaults with provenance.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineOrganizationInput) (*mcp.CallToolResult, any, error) {
		state, err := orgconfig.LoadActive()
		if err != nil {
			return machineMCPFailure(err)
		}
		effective, err := orgconfig.ResolveEffective(state, input.Environment)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, organizationView{ContractVersion: orgconfig.ContractVersion, State: state, Effective: effective}, nil
	})

	mcp.AddTool(server, machineMCPTool("organization.check", "Resolve the configured organization source and report a newer immutable digest/revision without changing the active configuration.", true), func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		status, available, err := orgconfig.Check(ctx)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, organizationCheckView{ContractVersion: orgconfig.ContractVersion, Status: status, Available: available}, nil
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

func registerMCPDevelopmentTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("workspace.update", "Safely fetch and fast-forward mapped Git repositories. Dirty, detached, ahead or diverged repositories are never modified.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineWorkspaceUpdateInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeMCPOperation(ctx, "workspace.update", "", "dev", "", input.Manifest); err != nil {
			return machineMCPFailure(err)
		}
		manifestPath, manifest, err := resolveWorkspaceManifest(input.Manifest)
		if err != nil {
			return machineMCPFailure(err)
		}
		model, _, err := development.LoadSourceModel(manifestPath)
		if err != nil {
			return machineMCPFailure(err)
		}
		mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := development.UpdateWorkspaceGit(ctx, model, mapping, input.Check)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("app.new", "Create and validate a new ecosystem-native application from portable capability intent. This writes only the generated application files and exposes no shell or runtime escape hatch.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineAppNewInput) (*mcp.CallToolResult, any, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return machineMCPFailure(usageError("application name is required", "Provide name explicitly."))
		}
		if err := authorizeMCPOperation(ctx, "app.new", "", input.Environment, name, ""); err != nil {
			return machineMCPFailure(err)
		}
		var (
			root string
			err  error
		)
		if strings.TrimSpace(input.Path) != "" {
			if strings.TrimSpace(input.Directory) != "" {
				return machineMCPFailure(usageError("path and directory cannot be combined", "Use directory for new clients; path is retained only for MCP compatibility."))
			}
			root, err = expandUserPath(input.Path)
			if err != nil {
				return machineMCPFailure(err)
			}
			if !filepath.IsAbs(root) {
				root, err = filepath.Abs(root)
				if err != nil {
					return machineMCPFailure(err)
				}
			}
		} else {
			root, err = resolveNewApplicationRoot(name, input.Directory)
			if err != nil {
				return machineMCPFailure(err)
			}
		}
		capabilities, err := developmentCapabilityKinds(input.Capabilities)
		if err != nil {
			return machineMCPFailure(err)
		}
		registry, err := referenceDevelopmentRegistry()
		if err != nil {
			return machineMCPFailure(err)
		}
		var adapterID string
		var profile *development.StackProfile
		stackProfile := strings.TrimSpace(input.StackProfile)
		if stackProfile == "" && strings.TrimSpace(input.Stack) == "" {
			stackProfile, err = organizationDefaultStack(input.Environment)
			if err != nil {
				return machineMCPFailure(err)
			}
		}
		if stackProfile != "" {
			if strings.TrimSpace(input.Stack) != "" {
				return machineMCPFailure(usageError("stack and stack_profile cannot be combined", "Select either one built-in stack or one reusable Stack Profile."))
			}
			catalog, err := effectiveDevelopmentProfileCatalog(".", registry)
			if err != nil {
				return machineMCPFailure(err)
			}
			resolved, err := development.ResolveStackProfile(stackProfile, development.ProfileMap(catalog))
			if err != nil {
				return machineMCPFailure(err)
			}
			profile = &resolved.Profile
		} else {
			adapterID, err = developmentAdapterID(input.Stack)
			if err != nil {
				return machineMCPFailure(err)
			}
		}
		result, err := development.CreateApplication(root, development.NewApplicationRequest{
			Name:               name,
			Environment:        strings.TrimSpace(input.Environment),
			Adapter:            adapterID,
			Profile:            profile,
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

}

func registerMCPLifecycleTools(server *mcp.Server, store application.Store) {
	registerMCPDevelopmentTools(server)

	registerMCPProviderOrganizationTools(server)

	mcp.AddTool(server, machineMCPTool("apply", "Converge the complete selected BaseHarbor application lifecycle and return verified semantic status. TIGHT host-memory headroom requires explicit skip_memory_preflight approval; hard failures remain enforced.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineApplyInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx = withMemoryPreflightOverride(ctx, input.SkipMemoryPreflight)
		args := machineApplicationArgs(input.Name, input.Environment)
		resolved, err := resolveApplication(ctx, store, args, "apply")
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := authorizeMCPOperation(ctx, "apply", resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.Name, ""); err != nil {
			return machineMCPFailure(err)
		}
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
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
		if err := authorizeMCPOperation(ctx, "update", resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.Name, ""); err != nil {
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
		args := machineApplicationArgs(input.Name, input.Environment)
		resolved, err := resolveApplication(ctx, store, args, "repair")
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := authorizeMCPOperation(ctx, "repair", resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.Name, ""); err != nil {
			return machineMCPFailure(err)
		}
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
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
			Result: applicationlifecycle.NewResult("repair", doctor.Application, doctor.Environment, doctor.State, doctor.Healthy).WithTarget(doctor.Target),
			Doctor: doctor,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("backup", "Create the currently supported encrypted application recovery unit. Passwords are accepted only through an owner-only local file reference.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineBackupInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
		passwordFile := strings.TrimSpace(input.PasswordFile)
		if passwordFile == "" {
			return machineMCPFailure(machine.NewError(machine.ErrorValidationFailed, "password_file is required.", "Provide an owner-only local password file; plaintext backup passwords are never accepted through MCP.", false))
		}
		args := machineApplicationArgs(input.Name, input.Environment)
		resolved, err := resolveApplication(ctx, store, args, "backup")
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := authorizeMCPOperation(ctx, "backup", resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.Name, ""); err != nil {
			return machineMCPFailure(err)
		}
		args = append(args, "--password-file", passwordFile)
		if output := strings.TrimSpace(input.OutputPath); output != "" {
			args = append(args, "--output", output)
		}
		for _, class := range input.IncludeState {
			args = append(args, "--include-state", class)
		}
		for _, class := range input.ExcludeState {
			args = append(args, "--exclude-state", class)
		}
		if err := executeApplicationBackupWithMetadataLifecycle(ctx, store, args, io.Discard, io.Discard); err != nil {
			return machineMCPFailure(err)
		}
		resolved, err = resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "backup")
		if err != nil {
			return machineMCPFailure(err)
		}
		metadata, err := resolved.Store.LastBackup(resolved.Manifest.Name)
		if err != nil {
			return machineMCPFailure(err)
		}
		result := machineBackupResult{
			Result: applicationlifecycle.NewResult("backup", resolved.Manifest.Name, resolved.Manifest.Environment, "backed_up", true).WithTarget(resolved.Target.Name),
			Backup: metadata,
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("restore", "Restore and verify the currently supported encrypted application recovery unit. Passwords are accepted only through an owner-only local file reference.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineRestoreInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
		backupPath := strings.TrimSpace(input.BackupPath)
		passwordFile := strings.TrimSpace(input.PasswordFile)
		if backupPath == "" || passwordFile == "" {
			return machineMCPFailure(machine.NewError(machine.ErrorValidationFailed, "backup_path and password_file are required.", "Provide the encrypted recovery archive and an owner-only local password file.", false))
		}
		if err := authorizeMCPOperation(ctx, "restore", input.Target, input.Environment, input.Name, ""); err != nil {
			return machineMCPFailure(err)
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
				Result: applicationlifecycle.NewResult("restore", status.Application, status.Environment, status.State, status.Ready).WithTarget(status.Target),
				Status: status,
			}
			return nil, result, nil
		}
		result := applicationlifecycle.NewResult("restore", strings.TrimSpace(input.Name), strings.TrimSpace(input.Environment), "restored", true)
		if target, targetErr := effectiveTarget(ctx); targetErr == nil {
			result = result.WithTarget(target.Name)
		}
		result.Detail = "Recovery unit restored and verified by the restore lifecycle; application identity can be discovered with baseharbor.status in repository context."
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("destroy", "Permanently remove BaseHarbor-owned application runtime resources and state. Explicit approval is mandatory and ownership verification remains fail-closed.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineDestroyInput) (*mcp.CallToolResult, any, error) {
		ctx = withTargetOverride(ctx, input.Target)
		if err := applicationlifecycle.RequireApproval("destroy", input.Approval); err != nil {
			return machineMCPFailure(err)
		}
		ctx, cancelLifecycle := machineLifecycleContext(ctx)
		defer cancelLifecycle()
		resolved, err := resolveApplication(ctx, store, machineApplicationArgs(input.Name, input.Environment), "destroy")
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := authorizeMCPOperation(ctx, "destroy", resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.Name, ""); err != nil {
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
		result := applicationlifecycle.NewResult("destroy", resolved.Manifest.Name, resolved.Manifest.Environment, "destroyed", true).WithTarget(resolved.Target.Name)
		return nil, result, nil
	})
}
