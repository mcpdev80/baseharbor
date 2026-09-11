package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

var guidedBackupInput io.Reader = os.Stdin
var guidedBackupReadPassword = readBackupPasswordFromTerminal

func appGuidedBackupCommand(store application.Store) *cli.Command {
	command := appBackupCommandWithMetadata(store)
	baseRun := command.Run
	command.Usage = "baha app backup [NAME] [--output FILE] [--password-file FILE]"
	command.Long = "Creates one encrypted application recovery unit. In an interactive terminal, omitting --password-file starts a guided flow with hidden password entry and safe output defaults. Automation keeps using an owner-only --password-file; backup passwords are never accepted as command-line values."
	command.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if hasOption(args, "--password-file") {
			return baseRun(ctx, args, out, errOut)
		}
		if !appInitReaderIsTerminal(guidedBackupInput) {
			return usageError("interactive application backup requires a terminal when --password-file is omitted", "For CI/scripts use an owner-only --password-file; never pass the password itself through argv.")
		}

		name, outputPath, err := parseGuidedBackupArgs(args)
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
		if outputPath == "" {
			outputPath = fmt.Sprintf("%s-%s-%s.bhbackup", m.Name, m.Environment, time.Now().UTC().Format("20060102T150405Z"))
		}

		formatBackupPreview(out, m, outputPath)
		confirmed, err := promptGuidedConfirmation(guidedBackupInput, out, "Create backup now?", true)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(out, "Backup cancelled; no state was changed.")
			return nil
		}

		password, err := guidedBackupReadPassword(out, true)
		if err != nil {
			return err
		}
		defer zeroBytes(password)
		return withInMemoryPasswordFile(password, func(passwordPath string) error {
			forwarded := append([]string(nil), args...)
			if !hasOption(forwarded, "--output") {
				forwarded = append(forwarded, "--output", outputPath)
			}
			forwarded = append(forwarded, "--password-file", passwordPath)
			return baseRun(ctx, forwarded, out, errOut)
		})
	}
	return command
}

func appGuidedRestoreCommand(store application.Store) *cli.Command {
	command := appRestoreCommand(store)
	baseRun := command.Run
	command.Usage = "baha app restore BACKUP [NAME] [--password-file FILE]"
	command.Long = "Validates and decrypts the complete archive before mutation. In an interactive terminal, omitting --password-file starts a guided flow with hidden password entry, shows the backup identity, included durable resources and mutation impact, and requires confirmation before restore. Automation keeps the deterministic --password-file path."
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

func parseGuidedBackupArgs(args []string) (name, outputPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return "", "", usageError("--output requires a file path", "Run 'baha app backup --help' for usage.")
			}
			outputPath = args[i]
		case "--password-file":
			return "", "", usageError("guided backup parser received --password-file", "Use the deterministic password-file path instead.")
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", usageError("unknown option "+args[i], "Run 'baha app backup --help' for usage.")
			}
			if name != "" {
				return "", "", usageError("baha app backup accepts at most one NAME", "Inside an application repository omit NAME.")
			}
			name = args[i]
		}
	}
	return name, outputPath, nil
}

func parseGuidedRestoreArgs(args []string) (backupPath, name string, err error) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return "", "", usageError("unknown option "+arg, "Run 'baha app restore --help' for usage.")
		}
		if backupPath == "" {
			backupPath = arg
			continue
		}
		if name == "" {
			name = arg
			continue
		}
		return "", "", usageError("baha app restore accepts BACKUP and optional NAME", "Run 'baha app restore --help' for usage.")
	}
	if strings.TrimSpace(backupPath) == "" {
		return "", "", usageError("BACKUP is required", "Run 'baha app restore --help' for usage.")
	}
	return backupPath, name, nil
}

func hasOption(args []string, option string) bool {
	for _, arg := range args {
		if arg == option {
			return true
		}
	}
	return false
}

func formatBackupPreview(out io.Writer, m application.Manifest, outputPath string) {
	fmt.Fprintln(out, "Application backup")
	fmt.Fprintf(out, "  Application: %s\n", m.Name)
	fmt.Fprintf(out, "  Environment: %s\n", m.Environment)
	fmt.Fprintf(out, "  Output: %s\n", outputPath)
	postgres := application.PostgresInstanceNames(m)
	if len(postgres) == 0 {
		fmt.Fprintln(out, "  PostgreSQL: none")
	} else {
		fmt.Fprintf(out, "  PostgreSQL: %s\n", strings.Join(postgres, ", "))
	}
	if m.Services.Secrets {
		fmt.Fprintln(out, "  Managed secrets: included")
	} else {
		fmt.Fprintln(out, "  Managed secrets: none")
	}
	fmt.Fprintln(out, "  Impact: repository workload and secret broker may be stopped briefly for a consistent snapshot.")
	fmt.Fprintln(out, "  Encryption: password entered with terminal echo disabled; the password is never placed in argv.")
}

