package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const maxBackupPasswordFileBytes = 64 << 10

func appBackupCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "backup",
		Summary: "Create one encrypted recovery unit for an application",
		Usage:   "baha app backup [NAME] --password-file FILE [--output FILE] [--include-state CLASS] [--exclude-state CLASS]",
		Long:    "Discovers typed application recovery state, applies optional state-class selectors, quiesces the workload, captures selected BaseHarbor-owned durable state into one encrypted recovery unit, and explicitly records external or unsupported contributors.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationBackupLifecycle(ctx, store, args, out, errOut)
		},
	}
}

func appRestoreCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "restore",
		Summary: "Restore and verify an encrypted application recovery unit",
		Usage:   "baha app restore BACKUP [NAME] --password-file FILE",
		Long:    "Validates and decrypts the recovery unit before mutation, follows its typed recovery manifest, reconstructs ephemeral identities, restores selected durable state while the workload remains stopped, then runs final status and doctor verification.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationRestoreLifecycle(ctx, store, args, out, errOut)
		},
	}
}

func executeApplicationBackupLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	selectionFiltered, selectionArgs, err := extractRecoverySelectionArgs(args)
	if err != nil {
		return err
	}
	filtered, environment, err := extractApplicationEnvironment(selectionFiltered, "backup")
	if err != nil {
		return err
	}
	name, outputPath, passwordPath, err := parseAppBackupArgs(filtered)
	if err != nil {
		return err
	}
	var appArgs []string
	if name != "" {
		appArgs = []string{name}
	}
	resolved, err := resolveApplicationEnvironment(ctx, store, appArgs, "backup", environment)
	if err != nil {
		return err
	}
	m := resolved.Manifest
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		return err
	}
	password, err := readBackupPasswordFile(passwordPath)
	if err != nil {
		return err
	}
	defer zeroBytes(password)
	if outputPath == "" {
		outputPath = fmt.Sprintf("%s-%s-%s.bhbackup", m.Name, m.Environment, time.Now().UTC().Format("20060102T150405Z"))
	}

	compose, err := detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityServiceExec, bhruntime.CapabilityResourceOwnership)
	if err != nil {
		return err
	}
	if err := verifyDesiredRuntimeServices(ctx, compose, m, files); err != nil {
		return fmt.Errorf("backup preflight runtime verification: %w", err)
	}
	var platformFiles bhruntime.Files
	if m.Services.Secrets || application.RequiresRuntimeBroker(m) {
		platformFiles, err = existingTargetRuntimeFiles(ctx)
		if err != nil {
			return err
		}
	}
	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		if err := openbao.CheckApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir)); err != nil {
			return fmt.Errorf("backup preflight OpenBao verification: %w", err)
		}
	}
	exposureStopped := false
	if len(m.Exposures) > 0 {
		if _, err := inspectManagedExposure(ctx, compose, m, files); err != nil {
			return fmt.Errorf("backup preflight managed exposure verification: %w", err)
		}
		if err := stopManagedExposure(ctx, compose, m, files); err != nil {
			return err
		}
		exposureStopped = true
	}

	workloadStopped, err := stopRepositoryWorkload(ctx, compose, resolved, files)
	if err != nil {
		if exposureStopped {
			if prepared, prepareErr := prepareManagedExposure(ctx, compose, resolved); prepareErr == nil {
				_ = convergeManagedExposure(ctx, io.Discard, prepared)
			}
		}
		return err
	}
	brokerStopped := false
	if application.RequiresRuntimeBroker(m) {
		if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
			if workloadStopped {
				_, _ = applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files)
			}
			return err
		}
		brokerStopped = true
	}

	captureErr := captureApplicationBackup(ctx, compose, platformFiles, resolved, files, selectionArgs, password, outputPath)

	// Cancellation of archive capture must not cancel recovery of the stopped
	// runtime. Recovery is bounded and uses the normal application lifecycle.
	recoveryCtx, recoveryCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer recoveryCancel()
	restartErr := restartAfterBackup(recoveryCtx, compose, platformFiles, resolved, files, brokerStopped, workloadStopped, exposureStopped)
	if restartErr != nil {
		restartErr = fmt.Errorf("backup runtime recovery failed; run 'baha app up' for the same target/application/environment before retrying: %w", restartErr)
	}
	if captureErr != nil || restartErr != nil {
		return errors.Join(captureErr, restartErr)
	}
	fmt.Fprintf(out, "Backup for %s / %s / %s written to %s.\n", resolved.Target.Name, m.Name, m.Environment, outputPath)
	return nil

}

func executeApplicationRestoreLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	filtered, environment, err := extractApplicationEnvironment(args, "restore")
	if err != nil {
		return err
	}
	backupPath, name, passwordPath, err := parseAppRestoreArgs(filtered)
	if err != nil {
		return err
	}
	password, err := readBackupPasswordFile(passwordPath)
	if err != nil {
		return err
	}
	defer zeroBytes(password)
	restoreData, err := loadApplicationRestoreData(backupPath, password, name, environment)
	if err != nil {
		return err
	}
	m := restoreData.manifest

	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	if err := ensureOperatorAuthForBoundary(ctx, target.Name, m.Environment); err != nil {
		return err
	}

	resolved, err := resolveRestoreTarget(ctx, store, m)
	if err != nil {
		return err
	}
	compose, err := detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityServiceExec, bhruntime.CapabilityResourceOwnership)
	if err != nil {
		return err
	}
	return restoreApplicationState(ctx, store, out, resolved, compose, restoreData)

}

func restoreApplicationState(ctx context.Context, store application.Store, out io.Writer, resolved resolvedApplication, compose bhruntime.RuntimeProvider, restoreData applicationRestoreData) error {
	m := restoreData.manifest
	postgresBackups := restoreData.postgresBackups
	secretBackup := restoreData.secretBackup
	objectBackups := restoreData.objectStorage
	logsHistory := restoreData.logsHistory
	var err error
	var platformFiles bhruntime.Files
	var issuer serviceaccess.Issuer
	if requiresManagedServiceIssuer(m) || m.Services.Secrets || application.RequiresRuntimeBroker(m) {
		platformFiles, err = existingTargetRuntimeFiles(ctx)
		if err != nil {
			return fmt.Errorf("restore preflight BaseHarbor control plane: %w", err)
		}
	}
	if requiresManagedServiceIssuer(m) {
		issuer = openbao.NewServiceIssuer(compose, platformFiles)
		status, err := issuer.Status(ctx)
		if err != nil || !status.Ready {
			if err != nil {
				return fmt.Errorf("restore preflight managed service PKI: %w", err)
			}
			return errors.New("restore preflight managed service PKI is not ready")
		}
	}
	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		if err := openbao.CheckApplicationProvisioning(ctx, compose, platformFiles, identity); err != nil {
			return fmt.Errorf("restore preflight OpenBao provisioning: %w", err)
		}
	}
	if _, err := preflightRepositoryWorkloadSecurity(ctx, compose, resolved); err != nil {
		return fmt.Errorf("restore preflight workload security: %w", err)
	}
	if err := ensureRestoreDeploymentInitialization(ctx, resolved); err != nil {
		return fmt.Errorf("restore deployment initialization: %w", err)
	}
	preparedExposure, err := prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		return fmt.Errorf("restore preflight managed exposure: %w", err)
	}

	if err := resetRestoreTarget(ctx, compose, platformFiles, resolved); err != nil {
		return err
	}
	manifestPath, err := resolved.Store.Sync(m)
	if err != nil {
		return fmt.Errorf("recreate application state: %w", err)
	}
	if !resolved.FromRepository {
		resolved.ManifestPath = manifestPath
	}
	files, err := application.EnsureRuntime(ctx, issuer, resolved.Store, m)
	if err != nil {
		return err
	}
	project := files.Project
	if application.HasSharedBackends(m) {
		if _, err := application.ReconcileSharedBackends(
			ctx,
			compose,
			issuer,
			resolved.TargetStateRoot,
			resolved.Target.Name,
			m,
			files,
		); err != nil {
			return fmt.Errorf("recreate shared backend provider before restore: %w", err)
		}
	}
	if application.HasApplicationScopedRuntimeServices(m) {
		if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
			return err
		}
	}
	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		credentialsPath := openbao.ApplicationCredentialsPath(files.Dir)
		if err := openbao.EnsureApplicationScope(ctx, compose, platformFiles, identity, credentialsPath); err != nil {
			return fmt.Errorf("recreate OpenBao application scope: %w", err)
		}
		keys, err := openbao.ListApplicationSecretKeys(ctx, compose, platformFiles, identity, credentialsPath)
		if err != nil {
			return err
		}
		if len(keys) != 0 {
			return errors.New("restore target OpenBao scope is not empty; refusing PostgreSQL mutation")
		}
	}
	var preparedObjectStorage *managedObjectStorageExecution
	if recoveryManifestHasSelected(restoreData.recoveryManifest, applicationbackup.StateObjectStorage) {
		preparedObjectStorage, err = prepareManagedObjectStorage(ctx, compose, resolved, issuer)
		if err != nil {
			return fmt.Errorf("prepare object-storage recovery: %w", err)
		}
		if err := convergeManagedObjectStorage(ctx, io.Discard, preparedObjectStorage); err != nil {
			return fmt.Errorf("provision object-storage recovery target: %w", err)
		}
	}
	if application.HasApplicationScopedRuntimeServices(m) {
		if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
			return err
		}
	}
	if err := waitForManagedRuntime(ctx, compose, m, files); err != nil {
		return err
	}
	if application.HasSharedBackends(m) {
		if err := application.VerifySharedBackends(ctx, compose, resolved.TargetStateRoot, resolved.Target.Name, m); err != nil {
			return fmt.Errorf("verify shared backend provider before restore: %w", err)
		}
	}
	if application.RequiresRuntimeBroker(m) {
		if err := ensureAndStartRuntimeBroker(ctx, io.Discard, compose, platformFiles, m, files); err != nil {
			return fmt.Errorf("recreate application runtime broker before observability restore: %w", err)
		}
	}
	var preparedLogs *managedLogsExecution
	if recoveryManifestHasSelected(restoreData.recoveryManifest, applicationbackup.StateLogs) {
		preparedLogs, err = prepareManagedLogs(ctx, compose, resolved, issuer)
		if err != nil {
			return fmt.Errorf("prepare log-history recovery: %w", err)
		}
		if err := convergeManagedLogsBeforeWorkload(ctx, io.Discard, files, preparedLogs); err != nil {
			return fmt.Errorf("provision log-history recovery target: %w", err)
		}
		if err := logsprovider.RestoreApplicationHistoryAt(ctx, m, resolved.TargetStateRoot, resolved.Target.Name, logsHistory); err != nil {
			return fmt.Errorf("restore application log history: %w", err)
		}
	}
	if recoveryManifestHasSelected(restoreData.recoveryManifest, applicationbackup.StateSQL) {
		if err := application.RestorePostgresInstancesAt(ctx, compose, m, files, resolved.TargetStateRoot, resolved.Target.Name, postgresBackups); err != nil {
			return err
		}
	}
	if m.Services.Secrets && recoveryManifestHasSelected(restoreData.recoveryManifest, applicationbackup.StateSecrets) {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		credentialsPath := openbao.ApplicationCredentialsPath(files.Dir)
		if err := openbao.RestoreApplicationSecrets(ctx, compose, platformFiles, identity, credentialsPath, secretBackup); err != nil {
			return err
		}
		if err := openbao.CheckApplicationScope(ctx, compose, platformFiles, identity, credentialsPath); err != nil {
			return fmt.Errorf("verify restored OpenBao scope: %w", err)
		}
	}
	if preparedObjectStorage != nil {
		for _, backup := range objectBackups {
			if err := preparedObjectStorage.driver.RestoreBucket(ctx, backup); err != nil {
				return err
			}
		}
		if err := objectstorage.VerifyApplicationBucketsAt(ctx, compose, m, files, resolved.TargetStateRoot, resolved.Target.Name); err != nil {
			return fmt.Errorf("verify restored object storage: %w", err)
		}
	}
	if err := verifyDesiredRuntimeServices(ctx, compose, m, files); err != nil {
		return fmt.Errorf("verify restored PostgreSQL runtime: %w", err)
	}
	if err := restoreSelectedWorkloadStorage(ctx, compose, resolved, files, restoreData); err != nil {
		return err
	}
	if _, err := applyRepositoryWorkload(ctx, out, compose, resolved, files); err != nil {
		_, _ = stopRepositoryWorkload(ctx, compose, resolved, files)
		return fmt.Errorf("start restored application workload: %w", err)
	}
	if preparedLogs != nil {
		if err := verifyManagedLogsAfterWorkload(ctx, io.Discard, preparedLogs); err != nil {
			return fmt.Errorf("verify restored log history: %w", err)
		}
	}
	if err := convergeManagedExposure(ctx, out, preparedExposure); err != nil {
		_, _ = stopRepositoryWorkload(ctx, compose, resolved, files)
		return fmt.Errorf("restore managed HTTP exposure: %w", err)
	}
	if err := reconcileRestoredDevelopmentRoutes(ctx, out, resolved, compose, platformFiles, issuer, files); err != nil {
		return err
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, m); err != nil {
		return fmt.Errorf("record provider registry after restore: %w", err)
	}
	if err := recordAppliedDeployment(ctx, resolved, files); err != nil {
		return fmt.Errorf("record restored deployment: %w", err)
	}
	if err := verifyRestoredApplicationHealth(ctx, store, m); err != nil {
		return err
	}
	fmt.Fprintf(out, "Application %s / %s / %s was restored and verified.\n", resolved.Target.Name, m.Name, m.Environment)
	return nil

}

