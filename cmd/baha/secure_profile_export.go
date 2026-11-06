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
	"github.com/mcpdev80/baseharbor/internal/devaccess"
)

// secure_profile_export.go wires the bounded secure-profile export adapter into a
// public command. This file owns the CLI command only; the adapter it calls lives
// in internal/applicationbackup/secureprofile.go. The single registration/dispatch
// line for this command lives in cmd/baha/commands.go.

const maxExportPasswordFileBytes = 64 << 10

func secureProfileExportCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "secure-profile",
		Summary: "Encrypt explicitly selected local credentials into a password-locked export",
		Usage:   "baha app secure-profile export [NAME] --password-file FILE --output FILE",
		Long:    "Opt-in, password-encrypted export of the eligible local credentials for one application. Selects only the local developer credentials that exist for the target/environment; never exports remote provider secrets or fresh device identities by default. The export password is read from a file (never argv); the ciphertext is unreadable without it.",
		Children: []*cli.Command{
			{
				Name:    "export",
				Summary: "Encrypt selected local credentials into a password-locked export file",
				Usage:   "baha app secure-profile export [NAME] --password-file FILE [--output FILE]",
				Long:    "Opt-in, password-encrypted export of the eligible local credentials for one application. Selects only the local developer credentials that exist for the target/environment; never exports remote provider secrets or fresh device identities by default. The export password is read from a file (never argv); the ciphertext is unreadable without it.",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					return executeSecureProfileExport(ctx, store, args, out, errOut)
				},
			},
		},
	}
}

func executeSecureProfileExport(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	name, passwordPath, outputPath, err := parseSecureProfileExportArgs(args)
	if err != nil {
		return err
	}
	resolved, err := resolveSecretApplication(ctx, store, name, "app secure-profile export")
	if err != nil {
		return err
	}

	// Eligible local credentials: only the opt-in local developer credentials for
	// this target/environment. This is the bounded, explicit selection — no silent
	// fallback and no remote provider secrets.
	local, err := devaccess.Load(resolved.Target.Name, resolved.Manifest.Environment)
	if err != nil {
		return fmt.Errorf("read eligible local credentials for secure-profile export: %w", err)
	}

	password, err := readSecureProfileExportPassword(passwordPath)
	if err != nil {
		return err
	}
	defer zeroBytes(password)

	manifest := applicationbackup.ExportManifest{
		ApplicationID: resolved.Manifest.ApplicationID,
		Application:   resolved.Manifest.Name,
		Environment:   resolved.Manifest.Environment,
	}
	entries := []applicationbackup.SecretEntry{
		{Name: "developer-access/username", Data: []byte(local.Username)},
		{Name: "developer-access/password", Data: []byte(local.Password)},
	}
	envelope, err := applicationbackup.BuildExport(manifest, entries, password)
	if err != nil {
		return fmt.Errorf("encrypt secure-profile export: %w", err)
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("%s-%s-%s.bhexport", resolved.Manifest.Name, resolved.Manifest.Environment, time.Now().UTC().Format("20060102T150405Z"))
	}
	if err := writeSecureProfileExport(outputPath, envelope); err != nil {
		return err
	}
	fmt.Fprintf(out, "Exported local credentials for %s / %s / %s to %s (encrypted; needs the export password to open).\n", resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, outputPath)
	return nil
}

func parseSecureProfileExportArgs(args []string) (name, passwordPath, outputPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output":
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError("--output requires a file path", "Run 'baha app secure-profile export --help' for usage.")
			}
			outputPath = args[i]
		case "--password-file":
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError("--password-file requires a file path", "Never pass the export password directly on the command line.")
			}
			passwordPath = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", "", usageError("unknown option "+args[i], "Run 'baha app secure-profile export --help' for usage.")
			}
			if name != "" {
				return "", "", "", usageError("baha app secure-profile export accepts at most one NAME", "Inside an application repository omit NAME.")
			}
			name = args[i]
		}
	}
	if passwordPath == "" {
		return "", "", "", usageError("--password-file is required", "Store the export password in an owner-only file; it is never accepted through argv.")
	}
	return name, outputPath, passwordPath, nil
}

func readSecureProfileExportPassword(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect export password file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("export password file must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("export password file %s is accessible by group or others (%o)", path, info.Mode().Perm())
	}
	if info.Size() > maxExportPasswordFileBytes {
		return nil, errors.New("export password file is too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read export password file: %w", err)
	}
	data = []byte(strings.TrimRight(string(data), "\r\n"))
	if len(data) < 12 {
		zeroBytes(data)
		return nil, errors.New("export password must contain at least 12 bytes")
	}
	return data, nil
}

func writeSecureProfileExport(path string, envelope []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("export output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && filepath.Dir(path) != "." {
		return fmt.Errorf("create export output directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create export output without overwriting existing data: %w", err)
	}
	_, writeErr := file.Write(envelope)
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