func formatRestorePreview(out io.Writer, backupPath string, m application.Manifest, createdAt time.Time, entries []applicationbackup.Entry) {
	absolutePath, err := filepath.Abs(backupPath)
	if err != nil {
		absolutePath = backupPath
	}
	fmt.Fprintln(out, "Restore preview")
	fmt.Fprintf(out, "  Application: %s\n", m.Name)
	fmt.Fprintf(out, "  Environment: %s\n", m.Environment)
	fmt.Fprintf(out, "  Backup: %s\n", absolutePath)
	fmt.Fprintf(out, "  Created: %s\n", createdAt.UTC().Format(time.RFC3339))
	postgres := application.PostgresInstanceNames(m)
	sort.Strings(postgres)
	if len(postgres) == 0 {
		fmt.Fprintln(out, "  PostgreSQL: none")
	} else {
		fmt.Fprintf(out, "  PostgreSQL: %s\n", strings.Join(postgres, ", "))
	}
	includesSecrets := false
	for _, entry := range entries {
		if entry.Kind == "secrets" {
			includesSecrets = true
			break
		}
	}
	if includesSecrets {
		fmt.Fprintln(out, "  Managed secrets: included")
	} else {
		fmt.Fprintln(out, "  Managed secrets: none")
	}
	fmt.Fprintln(out, "  Impact: matching managed backends, secret scope and repository workload are stopped/recreated as required by restore.")
	fmt.Fprintln(out, "  Verification: archive integrity is already validated; runtime identities are regenerated and readiness must pass before success is reported.")
}

func promptGuidedConfirmation(input io.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	file, ok := input.(*os.File)
	if !ok {
		return false, errors.New("guided confirmation requires terminal input")
	}
	prompt := " [y/N]: "
	if defaultYes {
		prompt = " [Y/n]: "
	}
	fmt.Fprint(out, label+prompt)
	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	value := strings.ToLower(strings.TrimSpace(line))
	if value == "" {
		return defaultYes, nil
	}
	switch value {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errors.New("confirmation must be yes or no")
	}
}

func readBackupPasswordFromTerminal(out io.Writer, confirm bool) ([]byte, error) {
	file, ok := guidedBackupInput.(*os.File)
	if !ok {
		return nil, errors.New("secure backup password entry requires a terminal")
	}
	first, err := readHiddenTerminalLine(file, out, "Backup password: ")
	if err != nil {
		return nil, err
	}
	if len(first) < 12 {
		zeroBytes(first)
		return nil, errors.New("backup password must contain at least 12 bytes")
	}
	if !confirm {
		return first, nil
	}
	second, err := readHiddenTerminalLine(file, out, "Confirm backup password: ")
	if err != nil {
		zeroBytes(first)
		return nil, err
	}
	defer zeroBytes(second)
	if !bytes.Equal(first, second) {
		zeroBytes(first)
		return nil, errors.New("backup password confirmation does not match")
	}
	return first, nil
}

func readHiddenTerminalLine(file *os.File, out io.Writer, prompt string) ([]byte, error) {
	fd := int(file.Fd())
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, fmt.Errorf("inspect terminal settings for secure password entry: %w", err)
	}
	original := *termios
	hidden := *termios
	hidden.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &hidden); err != nil {
		return nil, fmt.Errorf("disable terminal echo for secure password entry: %w", err)
	}
	defer func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, &original) }()

	fmt.Fprint(out, prompt)
	reader := bufio.NewReader(file)
	line, readErr := reader.ReadBytes('\n')
	fmt.Fprintln(out)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		zeroBytes(line)
		return nil, readErr
	}
	line = bytes.TrimRight(line, "\r\n")
	if len(line) > maxBackupPasswordFileBytes {
		zeroBytes(line)
		return nil, errors.New("backup password is too large")
	}
	return line, nil
}

func withInMemoryPasswordFile(password []byte, fn func(string) error) error {
	fd, err := unix.MemfdCreate("baseharbor-backup-password", unix.MFD_CLOEXEC)
	if err != nil {
		return fmt.Errorf("create in-memory backup password file: %w", err)
	}
	file := os.NewFile(uintptr(fd), "baseharbor-backup-password")
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("create in-memory backup password file handle")
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect in-memory backup password file: %w", err)
	}
	if _, err := file.Write(password); err != nil {
		return fmt.Errorf("write in-memory backup password file: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind in-memory backup password file: %w", err)
	}
	path := fmt.Sprintf("/proc/self/fd/%d", fd)
	return fn(path)
}