func ensureRestoreDeploymentInitialization(ctx context.Context, resolved resolvedApplication) error {
	if len(resolved.Manifest.Exposures) == 0 {
		return nil
	}
	state, err := loadRepositoryInitStateFromStateRoot(resolved.stateRoot())
	if err != nil {
		return err
	}
	if !restoreNeedsDeploymentInitialization(resolved.Manifest, state) {
		return nil
	}
	return runRepositoryRuntimeInitResolved(ctx, resolved, resolved.repositoryRoot(), repositoryInitOptions{Yes: true}, io.Discard)
}

func restoreNeedsDeploymentInitialization(m application.Manifest, state repositoryInitState) bool {
	return len(m.Exposures) > 0 &&
		devaccess.Enabled(m.Environment) &&
		strings.TrimSpace(state.Hostname) == ""
}

func verifyRestoredApplicationHealth(ctx context.Context, store application.Store, m application.Manifest) error {
	status, err := collectApplicationStatusResult(ctx, store, machineApplicationArgs(m.Name, m.Environment))
	if err != nil {
		return fmt.Errorf("final restore status verification: %w", err)
	}
	if !status.Ready {
		var failed []string
		for _, check := range status.Checks {
			if check.OK {
				continue
			}
			detail := strings.TrimSpace(check.Detail)
			if detail == "" {
				detail = strings.TrimSpace(check.State)
			}
			failed = append(failed, fmt.Sprintf("%s=%s", check.Name, detail))
		}
		if status.tlsErr != nil {
			failed = append(failed, fmt.Sprintf("tls=%v", status.tlsErr))
		} else if status.TLS != nil && !status.TLS.Healthy {
			failed = append(failed, fmt.Sprintf("tls=%s", strings.TrimSpace(status.TLS.Detail)))
		}
		if status.serviceTLSErr != nil {
			failed = append(failed, fmt.Sprintf("service-tls=%v", status.serviceTLSErr))
		} else {
			for _, observation := range status.ServiceTLS {
				if observation.Lifecycle.Health != "critical" && observation.Lifecycle.Health != "unknown" {
					continue
				}
				detail := strings.TrimSpace(observation.Lifecycle.Warning)
				if detail == "" {
					detail = observation.Lifecycle.Health
				}
				failed = append(failed, fmt.Sprintf("service-tls/%s/%s=%s", observation.Kind, observation.Instance, detail))
			}
		}
		if len(failed) == 0 {
			return errors.New("final restore status verification did not reach READY")
		}
		return fmt.Errorf("final restore status verification did not reach READY: %s", strings.Join(failed, "; "))
	}
	doctor, err := collectApplicationDoctor(ctx, store, machineApplicationArgs(m.Name, m.Environment))
	if err != nil {
		return fmt.Errorf("final restore doctor verification: %w", err)
	}
	if !doctor.Healthy {
		return errors.New("final restore doctor verification is not healthy")
	}
	return nil
}

