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
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const maxBackupPasswordFileBytes = 64 << 10

func appBackupCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "backup",
		Summary: "Create one encrypted recovery unit for an application",
		Usage:   "baha app backup [NAME] --password-file FILE [--output FILE]",
		Long:    "Quiesces the repository workload and per-application secret broker, captures desired application metadata, every managed PostgreSQL instance and the application-owned OpenBao secret scope, encrypts the complete recovery unit, then restarts the quiesced application components.",
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
		Long:    "Validates and decrypts the complete archive before mutation, rebuilds protected BaseHarbor application state, restores PostgreSQL and the matching OpenBao secret scope while the workload remains stopped, regenerates runtime identities, then starts and verifies the broker and repository workload.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationRestoreLifecycle(ctx, store, args, out, errOut)
		},
	}
}

func executeApplicationBackupLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	filtered, environment, err := extractApplicationEnvironment(args, "backup")
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
	if application.HasObjectStorage(m) {
		return errors.New("application backup does not yet include object-storage contents; refusing to create an incomplete recovery unit")
	}
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

	compose, err := detectComposeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityServiceExec, bhruntime.CapabilityResourceOwnership)
	if err != nil {
		return err
	}
	if err := verifyDesiredRuntimeServices(ctx, compose, m, files); err != nil {
		return fmt.Errorf("backup preflight runtime verification: %w", err)
	}
	var platformFiles bhruntime.Files
	if m.Services.Secrets {
		platformFiles, err = existingTargetRuntimeFiles(ctx)
		if err != nil {
			return err
		}
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

	captureErr := func() error {
		entries := make([]applicationbackup.PayloadEntry, 0, 2+len(application.SQLInstanceNames(m)))
		metadata, err := applicationbackup.ApplicationManifestPayloadEntry(m)
		if err != nil {
			return err
		}
		entries = append(entries, metadata)
		dumps, err := application.DumpPostgresInstances(ctx, compose, m, files)
		if err != nil {
			return err
		}
		postgresEntries, err := applicationbackup.PostgresPayloadEntries(dumps)
		if err != nil {
			return err
		}
		entries = append(entries, postgresEntries...)
		if m.Services.Secrets {
			identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
			secretBackup, err := openbao.ExportApplicationSecrets(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
			if err != nil {
				return err
			}
			secretEntry, err := applicationbackup.OpenBaoPayloadEntry(secretBackup)
			if err != nil {
				return err
			}
			entries = append(entries, secretEntry)
		}
		archive, err := applicationbackup.Build(m.Name, m.Environment, time.Now().UTC(), entries, password)
		if err != nil {
			return err
		}
		defer zeroBytes(archive)
		if err := writeBackupArchive(outputPath, archive); err != nil {
			return err
		}
		return nil
	}()

	restartErr := restartAfterBackup(ctx, compose, platformFiles, resolved, files, brokerStopped, workloadStopped, exposureStopped)
	if captureErr != nil || restartErr != nil {
		return errors.Join(captureErr, restartErr)
	}
	fmt.Fprintf(out, "Backup for %s (%s) written to %s.\n", m.Name, m.Environment, outputPath)
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
	archive, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read application backup: %w", err)
	}
	defer zeroBytes(archive)
	payload, err := applicationbackup.Open(archive, password)
	if err != nil {
		return fmt.Errorf("validate application backup before mutation: %w", err)
	}
	m, err := applicationbackup.ApplicationManifestFromPayload(payload)
	if err != nil {
		return fmt.Errorf("validate application metadata before mutation: %w", err)
	}
	if name != "" && name != m.Name {
		return errors.New("restore target NAME does not match backup application identity")
	}
	if environment != "" && environment != m.Environment {
		return fmt.Errorf("restore target environment %q does not match backup environment %q", environment, m.Environment)
	}
	if application.HasObjectStorage(m) {
		return errors.New("application restore does not yet restore object-storage contents; refusing an incomplete recovery")
	}
	postgresBackups, err := applicationbackup.PostgresBackupsFromPayload(m, payload)
	if err != nil {
		return fmt.Errorf("validate PostgreSQL backup before mutation: %w", err)
	}
	var secretBackup openbao.ApplicationSecretBackup
	if m.Services.Secrets {
		secretBackup, err = applicationbackup.OpenBaoBackupFromPayload(m.Name, m.Environment, payload)
		if err != nil {
			return fmt.Errorf("validate OpenBao backup before mutation: %w", err)
		}
	}

	resolved, err := resolveRestoreTarget(ctx, store, m)
	if err != nil {
		return err
	}
	compose, err := detectComposeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityServiceExec, bhruntime.CapabilityResourceOwnership)
	if err != nil {
		return err
	}
	var platformFiles bhruntime.Files
	var issuer serviceaccess.Issuer
	if requiresManagedServiceIssuer(m) || m.Services.Secrets {
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
	if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
		return err
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
	if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
		return err
	}
	if err := waitForManagedRuntime(ctx, compose, m, files); err != nil {
		return err
	}
	if err := application.RestorePostgresInstances(ctx, compose, m, files, postgresBackups); err != nil {
		return err
	}
	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		credentialsPath := openbao.ApplicationCredentialsPath(files.Dir)
		if err := openbao.RestoreApplicationSecrets(ctx, compose, platformFiles, identity, credentialsPath, secretBackup); err != nil {
			return err
		}
		if err := openbao.CheckApplicationScope(ctx, compose, platformFiles, identity, credentialsPath); err != nil {
			return fmt.Errorf("verify restored OpenBao scope: %w", err)
		}
	}
	if err := verifyDesiredRuntimeServices(ctx, compose, m, files); err != nil {
		return fmt.Errorf("verify restored PostgreSQL runtime: %w", err)
	}
	if application.RequiresRuntimeBroker(m) {
		if err := ensureAndStartRuntimeBroker(ctx, io.Discard, compose, platformFiles, m, files); err != nil {
			return err
		}
	}
	if _, err := applyRepositoryWorkload(ctx, out, compose, resolved, files); err != nil {
		_, _ = stopRepositoryWorkload(ctx, compose, resolved, files)
		return fmt.Errorf("start restored application workload: %w", err)
	}
	if err := convergeManagedExposure(ctx, out, preparedExposure); err != nil {
		_, _ = stopRepositoryWorkload(ctx, compose, resolved, files)
		return fmt.Errorf("restore managed HTTP exposure: %w", err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, m); err != nil {
		return fmt.Errorf("record provider registry after restore: %w", err)
	}
	fmt.Fprintf(out, "Application %s (%s) was restored and verified.\n", m.Name, m.Environment)
	return nil

}

