package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/development"
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
	case "installation.destroy":
		return e.executeHTTPInstallationDestroy(ctx, operationContext, raw)
	case "app.stop":
		return e.executeHTTPApplicationStop(ctx, operationContext, raw, report)
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

func (e *bahaMachineExecutor) executeHTTPInstallationDestroy(ctx context.Context, operationContext machine.OperationContext, raw json.RawMessage) (any, error) {
	var input struct {
		Approval              bool                               `json:"approval"`
		ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
	}
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	if operationContext.Environment == "" || operationContext.Target == "" {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Installation destroy requires an explicit Target and environment scope.", "Use the selected Core Target and the dev operator session.", false)
	}
	ctx = withTargetOverride(ctx, operationContext.Target)
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkBoundCoreTarget(ctx, target); err != nil {
		return nil, err
	}
	if err := authorizeCurrentMCPContext(ctx, "installation.destroy", operationContext.Target, operationContext.Environment, ""); err != nil {
		return nil, err
	}
	ctx = withProviderChangeConsent(ctx, input.ProviderChangeConsent)
	ctx, report := withDestroyReport(ctx, true)
	if err := destroyInstallation(ctx, input.Approval, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	return report, nil
}

// Normalize the verified uploaded source under the already authorized Core
// identity before provider consent checks. Revalidation must compare the same
// canonical workload and provider projection that apply actually consumes.
func prepareVerifiedHTTPSource(ctx context.Context, resolved resolvedApplication, selection application.RepositoryEnvironmentSelection) (resolvedApplication, error) {
	if selection.Manifest.Name != resolved.Manifest.Name ||
		selection.Manifest.Environment != resolved.Manifest.Environment ||
		(selection.Manifest.ApplicationID != "" && selection.Manifest.ApplicationID != resolved.DeploymentIdentity.ApplicationID) {
		return resolvedApplication{}, machine.NewError(machine.ErrorPolicyDenied, "Source snapshot does not belong to the authorized Core application.", "Use the matching application identity.", false)
	}
	selection.Manifest.ApplicationID = resolved.DeploymentIdentity.ApplicationID
	selection, err := resolveRepositoryLifecycleIntent(ctx, resolved.Target, selection, "plan")
	if err != nil {
		return resolvedApplication{}, err
	}
	resolved.Manifest = selection.Manifest
	resolved.ManifestPath = selection.ManifestPath
	resolved.RepositoryRoot = selection.RepositoryRoot
	resolved.SourceAvailable = true
	resolved.FromRepository = true
	resolved.Manifest, err = resolveAppliedBackendSelection(resolved, "apply")
	if err != nil {
		return resolvedApplication{}, err
	}
	return resolved, nil
}

// Preserve the typed machine error code while making the failing Core stage
// observable through the verified Machine HTTP response. A plain fmt.Errorf
// wrapper is lost when machine.Classify unwraps an existing typed error.
func coreHTTPApplyStageError(stage string, err error) error {
	classified := machine.Classify(err)
	if classified == nil {
		return nil
	}
	staged := *classified
	staged.Message = "Core remote apply " + stage + ": " + classified.Message
	staged.Cause = err
	return &staged
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
	if input.SourceModel != nil {
		if input.SourceModel.Application != input.Name {
			return nil, machine.NewError(machine.ErrorValidationFailed, "Remote source selection does not belong to the selected application.", "Use the matching source model for this registered application.", false)
		}
		if err := development.ValidateImmutableRemoteSources(*input.SourceModel); err != nil {
			return nil, machine.Wrap(machine.ErrorValidationFailed, err, "Use immutable Git commit SHAs or OCI image digests.", false)
		}
	}
	ctx = withTargetOverride(ctx, input.Target)
	ctx = withMemoryPreflightOverride(ctx, input.SkipMemoryPreflight)
	ctx = withCandidateConsentDigest(ctx, input.ConfirmCandidate)
	ctx = withProviderChangeConsent(ctx, input.ProviderChangeConsent)
	if len(input.WorkloadPorts) > 32 {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Too many workload port selections were supplied.", "Choose ports only for variables used by the selected Compose workload.", false)
	}
	for name, value := range input.WorkloadPorts {
		if !validWorkloadEnvironmentName(name) {
			return nil, machine.NewError(machine.ErrorValidationFailed, "A workload port selection has an invalid variable name.", "Use the exact variable name from the selected Compose workload.", false)
		}
		if _, err := parsePort(value); err != nil {
			return nil, machine.NewError(machine.ErrorValidationFailed, "A workload port selection is invalid.", "Choose a TCP port from 1 through 65535.", false)
		}
	}
	ctx = withRequestedWorkloadPorts(ctx, input.WorkloadPorts)
	args := machineApplicationArgs(input.Name, input.Environment)

	reportHTTPProgress(report, "resolve", "Resolving application and target.", 10)
	resolved, err := resolveApplication(ctx, e.store, args, "apply")
	if err != nil {
		return nil, coreHTTPApplyStageError("resolve registered application", err)
	}
	if err := ensureResolvedHTTPContext(operationContext, resolved); err != nil {
		return nil, err
	}
	if input.SourceSnapshotDigest != "" {
		if input.SourceModel != nil || e.sourceTransfers == nil {
			return nil, machine.NewError(machine.ErrorConflict, "Snapshot source conflicts with an unbound immutable source.", "Select one Core-verified source transport.", false)
		}
		manifest, root, err := e.sourceTransfers.Current(resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment)
		if err != nil || manifest.Digest != input.SourceSnapshotDigest {
			return nil, machine.NewError(machine.ErrorSourceMissing, "The selected source snapshot is missing or no longer current.", "Upload the exact source bytes and retry.", false)
		}
		selection, err := application.ResolveRepositoryEnvironment(root, resolved.Manifest.Environment)
		if err != nil {
			return nil, machine.Wrap(machine.ErrorValidationFailed, err, "The uploaded snapshot needs a valid baseharbor.yaml.", false)
		}
		if selection.Manifest.Name != resolved.Manifest.Name || selection.Manifest.Environment != resolved.Manifest.Environment ||
			(selection.Manifest.ApplicationID != "" && selection.Manifest.ApplicationID != resolved.DeploymentIdentity.ApplicationID) {
			return nil, machine.NewError(machine.ErrorPolicyDenied, "Source snapshot does not belong to the authorized Core application.", "Use the matching application identity.", false)
		}
		resolved, err = prepareVerifiedHTTPSource(ctx, resolved, selection)
		if err != nil {
			return nil, err
		}
		ctx = withVerifiedHTTPSource(ctx, resolved)
	} else {
		// Registered immutable sources retain their existing Core-side checks.
		input.SourceModel, err = bindRegisteredCoreSource(resolved, input.SourceModel)
		if err != nil {
			return nil, machine.Wrap(machine.ErrorConflict, err,
				"Use the immutable source registered on this Core application; a source change needs an explicit authorized operation.", false)
		}
	}
	if input.SourceModel != nil {
		// A Core-registered clean Git worktree at the exact commit can be
		// used through the existing source lifecycle. Do not fetch URLs
		// provided by the workstation or reinterpret OCI as local files.
		if len(input.SourceModel.Sources) != 1 || resolved.DeploymentRecord == nil {
			return nil, machine.NewError(machine.ErrorUnsupported, "Remote source requires one registered Core-owned immutable Git checkout.", "Register the exact Git source with the owning Core before apply.", false)
		}
		candidate, err := prepareStagedCoreGitCandidate(ctx,
			input.SourceModel.Sources[0], resolved.DeploymentRecord.Source,
			resolved.DeploymentIdentity, resolved.Manifest)
		if err != nil {
			return nil, machine.Wrap(machine.ErrorSourceMissing, err, "Ensure the Core-registered Git checkout matches the selected remote, commit and application identity.", false)
		}
		defer candidate.Close()
		if err := confirmStagedCoreGitCandidate(candidate, input.ConfirmCandidate); err != nil {
			return nil, machine.Wrap(machine.ErrorApprovalRequired, err,
				"Confirm the exact staged Git candidate returned by the owning Core before deployment.", false)
		}
		// The staged bytes and operator consent are now verified together.
		// The existing apply lifecycle still reads its registered source
		// directly, not this staged tree: do not allow a TOCTOU bypass or
		// falsely imply that the runtime consumed the approved artifact.
		return nil, machine.NewError(machine.ErrorUnsupported,
			"Pinned Git candidate and consent verified, but immutable Connector delivery is not yet bound to this artifact.",
			"Bind this exact staged tree to the existing Core-owned deployment transaction before applying.", false)
	}

	reportHTTPProgress(report, "apply", "Converging desired application state.", 35)
	ctx = context.WithValue(ctx, applicationPhaseReporterKey{}, func(label string) {
		reportHTTPProgress(report, "apply", strings.Replace(label, "Reconciling ", "Deploying ", 1), 0)
	})
	if err := executeApplicationApplyLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		if errors.Is(err, errConsentRequiredForCandidateSubmission) {
			// The first attempt is a read-only consent discovery: no pending
			// deployment was written. Return the exact Core-side candidate hash
			// so a human or automation can explicitly authorize a second attempt.
			submission, _ := candidateSubmissionForResolvedApp(ctx, resolved)
			if hash := delivery.CandidateConsentDigest(submission); hash != "" {
				preview, previewErr := buildResolvedApplicationPlan(ctx, resolved)
				if previewErr != nil {
					return nil, coreHTTPApplyStageError("deployment preview", previewErr)
				}
				return nil, &machine.Error{
					Code:      machine.ErrorApprovalRequired,
					CauseCode: "candidate_confirmation_required",
					Message:   "Core requires explicit confirmation of this exact application source candidate.",
					Details:   preview,
					Next:      "Re-run baha app apply " + input.Name + " --environment " + input.Environment + " --confirm-candidate " + hash,
				}
			}
		}
		return nil, coreHTTPApplyStageError("execute lifecycle", err)
	}
	reportHTTPProgress(report, "verify", "Verifying application readiness.", 85)
	status, err := collectApplicationStatusResult(ctx, e.store, args)
	if err != nil {
		return nil, coreHTTPApplyStageError("verify status", err)
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
	if err := rejectUnboundRegisteredSourceMutation(resolved, "updated"); err != nil {
		return nil, machine.Wrap(machine.ErrorUnsupported, err,
			"Use the Core-owned immutable source/candidate workflow before attempting an update.", false)
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
	// Local Git update works on a checked-out repository, never on an
	// uploaded workstation snapshot. Refuse unsupported remote sources before
	// running the local update parser or attempting a Git fetch.
	if _, err := inspectGitApplicationUpdate(ctx, resolved.repositoryRoot(), false); err != nil {
		return nil, machine.Wrap(machine.ErrorUnsupported, err,
			"Remote app update requires a Core-owned Git worktree. For uploaded Compose sources, submit a new verified source candidate with app apply.", false)
	}
	// Name is an authenticated selector, not a positional option of the
	// repository-local Git update command.
	args := machineApplicationArgs("", input.Environment)
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
	if err := rejectUnboundRegisteredSourceMutation(resolved, "repaired"); err != nil {
		return nil, machine.Wrap(machine.ErrorUnsupported, err,
			"Use the Core-owned immutable source/candidate workflow before attempting repair.", false)
	}
	reportHTTPProgress(report, "repair", "Repairing safely reconcilable drift.", 40)
	if err := executeApplicationRepairLifecycle(ctx, e.store, append(args, "--fix", "--yes"), io.Discard, io.Discard); err != nil {
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
	ctx = withProviderChangeConsent(ctx, input.ProviderChangeConsent)
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
	ctx = withProviderChangeConsent(ctx, input.ProviderChangeConsent)
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
	args = append(args, "--full-reset")
	reportHTTPProgress(report, "destroy", "Removing BaseHarbor-owned application resources.", 40)
	if err := executeApplicationDestroyLifecycle(ctx, e.store, args, io.Discard, io.Discard); err != nil {
		return nil, err
	}
	reportHTTPProgress(report, "destroy", "Owned application resources removed.", 100)
	return applicationDestroyResult(resolved, true)
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
