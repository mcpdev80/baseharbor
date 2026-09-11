package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func appRestoreCommandWithRecoveryMetadata(store application.Store) *cli.Command {
	command := appRestoreCommand(store)
	restoreRun := command.Run
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
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
			return fmt.Errorf("prepare recovery metadata: read application backup: %w", err)
		}
		defer zeroBytes(archive)
		payload, err := applicationbackup.Open(archive, password)
		if err != nil {
			return fmt.Errorf("prepare recovery metadata: validate application backup: %w", err)
		}
		m, err := applicationbackup.ApplicationManifestFromPayload(payload)
		if err != nil {
			return fmt.Errorf("prepare recovery metadata: validate application metadata: %w", err)
		}
		if name != "" && name != m.Name {
			return errors.New("restore target NAME does not match backup application identity")
		}
		resolved, err := resolveRestoreTarget(store, m)
		if err != nil {
			return err
		}
		absolutePath, err := filepath.Abs(backupPath)
		if err != nil {
			return fmt.Errorf("prepare recovery metadata: resolve archive path: %w", err)
		}

		if err := restoreRun(ctx, args, out, errOut); err != nil {
			return err
		}

		metadata := application.RecoveryMetadata{
			Version:         application.LastRecoveryMetadataVersion,
			Application:     m.Name,
			Environment:     m.Environment,
			RestoredAt:      time.Now().UTC(),
			BackupCreatedAt: payload.Manifest.CreatedAt,
			ArchivePath:     absolutePath,
		}
		if err := resolved.Store.RecordLastRecovery(metadata); err != nil {
			return fmt.Errorf("application was restored and verified but recording recovery metadata failed: %w", err)
		}
		fmt.Fprintln(out, "Status: READY")
		return nil
	}
	return command
}

func appShowCommandWithRecoveryMetadata(store application.Store) *cli.Command {
	command := appShowCommand(store)
	showRun := command.Run
	command.Long = "Shows application identity, backend readiness, repository workload state, secret readiness, the last recorded successful backup and the last verified recovery without revealing secret values or credential-bearing URLs. It uses the same repository workload readiness model as app status and app doctor."
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		resolved, err := resolveApplication(store, args, "show")
		if err != nil {
			return err
		}
		if err := showRun(ctx, args, out, errOut); err != nil {
			return err
		}
		fmt.Fprintln(out, "\nLast recovery")
		metadata, err := resolved.Store.LastRecovery(resolved.Manifest.Name)
		if errors.Is(err, application.ErrNoRecoveryMetadata) {
			fmt.Fprintln(out, "  not recorded yet")
			return nil
		}
		if err != nil {
			return err
		}
		if metadata.Environment != resolved.Manifest.Environment {
			fmt.Fprintln(out, "  not recorded for this environment")
			return nil
		}
		fmt.Fprintf(out, "  restored             %s\n", metadata.RestoredAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(out, "  backup created       %s\n", metadata.BackupCreatedAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(out, "  archive              %s\n", metadata.ArchivePath)
		fmt.Fprintln(out, "  verification         READY")
		return nil
	}
	return command
}
