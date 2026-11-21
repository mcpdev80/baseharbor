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

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
)

// app_export.go wires the bounded, explicitly selected local credential export
// into a public command. This file owns the CLI command and its selection flow;
// the encrypted adapter it builds on lives in
// internal/applicationbackup/secureprofile.go, and the single registration line
// lives in cmd/baha/commands.go.
//
// Scope (0428-065 / t-20261114-r28-cli2-app-export): this is the `app data
// export` surface from the reviewed CLI contract — a selectable encrypted export
// that is distinct from `app data backup`/`app data restore` (typed recovery
// units) and from `app evidence` (secret-safe lifecycle bundle). The export is
// fail-closed by construction: it encrypts only the local entries a caller
// explicitly selects, never leaks the export password through argv, and never
// carries remote provider secrets, fresh device identities, or managed secret
// values unless they pass the fail-closed human-authorization reveal seam
// (operatorauth.RequireHumanOperator) the base already delivers. There are no
// silent fallback fields: an export with zero selected sources fails closed.

// ErrExportEmptySelection fails closed when a caller asks to export without
// selecting at least one entry. There are no silent fallback fields.
var ErrExportEmptySelection = errors.New("app export requires at least one selected source")

// appExportInput and appExportReadPassword keep the guided flow testable; they
// mirror the pattern used by the guided backup command.
var (
	appExportInput       io.Reader = os.Stdin
	appExportReadPassword = readExportPasswordFromTerminal
)

func appExportCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "export",
		Summary: "Export selected local application credentials into an encrypted file",
		Usage:   "baha app export [NAME] [--include STATE]... --password-file FILE [--output FILE]",
		Long: "Opt-in, password-encrypted export of the eligible local credentials that exist for one application.\n" +
			"Selects only the local developer credentials that exist for the target/environment; never exports remote\n" +
			"provider secrets or fresh device identities by default, and never carries managed secret values unless the\n" +
			"caller passes the fail-closed human-authorization reveal seam. The export password is read from a file\n" +
			"(never argv); the ciphertext is unreadable without it. An export without any selected source fails closed.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationExport(ctx, store, args, out, errOut)
		},
	}
}

// appExportSource is one eligible, explicitly selected local export source. It
// never carries a raw secret value in transport or audit metadata: only a
// destination-relative logical name is visible before the caller confirms the
// selection, and the entry Data enters the encrypted envelope only after
// selection.
type appExportSource struct {
	// StateClass is the human-facing source id reported to the caller and used
	// as the --include selector (e.g. "local.developer-credentials").
	StateClass string
	// Description is a safe, non-secret explanation printed during selection.
	Description string
	// Available reports whether the material for this source actually exists on
	// this target/environment. An unavailable source is reported, never silently
	// dropped, and never selected by default.
	Available bool
	// NeedsHumanReveal marks a source whose entries carry a managed secret value
	// that must pass the fail-closed human-authorization reveal seam before it
	// enters the envelope.
	NeedsHumanReveal bool
}

func executeApplicationExport(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	selectionFiltered, includeStates, err := extractExportSelectionArgs(args)
	if err != nil {
		return err
	}
	filtered, environment, err := extractApplicationEnvironment(selectionFiltered, "export")
	if err != nil {
		return err
	}
	name, outputPath, passwordPath, err := parseAppExportArgs(filtered)
	if err != nil {
		return err
	}
	var appArgs []string
	if name != "" {
		appArgs = []string{name}
	}
	resolved, err := resolveApplicationEnvironment(ctx, store, appArgs, "export", environment)
	if err != nil {
		return err
	}
	return executeResolvedApplicationExport(ctx, resolved, includeStates, outputPath, passwordPath, out, newAppExportSecretReader)
}

type appExportSecretReader interface {
	List(context.Context, string) ([]applicationsecret.Metadata, error)
	GetMany(context.Context, string, []string) (map[string][]byte, error)
}

type appExportSecretReaderFactory func(context.Context, resolvedApplication) (appExportSecretReader, error)

func newAppExportSecretReader(ctx context.Context, resolved resolvedApplication) (appExportSecretReader, error) {
	return resolvedApplicationSecretService(ctx, resolved)
}

