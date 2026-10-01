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
	if outputPath == "" {
		outputPath = fmt.Sprintf("%s-%s-%s.bhbackup", resolved.Manifest.Name, resolved.Manifest.Environment, time.Now().UTC().Format("20060102T150405Z"))
		filtered = append(append([]string(nil), filtered...), "--output", outputPath)
	}
	backupArgs := append([]string(nil), filtered...)
	for _, class := range selectionArgs.Include {
		backupArgs = append(backupArgs, "--include-state", string(class))
	}
	for _, class := range selectionArgs.Exclude {
		backupArgs = append(backupArgs, "--exclude-state", string(class))
	}
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
	if payload.Manifest.ApplicationID != resolved.Manifest.ApplicationID ||
		payload.Manifest.Application != resolved.Manifest.Name ||
		payload.Manifest.Environment != resolved.Manifest.Environment {
		return fmt.Errorf("record successful backup metadata: archive identity does not match application")
	}
	recoveryManifest, found, err := applicationbackup.RecoveryManifestFromPayload(payload)
	if err != nil {
		return fmt.Errorf("record successful backup metadata: validate recovery manifest: %w", err)
	}
	if !found {
		return fmt.Errorf("record successful backup metadata: recovery manifest is missing")
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
		ApplicationID:     resolved.Manifest.ApplicationID,
		DeploymentID:      resolved.DeploymentIdentity.DeploymentID,
		Application:       resolved.Manifest.Name,
		Environment:       resolved.Manifest.Environment,
		CreatedAt:         payload.Manifest.CreatedAt,
		ArchivePath:       absolutePath,
		PostgresResources: selectedRecoveryResources(recoveryManifest, applicationbackup.StateSQL),
		IncludesSecrets:   includesSecrets,
		Recovery:          recoveryContributorMetadata(recoveryManifest, true),
	}
	if err := resolved.Store.RecordLastBackup(metadata); err != nil {
		return fmt.Errorf("record successful backup metadata: %w", err)
	}
	return recordApplicationAudit(ctx, resolved, "backup", "success", "verified", "encrypted recovery unit created and reopened successfully")

}
