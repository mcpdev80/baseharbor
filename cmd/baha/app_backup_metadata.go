package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func appBackupCommandWithMetadata(store application.Store) *cli.Command {
	command := appBackupCommand(store)
	backupRun := command.Run
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
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
		if outputPath == "" {
			outputPath = fmt.Sprintf("%s-%s-%s.bhbackup", resolved.Manifest.Name, resolved.Manifest.Environment, time.Now().UTC().Format("20060102T150405Z"))
			args = append(append([]string(nil), args...), "--output", outputPath)
		}
		if err := backupRun(ctx, args, out, errOut); err != nil {
			return err
		}
		password, err := readBackupPasswordFile(passwordPath)
		if err != nil {
			return fmt.Errorf("record successful backup metadata: %w", err)
		}
		defer zeroBytes(password)
		archive, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("record successful backup metadata: read archive: %w", err)
		}
		defer zeroBytes(archive)
		payload, err := applicationbackup.Open(archive, password)
		if err != nil {
			return fmt.Errorf("record successful backup metadata: validate archive: %w", err)
		}
		if payload.Manifest.Application != resolved.Manifest.Name || payload.Manifest.Environment != resolved.Manifest.Environment {
			return fmt.Errorf("record successful backup metadata: archive identity does not match application")
		}
		absolutePath, err := filepath.Abs(outputPath)
		if err != nil {
			return fmt.Errorf("record successful backup metadata: resolve archive path: %w", err)
		}
		includesSecrets := false
		for _, entry := range payload.Manifest.Entries {
			if entry.Kind == "secrets" {
				includesSecrets = true
				break
			}
		}
		metadata := application.BackupMetadata{
			Version:           application.LastBackupMetadataVersion,
			Application:       resolved.Manifest.Name,
			Environment:       resolved.Manifest.Environment,
			CreatedAt:         payload.Manifest.CreatedAt,
			ArchivePath:       absolutePath,
			PostgresResources: application.PostgresInstanceNames(resolved.Manifest),
			IncludesSecrets:   includesSecrets,
		}
		if err := resolved.Store.RecordLastBackup(metadata); err != nil {
			return fmt.Errorf("record successful backup metadata: %w", err)
		}
		return nil
	}
	return command
}
