package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func appGuidedRestoreCommandWithRecoveryMetadata(store application.Store) *cli.Command {
	command := appRestoreCommandWithRecoveryMetadata(store)
	baseRun := command.Run
	command.Usage = "baha app restore BACKUP [NAME] [--password-file FILE]"
	command.Long = "Validates and decrypts the complete archive before mutation. In an interactive terminal, omitting --password-file starts a guided flow with hidden password entry, shows the backup identity, included durable resources and mutation impact, and requires confirmation before restore. Automation keeps the deterministic --password-file path. A successful restore reports READY only after verification and records the verified recovery metadata."
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if hasOption(args, "--password-file") {
			return baseRun(ctx, args, out, errOut)
		}
		if !appInitReaderIsTerminal(guidedBackupInput) {
			return usageError("interactive application restore requires a terminal when --password-file is omitted", "For CI/scripts use an owner-only --password-file; never pass the password itself through argv.")
		}

		backupPath, name, err := parseGuidedRestoreArgs(args)
		if err != nil {
			return err
		}
		password, err := guidedBackupReadPassword(out, false)
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
		if _, err := applicationbackup.PostgresBackupsFromPayload(m, payload); err != nil {
			return fmt.Errorf("validate PostgreSQL backup before mutation: %w", err)
		}

		formatRestorePreview(out, backupPath, m, payload.Manifest.CreatedAt, payload.Manifest.Entries)
		confirmed, err := promptGuidedConfirmation(guidedBackupInput, out, "Restore this backup and replace matching managed state?", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(out, "Restore cancelled; no state was changed.")
			return nil
		}

		return withInMemoryPasswordFile(password, func(passwordPath string) error {
			forwarded := append(append([]string(nil), args...), "--password-file", passwordPath)
			return baseRun(ctx, forwarded, out, errOut)
		})
	}
	return command
}
