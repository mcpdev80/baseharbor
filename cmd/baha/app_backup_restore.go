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
)

const maxBackupPasswordFileBytes = 64 << 10

func appBackupCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "backup",
		Summary: "Create one encrypted recovery unit for an application",
		Usage:   "baha app backup [NAME] --password-file FILE [--output FILE]",
		Long:    "Quiesces the repository workload and per-application secret broker, captures desired application metadata, every managed PostgreSQL instance and the application-owned OpenBao secret scope, encrypts the complete recovery unit, then restarts the quiesced application components.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			name, outputPath, passwordPath, err := parseAppBackupArgs(args)
			if err != nil {
				return err
			}
			var appArgs []string
			if name != "" {
				appArgs = []string{name}
			}
			resolved, err := resolveApplication(store, appArgs, "backup")
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

			compose, err := bhruntime.DetectCompose(ctx)
			if err != nil {
				return err
			}
			if err := verifyDesiredRuntimeServices(ctx, compose, m, files); err != nil {
				return fmt.Errorf("backup preflight runtime verification: %w", err)
			}
			var platformFiles bhruntime.Files
			if m.Services.Secrets {
				platformFiles, err = bhruntime.ExistingFiles("")
				if err != nil {
					return err
				}
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				if err := openbao.CheckApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir)); err != nil {
					return fmt.Errorf("backup preflight OpenBao verification: %w", err)
				}
			}

			workloadStopped, err := stopRepositoryWorkload(ctx, compose, resolved, files)
			if err != nil {
				return err
			}
			brokerStopped := false
			if m.Services.Secrets {
				if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
					if workloadStopped {
						_, _ = applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files)
					}
					return err
				}
				brokerStopped = true
			}

			captureErr := func() error {
				entries := make([]applicationbackup.PayloadEntry, 0, 2+len(application.PostgresInstanceNames(m)))
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

			restartErr := restartAfterBackup(ctx, compose, platformFiles, resolved, files, brokerStopped, workloadStopped)
			if captureErr != nil || restartErr != nil {
				return errors.Join(captureErr, restartErr)
			}
			fmt.Fprintf(out, "Backup for %s (%s) written to %s.\n", m.Name, m.Environment, outputPath)
			return nil
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
			backupPath, name, passwordPath, err := parseAppRestoreArgs(args)
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

			resolved, err := resolveRestoreTarget(store, m)
			if err != nil {
				return err
			}
			compose, err := bhruntime.DetectCompose(ctx)
			if err != nil {
				return err
			}
			var platformFiles bhruntime.Files
			if m.Services.Secrets {
				platformFiles, err = bhruntime.ExistingFiles("")
				if err != nil {
					return fmt.Errorf("restore preflight BaseHarbor control plane: %w", err)
				}
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				if err := openbao.CheckApplicationProvisioning(ctx, compose, platformFiles, identity); err != nil {
					return fmt.Errorf("restore preflight OpenBao provisioning: %w", err)
				}
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
			files, err := application.EnsureRuntime(resolved.Store, m)
			if err != nil {
				return err
			}
			project := application.RuntimeProjectName(m)
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
			if m.Services.Secrets {
				if err := ensureAndStartRuntimeBroker(ctx, compose, platformFiles, m, files); err != nil {
					return err
				}
			}
			if _, err := applyRepositoryWorkload(ctx, out, compose, resolved, files); err != nil {
				_, _ = stopRepositoryWorkload(ctx, compose, resolved, files)
				return fmt.Errorf("start restored application workload: %w", err)
			}
			fmt.Fprintf(out, "Application %s (%s) was restored and verified.\n", m.Name, m.Environment)
			return nil
		},
	}
}

func restartAfterBackup(ctx context.Context, compose bhruntime.Compose, platformFiles bhruntime.Files, resolved resolvedApplication, files application.RuntimeFiles, brokerStopped, workloadStopped bool) error {
	var result error
	if brokerStopped {
		if err := ensureAndStartRuntimeBroker(ctx, compose, platformFiles, resolved.Manifest, files); err != nil {
			result = errors.Join(result, fmt.Errorf("restart secret broker after backup: %w", err))
		}
	}
	if workloadStopped {
		if _, err := applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files); err != nil {
			result = errors.Join(result, fmt.Errorf("restart repository workload after backup: %w", err))
		}
	}
	return result
}

func resolveRestoreTarget(store application.Store, backupManifest application.Manifest) (resolvedApplication, error) {
	resolved := resolvedApplication{Manifest: backupManifest, Store: store}
	cwd, err := os.Getwd()
	if err != nil {
		return resolved, err
	}
	path, err := application.FindRepositoryManifest(cwd)
	if err != nil {
		return resolved, nil
	}
	repositoryManifest, err := application.LoadManifestFile(path)
	if err != nil {
		return resolved, err
	}
	if repositoryManifest.YAML() != backupManifest.YAML() {
		return resolved, errors.New("repository baseharbor.yaml does not match backup desired state")
	}
	resolved.Store = application.Store{Root: filepath.Join(filepath.Dir(path), ".baseharbor", "apps")}
	resolved.ManifestPath = path
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
	if _, err := stopRepositoryWorkload(ctx, compose, resolved, files); err != nil {
		return err
	}
	if m.Services.Secrets {
		if err := stopRuntimeBroker(ctx, compose, m, files); err != nil {
			return err
		}
	}
	if err := compose.DestroyProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env); err != nil {
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