func executeResolvedApplicationExport(ctx context.Context, resolved resolvedApplication, includeStates []string, outputPath, passwordPath string, out io.Writer, secretReader appExportSecretReaderFactory) error {
	if len(includeStates) == 0 && noInput(ctx) {
		return usageError("export requires at least one --include STATE", "Select a local source such as local.developer-credentials; an export must not be empty.")
	}
	// An explicit selector is already user consent. Authenticate it before any
	// managed-secret availability probe; interactive selection is authorized
	// only after the user chooses the source below.
	if err := authorizeExport(ctx, resolved, includeStates); err != nil {
		return err
	}

	sources, err := discoverApplicationExportSources(ctx, resolved, includeStates, secretReader)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		// No eligible local source exists for this target/environment. Report
		// explicitly rather than failing closed on an empty selection the caller
		// could not have produced.
		return fmt.Errorf("no eligible local export source for %s / %s", resolved.Target.Name, resolved.Manifest.Environment)
	}

	// Fail-closed human-authorization reveal seam: authorize the selected
	// sources BEFORE the availability probe so a managed-secret selection that
	// a caller cannot authorize fails closed with the operator-authentication
	// denial rather than a silent "source unavailable".
	if err := authorizeExport(ctx, resolved, includeStates); err != nil {
		return err
	}

	// Fail closed: an empty selection (no --include, no interactive choice) is a
	// usage error, never a silent fallback.
	if len(includeStates) == 0 {
		if !appExportIsTerminal() {
			return usageError("interactive export requires a terminal when --include STATE is omitted", "For CI/scripts pass at least one --include STATE; never read a password through argv.")
		}
		selected, err := promptGuidedExportSelection(appExportInput, out, sources)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			return ErrExportEmptySelection
		}
		includeStates = selected
		if err := authorizeExport(ctx, resolved, includeStates); err != nil {
			return err
		}
	} else {
		// Every --include STATE must reference a known, available source; an
		// unknown or unavailable class fails closed.
		for _, state := range includeStates {
			match := false
			for _, s := range sources {
				if s.StateClass == state {
					match = true
					if !s.Available {
						return fmt.Errorf("export source %q exists as a selector but has no local material for %s / %s", state, resolved.Target.Name, resolved.Manifest.Environment)
					}
					break
				}
			}
			if !match {
				return usageError("unknown export state "+state, "Select from an available source, e.g. local.developer-credentials. Run 'baha app export --help' for usage.")
			}
		}
	}

	entries, err := collectExportEntries(ctx, resolved, includeStates, secretReader)
	if err != nil {
		return err
	}
	defer zeroExportEntries(entries)
	if len(entries) == 0 {
		return ErrExportEmptySelection
	}

	password, err := readExportPasswordFile(passwordPath)
	if err != nil {
		return err
	}
	defer zeroBytes(password)

	manifest := applicationbackup.ExportManifest{
		ApplicationID: resolved.Manifest.ApplicationID,
		Application:   resolved.Manifest.Name,
		Environment:   resolved.Manifest.Environment,
	}

	// Report what was selected without printing any secret value.
	reportExportSelection(out, includeStates)

	envelope, err := applicationbackup.BuildExport(manifest, entries, password)
	if err != nil {
		return fmt.Errorf("encrypt app export: %w", err)
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("%s-%s-%s.bhexport", resolved.Manifest.Name, resolved.Manifest.Environment, time.Now().UTC().Format("20060102T150405Z"))
	}
	if err := writeAppExport(outputPath, envelope); err != nil {
		return err
	}
	fmt.Fprintf(out, "Exported local credentials for %s / %s / %s to %s (encrypted; needs the export password to open).\n", resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, outputPath)
	return nil
}

