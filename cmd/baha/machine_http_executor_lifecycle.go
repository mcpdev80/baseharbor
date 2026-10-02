package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

func (e *bahaMachineExecutor) executeHTTPLifecycle(
	ctx context.Context,
	operationID string,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	switch operationID {
	case "apply":
		return e.executeHTTPApply(ctx, operationContext, raw, report)
	case "update":
		return e.executeHTTPUpdate(ctx, operationContext, raw, report)
	case "repair":
		return e.executeHTTPRepair(ctx, operationContext, raw, report)
	case "backup":
		return e.executeHTTPBackup(ctx, operationContext, raw, report)
	case "restore":
		return e.executeHTTPRestore(ctx, operationContext, raw, report)
	case "destroy":
		return e.executeHTTPDestroy(ctx, operationContext, raw, report)
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Unsupported lifecycle operation.", "Use machine discovery.", false)
	}
}

func (e *bahaMachineExecutor) executeHTTPApply(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineApplyInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationSelectors(operationContext, &input.Target, &input.Environment, &input.Name); err != nil {
		return nil, err
	}
	ctx = withTargetOverride(ctx, input.Target)
	ctx = withMemoryPreflightOverride(ctx, input.SkipMemoryPreflight)
	args := machineApplicationArgs(input.Name, input.Environment)

	reportHTTPProgress(report, "resolve", "Resolving application and target.", 10)
	resolved, err := resolveApplication(ctx, e.store, args, "apply")
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}

	reportHTTPProgress(report, "apply", "Converging desired application state.", 35)
	if err := executeApplicationApplyLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "verify", "Verifying application readiness.", 85)
	status, err := collectApplicationStatusResult(ctx, e.store, args)
	if err != nil {
		return nil, err
	}
	if !status.Ready {
		return nil, &machine.Error{
			Code:        machine.ErrorVerificationFailed,
			CauseCode:   "post_apply_status_not_ready",
			Message:     "Application apply completed but the verified application status is not READY.",
			Resource:    status.Application,
			Remediation: "manual/admin action required",
			Next:        "Run doctor and resolve degraded checks before retrying.",
		}
	}
	reportHTTPProgress(report, "verify", "Application is READY.", 100)
	return machineLifecycleStatusResult{
		Result: applicationlifecycle.NewResult("apply", status.Application, status.Environment, status.State, true).WithTarget(status.Target),
		Status: status,
	}, nil
}

func (e *bahaMachineExecutor) executeHTTPUpdate(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineUpdateInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	name := operationContext.Application
	if err := bindHTTPApplicationSelectors(operationContext, &input.Target, &input.Environment, &name); err != nil {
		return nil, err
	}
	ctx = withTargetOverride(ctx, input.Target)
	resolved, err := resolveApplicationEnvironment(ctx, e.store, machineApplicationArgs(name, ""), "update", input.Environment)
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}
	if applicationUpdateHasDurableState(resolved.Manifest) && strings.TrimSpace(input.BackupPasswordFile) == "" && !input.NoBackup {
		return nil, &machine.Error{
			Code:        machine.ErrorApprovalRequired,
			CauseCode:   "recovery_choice_required",
			Message:     "Updating an application with durable managed state requires an explicit recovery choice.",
			Resource:    resolved.Manifest.Name,
			Remediation: "requires operator approval",
			Next:        "Provide backup_password_file or explicitly set no_backup=true.",
		}
	}
	args := machineApplicationArgs(name, input.Environment)
	if passwordFile := strings.TrimSpace(input.BackupPasswordFile); passwordFile != "" {
		args = append(args, "--backup-password-file", passwordFile)
	}
	if input.NoBackup {
		args = append(args, "--no-backup")
	}
	reportHTTPProgress(report, "update", "Updating source and reconciling application.", 35)
	if err := executeApplicationUpdateLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "verify", "Verifying updated application.", 85)
	status, err := collectApplicationStatusResult(ctx, e.store, machineApplicationArgs(name, input.Environment))
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "verify", "Update verification completed.", 100)
	return machineLifecycleStatusResult{
		Result: applicationlifecycle.NewResult("update", status.Application, status.Environment, status.State, status.Ready).WithTarget(status.Target),
		Status: status,
	}, nil
}

func (e *bahaMachineExecutor) executeHTTPRepair(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineApplicationInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationInput(operationContext, &input); err != nil {
		return nil, err
	}
	ctx = withTargetOverride(ctx, input.Target)
	args := machineApplicationArgs(input.Name, input.Environment)
	resolved, err := resolveApplication(ctx, e.store, args, "repair")
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "repair", "Repairing safely reconcilable drift.", 40)
	if err := executeApplicationRepairLifecycle(ctx, e.store, append(args, "--fix"), io.Discard, io.Discard); err != nil {
		return nil, err
	}
	doctor, err := collectApplicationDoctor(ctx, e.store, args)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "verify", "Repair verification completed.", 100)
	return machineLifecycleDoctorResult{
		Result: applicationlifecycle.NewResult("repair", doctor.Application, doctor.Environment, doctor.State, doctor.Healthy).WithTarget(doctor.Target),
		Doctor: doctor,
	}, nil
}