func reconcileRestoredDevelopmentRoutes(ctx context.Context, out io.Writer, resolved resolvedApplication, compose bhruntime.RuntimeProvider, platformFiles bhruntime.Files, issuer serviceaccess.Issuer, files application.RuntimeFiles) error {
	m := resolved.Manifest
	if !requiresDevelopmentGateway(m) {
		return nil
	}
	routes := &applicationApplyExecution{
		resolved:      resolved,
		manifest:      m,
		term:          cli.NewTerminal(ctx, out, io.Discard),
		out:           out,
		errOut:        io.Discard,
		compose:       compose,
		platformFiles: platformFiles,
		issuer:        issuer,
		files:         files,
	}
	if err := routes.reconcileDevelopmentCanonicalRoutes(ctx); err != nil {
		_, _ = stopRepositoryWorkload(ctx, compose, resolved, files)
		return fmt.Errorf("restore canonical development routes: %w", err)
	}
	return nil
}

func restoreSelectedWorkloadStorage(ctx context.Context, compose bhruntime.RuntimeProvider, resolved resolvedApplication, files application.RuntimeFiles, restoreData applicationRestoreData) error {
	if !recoveryManifestHasSelected(restoreData.recoveryManifest, applicationbackup.StateWorkloadStorage) {
		return nil
	}
	_, targetVolumes, err := resolveRecoveryWorkloadStorage(ctx, compose, resolved, files, false)
	if err != nil {
		return fmt.Errorf("resolve workload storage recovery target: %w", err)
	}
	resolvedNames := make([]string, 0, len(targetVolumes))
	for _, volume := range targetVolumes {
		resolvedNames = append(resolvedNames, volume.Logical)
		archive, ok := restoreData.workloadStorage[volume.Logical]
		if !ok {
			return fmt.Errorf("workload recovery payload is missing %q", volume.Logical)
		}
		if err := compose.EnsureOwnedVolume(ctx, volume.Project, volume.Volume); err != nil {
			return err
		}
		if err := compose.RestoreOwnedVolume(ctx, volume.Project, volume.Volume, archive); err != nil {
			return err
		}
	}
	return validateRecoveredLogicalResources(restoreData.recoveryManifest, applicationbackup.StateWorkloadStorage, resolvedNames)
}