// authorizeExport enforces the fail-closed human-authorization reveal seam for
// any selected source that carries a managed secret value. A bare
// runtime/trusted-local identity cannot satisfy it in any environment, so the
// export fails closed; a verified operator principal is mandatory.
func authorizeExport(ctx context.Context, resolved resolvedApplication, includeStates []string) error {
	for _, state := range includeStates {
		switch state {
		case "secrets/openbao":
			if err := authorizeHumanReveal(ctx, resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// discoverApplicationExportSources returns the eligible local export sources for
// the resolved application. Every source is unavailable by default; only a
// source with material actually present on this target/environment reports
// Available=true. Sources are sorted by StateClass for deterministic output.
func discoverApplicationExportSources(ctx context.Context, resolved resolvedApplication, includeStates []string, secretReader appExportSecretReaderFactory) ([]appExportSource, error) {
	m := resolved.Manifest
	target := resolved.Target.Name
	env := m.Environment

	sources := []appExportSource{
		{
			StateClass:     "local.developer-credentials",
			Description:    "local developer credentials for this target/environment",
			NeedsHumanReveal: false,
		},
		{
			StateClass:     "secrets/openbao",
			Description:    "managed application secret values (requires human reveal)",
			NeedsHumanReveal: true,
		},
	}

	// Local developer credentials: only present for development environments.
	_, devErr := devaccess.Load(target, env)
	if devErr == nil {
		sources[0].Available = true
	}

	// Managed secret values: present when the application owns an OpenBao scope
	// with at least one configured secret key.
	probeManagedSecrets := len(includeStates) == 0 || includesManagedSecrets(includeStates)
	if m.Services.Secrets && probeManagedSecrets {
		if present, err := managedSecretScopePresent(ctx, resolved, secretReader); err != nil {
			return nil, err
		} else if present {
			sources[1].Available = true
		}
	}

	sort.SliceStable(sources, func(i, j int) bool { return sources[i].StateClass < sources[j].StateClass })
	return sources, nil
}

// managedSecretScopePresent reports whether the application owns an OpenBao
// scope with at least one configured secret key. It authorizes secret.list and
// reads only metadata through the application's managed runtime. Values are
// fetched only after the caller selects this source and passes human reveal.
func managedSecretScopePresent(ctx context.Context, resolved resolvedApplication, secretReader appExportSecretReaderFactory) (bool, error) {
	metadata, err := listExportManagedSecretMetadata(ctx, resolved, secretReader)
	if err != nil {
		return false, err
	}
	return len(metadata) > 0, nil
}

// collectExportEntries gathers the selected entries. It never returns a raw
// secret value in transport or audit metadata.
func collectExportEntries(ctx context.Context, resolved resolvedApplication, includeStates []string, secretReader appExportSecretReaderFactory) (entries []applicationbackup.SecretEntry, err error) {
	target := resolved.Target.Name
	env := resolved.Manifest.Environment
	defer func() {
		// If a later source fails, clear any credential bytes collected earlier.
		// On success the caller clears them after encryption.
		if err != nil {
			zeroExportEntries(entries)
			entries = nil
		}
	}()

	for _, state := range includeStates {
		switch state {
		case "local.developer-credentials":
			local, err := devaccess.Load(target, env)
			if err != nil {
				return nil, fmt.Errorf("read eligible local credentials for app export: %w", err)
			}
			entries = append(entries,
				applicationbackup.SecretEntry{Name: "developer-access/username", Data: []byte(local.Username)},
				applicationbackup.SecretEntry{Name: "developer-access/password", Data: []byte(local.Password)},
			)
		case "secrets/openbao":
			if err := authorizeHumanReveal(ctx, resolved.Target.Name, env, resolved.Manifest.Name); err != nil {
				return nil, err
			}
			metadata, err := listExportManagedSecretMetadata(ctx, resolved, secretReader)
			if err != nil {
				return nil, err
			}
			keys := make([]string, 0, len(metadata))
			for _, item := range metadata {
				if item.Present {
					keys = append(keys, item.Name)
				}
			}
			if len(keys) == 0 {
				return nil, errors.New("managed application secret source is no longer available for export")
			}
			reader, err := secretReader(ctx, resolved)
			if err != nil {
				return nil, errors.New("unable to read managed application secrets for export")
			}
			values, err := reader.GetMany(ctx, resolved.Manifest.Name, keys)
			if err != nil {
				zeroExportSecretValues(values)
				return nil, errors.New("unable to read managed application secrets for export")
			}
			defer zeroExportSecretValues(values)
			for _, key := range keys {
				value, ok := values[key]
				if !ok {
					return nil, errors.New("managed application secret source changed during export; retry after checking the selection")
				}
				entries = append(entries, applicationbackup.SecretEntry{Name: "secrets/openbao/" + key, Data: append([]byte(nil), value...)})
			}
		}
	}
	return entries, nil
}

func listExportManagedSecretMetadata(ctx context.Context, resolved resolvedApplication, secretReader appExportSecretReaderFactory) ([]applicationsecret.Metadata, error) {
	if err := authorizeApplicationOperation(ctx, "secret.list", resolved); err != nil {
		return nil, err
	}
	reader, err := secretReader(ctx, resolved)
	if err != nil {
		return nil, errors.New("unable to inspect managed application secret availability for export")
	}
	items, err := reader.List(ctx, resolved.Manifest.Name)
	if err != nil {
		return nil, errors.New("unable to inspect managed application secret availability for export")
	}
	configured := make([]applicationsecret.Metadata, 0, len(items))
	for _, item := range items {
		if item.Present {
			configured = append(configured, item)
		}
	}
	return configured, nil
}

func includesManagedSecrets(states []string) bool {
	for _, state := range states {
		if state == "secrets/openbao" {
			return true
		}
	}
	return false
}

func zeroExportEntries(entries []applicationbackup.SecretEntry) {
	for i := range entries {
		zeroBytes(entries[i].Data)
	}
}

func zeroExportSecretValues(values map[string][]byte) {
	for _, value := range values {
		zeroBytes(value)
	}
}

// reportExportSelection prints a safe, non-secret summary of the selected
// sources. It never prints any secret value, entry Data, or password.
func reportExportSelection(out io.Writer, includeStates []string) {
	sorted := append([]string(nil), includeStates...)
	sort.Strings(sorted)
	fmt.Fprintf(out, "Selected export sources: %s\n", strings.Join(sorted, ", "))
}

func promptGuidedExportSelection(input io.Reader, out io.Writer, sources []appExportSource) ([]string, error) {
	file, ok := input.(*os.File)
	if !ok {
		return nil, errors.New("guided export selection requires terminal input")
	}
	reader := bufio.NewReader(file)
	fmt.Fprintln(out, "Export selection")
	var selected []string
	for _, s := range sources {
		if !s.Available {
			fmt.Fprintf(out, "  %-30s (not available for this target/environment)\n", s.StateClass)
			continue
		}
		prompt := fmt.Sprintf("Include %s? [%s] ", s.StateClass, defaultYesPrompt(s.NeedsHumanReveal))
		fmt.Fprint(out, prompt)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		value := strings.ToLower(strings.TrimSpace(line))
		if value == "" {
			continue
		}
		switch value {
		case "y", "yes":
			selected = append(selected, s.StateClass)
		case "n", "no":
		default:
			return nil, errors.New("confirmation must be yes or no")
		}
	}
	return selected, nil
}

func defaultYesPrompt(needsHumanReveal bool) string {
	if needsHumanReveal {
		return "N"
	}
	return "y"
}

func parseAppExportArgs(args []string) (name, outputPath, passwordPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output":
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError("--output requires a file path", "Run 'baha app export --help' for usage.")
			}
			outputPath = args[i]
		case "--password-file":
			i++
			if i >= len(args) || args[i] == "" {
				return "", "", "", usageError("--password-file requires a file path", "Never pass an export password directly on the command line.")
			}
			passwordPath = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", "", usageError("unknown option "+args[i], "Run 'baha app export --help' for usage.")
			}
			if name != "" {
				return "", "", "", usageError("baha app export accepts at most one NAME", "Inside an application repository omit NAME.")
			}
			name = args[i]
		}
	}
	if passwordPath == "" {
		return "", "", "", usageError("--password-file is required", "Store the export password in an owner-only file; it is never accepted through argv.")
	}
	return name, outputPath, passwordPath, nil
}