func (e *bahaMachineExecutor) executeHTTPBackup(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineBackupInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationSelectors(operationContext, &input.Target, &input.Environment, &input.Name); err != nil {
		return nil, err
	}
	passwordFile := strings.TrimSpace(input.PasswordFile)
	if passwordFile == "" {
		return nil, machine.NewError(machine.ErrorValidationFailed, "password_file is required.", "Provide an owner-only local password file reference.", false)
	}
	ctx = withTargetOverride(ctx, input.Target)
	args := machineApplicationArgs(input.Name, input.Environment)
	resolved, err := resolveApplication(ctx, e.store, args, "backup")
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
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
	reportHTTPProgress(report, "backup", "Creating encrypted recovery unit.", 40)
	if err := executeApplicationBackupWithMetadataLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	metadata, err := resolved.Store.LastBackup(resolved.Manifest.Name)
	if err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "verify", "Backup recovery unit verified.", 100)
	return machineBackupResult{
		Result: applicationlifecycle.NewResult("backup", resolved.Manifest.Name, resolved.Manifest.Environment, "backed_up", true).WithTarget(resolved.Target.Name),
		Backup: metadata,
	}, nil
}

func (e *bahaMachineExecutor) executeHTTPRestore(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineRestoreInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationSelectors(operationContext, &input.Target, &input.Environment, &input.Name); err != nil {
		return nil, err
	}
	backupPath := strings.TrimSpace(input.BackupPath)
	passwordFile := strings.TrimSpace(input.PasswordFile)
	if backupPath == "" || passwordFile == "" {
		return nil, machine.NewError(machine.ErrorValidationFailed, "backup_path and password_file are required.", "Provide the encrypted archive and owner-only password-file reference.", false)
	}
	ctx = withTargetOverride(ctx, input.Target)
	args := []string{backupPath}
	if input.Name != "" {
		args = append(args, input.Name)
	}
	args = append(args, "--password-file", passwordFile)
	if input.Environment != "" {
		args = append(args, "--environment", input.Environment)
	}
	reportHTTPProgress(report, "restore", "Restoring encrypted recovery unit.", 35)
	if err := executeApplicationRestoreLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	status, err := collectApplicationStatusResult(ctx, e.store, machineApplicationArgs(input.Name, input.Environment))
	if err != nil {
		return nil, err
	}
	if operationContext.Target != "" && status.Target != operationContext.Target {
		return nil, httpContextMismatch("target")
	}
	reportHTTPProgress(report, "verify", "Restore verification completed.", 100)
	return machineLifecycleStatusResult{
		Result: applicationlifecycle.NewResult("restore", status.Application, status.Environment, status.State, status.Ready).WithTarget(status.Target),
		Status: status,
	}, nil
}

func (e *bahaMachineExecutor) executeHTTPDestroy(
	ctx context.Context,
	operationContext machine.OperationContext,
	raw json.RawMessage,
	report machinehttp.ProgressReporter,
) (any, error) {
	var input machineDestroyInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if err := bindHTTPApplicationSelectors(operationContext, &input.Target, &input.Environment, &input.Name); err != nil {
		return nil, err
	}
	if err := applicationlifecycle.RequireApproval("destroy", input.Approval); err != nil {
		return nil, err
	}
	ctx = withTargetOverride(ctx, input.Target)
	resolved, err := resolveApplication(ctx, e.store, machineApplicationArgs(input.Name, input.Environment), "destroy")
	if err != nil {
		return nil, err
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}
	args := machineApplicationArgs(input.Name, input.Environment)
	args = append(args, "--yes")
	if input.FullReset {
		args = append(args, "--full-reset")
	}
	reportHTTPProgress(report, "destroy", "Removing BaseHarbor-owned application resources.", 40)
	if err := executeApplicationDestroyLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "destroy", "Owned application resources removed.", 100)
	return applicationlifecycle.NewResult("destroy", resolved.Manifest.Name, resolved.Manifest.Environment, "destroyed", true).WithTarget(resolved.Target.Name), nil
}

func ensureResolvedHTTPContext(operationContext machine.OperationContext, resolved resolvedApplication) error {
	if operationContext.Target != "" && operationContext.Target != resolved.Target.Name {
		return httpContextMismatch("target")
	}
	if operationContext.Environment != "" && operationContext.Environment != resolved.Manifest.Environment {
		return httpContextMismatch("environment")
	}
	if operationContext.Application != "" && operationContext.Application != resolved.Manifest.Name {
		return httpContextMismatch("application")
	}
	return nil
}

func httpContextMismatch(resource string) error {
	return &machine.Error{
		Code:      machine.ErrorPolicyDenied,
		CauseCode: "operation_context_mismatch",
		Message:   "Resolved BaseHarbor state does not match the authorized HTTP operation context.",
		Resource:  resource,
		Next:      "Refresh application context and retry with matching stable identity.",
	}
}