func resolveRestoreTarget(ctx context.Context, _ application.Store, backupManifest application.Manifest) (resolvedApplication, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return resolvedApplication{}, err
	}
	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return resolvedApplication{}, err
	}
	existing, deploymentFound, err := deployment.FindDeployment(target.Name, backupManifest.ApplicationID, backupManifest.Environment)
	if err != nil {
		return resolvedApplication{}, err
	}
	var id deployment.DeploymentIdentity
	if deploymentFound {
		id = existing.Identity
		id.Application = backupManifest.Name
	} else {
		id, err = deployment.NewDeploymentIdentity(target.Name, backupManifest.ApplicationID, backupManifest.Name, backupManifest.Environment)
		if err != nil {
			return resolvedApplication{}, err
		}
	}
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		return resolvedApplication{}, err
	}
	resolved := resolvedApplication{
		Target:              target,
		DeploymentIdentity:  id,
		Manifest:            backupManifest,
		TargetStateRoot:     targetRoot,
		DeploymentStateRoot: deploymentRoot,
		Store:               application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name},
	}

	cwd, err := os.Getwd()
	if err != nil {
		return resolved, err
	}
	found, err := application.HasRepositoryApplication(cwd)
	if err != nil {
		return resolved, err
	}
	if !found {
		var record deployment.DeploymentRecord
		var recordErr error
		if deploymentFound {
			record = existing
			record.Identity = id
		} else {
			recordErr = os.ErrNotExist
		}
		if recordErr == nil && deployment.SourceAvailable(record.Source) {
			selection, sourceErr := application.ResolveRepositoryEnvironment(record.Source.Repository, backupManifest.Environment)
			if sourceErr != nil {
				return resolved, fmt.Errorf("restore preflight registered source: %w", sourceErr)
			}
			if selection.Manifest.YAML() != backupManifest.YAML() {
				return resolved, errors.New("registered repository environment manifest does not match backup desired state")
			}
			resolved.DeploymentRecord = &record
			resolved.ManifestPath = selection.ManifestPath
			resolved.RepositoryRoot = selection.RepositoryRoot
			resolved.SourceAvailable = true
			resolved.FromRepository = true
			return resolved, nil
		}
		if recordErr != nil && !errors.Is(recordErr, os.ErrNotExist) {
			return resolved, fmt.Errorf("restore preflight deployment record: %w", recordErr)
		}
		return resolved, errors.New("restore preflight source repository is unavailable; run restore from the matching repository or restore the registered repository path before mutation")
	}
	selection, err := application.ResolveRepositoryEnvironment(cwd, backupManifest.Environment)
	if err != nil {
		return resolved, err
	}
	if selection.Manifest.YAML() != backupManifest.YAML() {
		return resolved, errors.New("selected repository environment manifest does not match backup desired state")
	}
	resolved.ManifestPath = selection.ManifestPath
	resolved.RepositoryRoot = selection.RepositoryRoot
	resolved.SourceAvailable = true
	resolved.FromRepository = true
	return resolved, nil
}