func extractExportSelectionArgs(args []string) ([]string, []string, error) {
	filtered := make([]string, 0, len(args))
	var include []string
	for i := 0; i < len(args); i++ {
		if args[i] != "--include" {
			filtered = append(filtered, args[i])
			continue
		}
		i++
		if i >= len(args) || args[i] == "" {
			return nil, nil, usageError("--include requires a state", "Select a local source such as local.developer-credentials.")
		}
		include = append(include, args[i])
	}
	return filtered, include, nil
}

func readExportPasswordFile(path string) ([]byte, error) {
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
	if info.Size() > 64<<10 {
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

func readExportPasswordFromTerminal(out io.Writer) ([]byte, error) {
	file, ok := appExportInput.(*os.File)
	if !ok {
		return nil, errors.New("secure export password entry requires a terminal")
	}
	for {
		first, err := readHiddenTerminalLine(file, out, "Export password (minimum 12 bytes): ")
		if err != nil {
			return nil, err
		}
		if len(first) < 12 {
			zeroBytes(first)
			fmt.Fprintln(out, "[WARN] Password is too short. Use at least 12 bytes (12+ ASCII characters recommended) and try again.")
			continue
		}
		second, err := readHiddenTerminalLine(file, out, "Confirm export password: ")
		if err != nil {
			zeroBytes(first)
			return nil, err
		}
		if !bytes.Equal(first, second) {
			zeroBytes(first)
			zeroBytes(second)
			fmt.Fprintln(out, "[WARN] Password confirmation does not match. Please enter the password again.")
			continue
		}
		zeroBytes(second)
		return first, nil
	}
}

func appExportIsTerminal() bool {
	file, ok := appExportInput.(*os.File)
	return ok && file.Fd() == 0
}

func writeAppExport(path string, envelope []byte) error {
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
