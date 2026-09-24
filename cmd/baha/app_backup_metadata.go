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
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		return executeApplicationBackupWithMetadataLifecycle(ctx, store, args, out, errOut)
	}
	return command
}

func executeApplicationBackupWithMetadataLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
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
	resolved, err := resolveApplicationEnvironment(store, appArgs, "backup", environment)
	if err != nil {
		return err
	}
	if outputPath == "" {
		outputPath = fmt.Sprintf("%s-%s-%s.bhbackup", resolved.Manifest.Name, resolved.Manifest.Environment, time.Now().UTC().Format("20060102T150405Z"))
		filtered = append(append([]string(nil), filtered...), "--output", outputPath)
	}
	backupArgs := append([]string(nil), filtered...)
	if environment != "" {
		backupArgs = append(backupArgs, "--environment", environment)
	}
	if err := executeApplicationBackupLifecycle(ctx, store, backupArgs, out, errOut); err != nil {
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
		PostgresResources: application.SQLInstanceNames(resolved.Manifest),
		IncludesSecrets:   includesSecrets,
	}
	if err := resolved.Store.RecordLastBackup(metadata); err != nil {
		return fmt.Errorf("record successful backup metadata: %w", err)
	}
	return nil
}