func resetRestoreTarget(ctx context.Context, compose bhruntime.RuntimeProvider, platformFiles bhruntime.Files, resolved resolvedApplication) error {
	m := resolved.Manifest
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		if errors.Is(err, application.ErrRuntimeNotApplied) {
			return nil
		}
		return err
	}
	if err := destroyManagedExposure(ctx, compose, m, files); err != nil {
		return err
	}
	if _, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
		return err
	}
	if application.RequiresRuntimeBroker(m) {
		if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
			return err
		}
	}
	if application.HasSharedBackends(m) {
		if err := application.ReleaseSharedBackendApplication(ctx, compose, resolved.TargetStateRoot, resolved.Target.Name, m); err != nil {
			return fmt.Errorf("release previous shared backend application resources before restore: %w", err)
		}
	}
	if err := compose.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("destroy previous managed backend before restore: %w", err)
	}
	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		if err := openbao.DestroyVerifiedApplicationScope(ctx, compose, platformFiles, identity); err != nil {
			return fmt.Errorf("destroy previous OpenBao scope before restore: %w", err)
		}
	}
	if err := resolved.Store.Delete(m.Name); err != nil {
		return err
	}
	return nil
}

func waitForManagedRuntime(ctx context.Context, compose bhruntime.RuntimeProvider, m application.Manifest, files application.RuntimeFiles) error {
	verifyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var last error
	for verifyCtx.Err() == nil {
		last = verifyDesiredRuntimeServices(verifyCtx, compose, m, files)
		if last == nil {
			return nil
		}
		select {
		case <-verifyCtx.Done():
		case <-time.After(time.Second):
		}
	}
	if last == nil {
		last = verifyCtx.Err()
	}
	return fmt.Errorf("restored managed runtime did not become ready: %w", last)
}

func parseAppBackupArgs(args []string) (name, outputPath, passwordPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output":
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError("--output requires a file path", "Run 'baha app backup --help' for usage.")
			}
			outputPath = args[i]
		case "--password-file":
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError("--password-file requires a file path", "Never pass a backup password directly on the command line.")
			}
			passwordPath = args[i]
		case "--include-state", "--exclude-state":
			option := args[i]
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError(option+" requires a recovery state class", "Use a typed recovery state class such as database.sql or object-storage.s3.")
			}
			if _, parseErr := applicationbackup.ParseRecoveryStateClass(args[i]); parseErr != nil {
				return "", "", "", parseErr
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", "", usageError("unknown option "+args[i], "Run 'baha app backup --help' for usage.")
			}
			if name != "" {
				return "", "", "", usageError("baha app backup accepts at most one NAME", "Inside an application repository omit NAME.")
			}
			name = args[i]
		}
	}
	if passwordPath == "" {
		return "", "", "", usageError("--password-file is required", "Store the backup password in an owner-only file; it is never accepted through argv.")
	}
	return name, outputPath, passwordPath, nil
}

func parseAppRestoreArgs(args []string) (backupPath, name, passwordPath string, err error) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--password-file" {
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError("--password-file requires a file path", "Never pass a backup password directly on the command line.")
			}
			passwordPath = args[i]
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			return "", "", "", usageError("unknown option "+args[i], "Run 'baha app restore --help' for usage.")
		}
		if backupPath == "" {
			backupPath = args[i]
			continue
		}
		if name == "" {
			name = args[i]
			continue
		}
		return "", "", "", usageError("baha app restore accepts BACKUP and optional NAME", "Run 'baha app restore --help' for usage.")
	}
	if backupPath == "" || passwordPath == "" {
		return "", "", "", usageError("BACKUP and --password-file are required", "Run 'baha app restore --help' for usage.")
	}
	return backupPath, name, passwordPath, nil
}

func readBackupPasswordFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect backup password file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("backup password file must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("backup password file %s is accessible by group or others (%o)", path, info.Mode().Perm())
	}
	if info.Size() > maxBackupPasswordFileBytes {
		return nil, errors.New("backup password file is too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read backup password file: %w", err)
	}
	data = []byte(strings.TrimRight(string(data), "\r\n"))
	if len(data) < 12 {
		zeroBytes(data)
		return nil, errors.New("backup password must contain at least 12 bytes")
	}
	return data, nil
}

func writeBackupArchive(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("backup output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && filepath.Dir(path) != "." {
		return fmt.Errorf("create backup output directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create backup archive without overwriting existing data: %w", err)
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	return os.Chmod(path, 0o600)
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