func restartAfterBackup(ctx context.Context, compose bhruntime.Compose, platformFiles bhruntime.Files, resolved resolvedApplication, files application.RuntimeFiles, brokerStopped, workloadStopped, exposureStopped bool) error {
	var result error
	if brokerStopped {
		if err := ensureAndStartRuntimeBroker(ctx, io.Discard, compose, platformFiles, resolved.Manifest, files); err != nil {
			result = errors.Join(result, fmt.Errorf("restart application runtime broker after backup: %w", err))
		}
	}
	if workloadStopped && result == nil {
		if _, err := applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files); err != nil {
			result = errors.Join(result, fmt.Errorf("restart repository workload after backup: %w", err))
		}
	}
	if exposureStopped && result == nil {
		prepared, err := prepareManagedExposure(ctx, compose, resolved)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("prepare managed exposure after backup: %w", err))
		} else if err := convergeManagedExposure(ctx, io.Discard, prepared); err != nil {
			result = errors.Join(result, fmt.Errorf("restart managed exposure after backup: %w", err))
		}
	}
	return result
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
	id := deployment.DeploymentIdentity{
		Target:      target.Name,
		Application: backupManifest.Name,
		Environment: backupManifest.Environment,
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
		return resolved, nil
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

func resetRestoreTarget(ctx context.Context, compose bhruntime.Compose, platformFiles bhruntime.Files, resolved resolvedApplication) error {
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

func waitForManagedRuntime(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
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
