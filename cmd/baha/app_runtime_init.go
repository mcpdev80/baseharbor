package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

const repositoryInitEnvName = "init.env"

type repositoryInitOptions struct {
	Hostname string
	TLSMode  string
	CertDir  string
	Yes      bool
}

type repositoryInitState struct {
	Hostname string
	TLSMode  string
	CertDir  string
}

func appInitOrConfigureCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "init",
		Summary: "Create the application contract or initialize deployment settings",
		Usage:   "baha app init [--hostname HOST] [--tls acme|existing|openbao-pki] [--cert-dir DIR] [--yes]",
		Long:    "Without an existing baseharbor.yaml, runs the normal guided application-contract generator. With an existing repository manifest, initializes deployment-specific hostname and TLS settings. Values are stored under .baseharbor and stay outside the repository contract. Existing certificate mode accepts one directory; BaseHarbor resolves the certificate set from that directory during startup.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if _, err := application.FindRepositoryManifest(cwd); err != nil {
				if strings.Contains(err.Error(), application.RepositoryManifestName+" not found") {
					return appGuidedInitCommand().Run(ctx, args, out, errOut)
				}
				return err
			}
			opts, err := parseRepositoryInitOptions(args)
			if err != nil {
				return err
			}
			resolved, err := resolveApplication(store, nil, "init")
			if err != nil {
				return err
			}
			return runRepositoryRuntimeInit(resolved, opts, out)
		},
	}
}

func parseRepositoryInitOptions(args []string) (repositoryInitOptions, error) {
	var opts repositoryInitOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func(option string) (string, error) {
			if i+1 >= len(args) {
				return "", usageError(option+" requires a value", "Run 'baha app init --help' for usage.")
			}
			i++
			return strings.TrimSpace(args[i]), nil
		}
		var err error
		switch {
		case arg == "--yes" || arg == "-y":
			opts.Yes = true
		case arg == "--hostname":
			opts.Hostname, err = next("--hostname")
		case strings.HasPrefix(arg, "--hostname="):
			opts.Hostname = strings.TrimSpace(strings.TrimPrefix(arg, "--hostname="))
		case arg == "--tls":
			opts.TLSMode, err = next("--tls")
		case strings.HasPrefix(arg, "--tls="):
			opts.TLSMode = strings.TrimSpace(strings.TrimPrefix(arg, "--tls="))
		case arg == "--cert-dir":
			opts.CertDir, err = next("--cert-dir")
		case strings.HasPrefix(arg, "--cert-dir="):
			opts.CertDir = strings.TrimSpace(strings.TrimPrefix(arg, "--cert-dir="))
		default:
			return repositoryInitOptions{}, usageError("unknown app init option "+arg, "With an existing baseharbor.yaml use --hostname, --tls, --cert-dir and --yes.")
		}
		if err != nil {
			return repositoryInitOptions{}, err
		}
	}
	if opts.TLSMode != "" && opts.TLSMode != "acme" && opts.TLSMode != "existing" && opts.TLSMode != "openbao-pki" {
		return repositoryInitOptions{}, usageError("unsupported --tls value "+opts.TLSMode, "Use acme, existing or openbao-pki.")
	}
	return opts, nil
}

func runRepositoryRuntimeInit(resolved resolvedApplication, opts repositoryInitOptions, out io.Writer) error {
	repoRoot := filepath.Dir(resolved.ManifestPath)
	current, err := loadRepositoryInitState(repoRoot)
	if err != nil {
		return err
	}
	hostname := strings.TrimSpace(opts.Hostname)
	if hostname == "" {
		hostname = current.Hostname
	}
	tlsMode := strings.TrimSpace(opts.TLSMode)
	if tlsMode == "" {
		tlsMode = current.TLSMode
	}
	certDir := strings.TrimSpace(opts.CertDir)
	if certDir == "" {
		certDir = current.CertDir
	}

	interactive := appInitReaderIsTerminal(appInitInput) && !opts.Yes
	reader := bufio.NewReader(appInitInput)
	if hostname == "" {
		if interactive {
			hostname, err = promptLine(reader, out, "Hostname", "localhost")
			if err != nil {
				return err
			}
		} else {
			hostname = "localhost"
		}
	}
	hostname = strings.TrimSpace(hostname)
	if err := validateRuntimeHostname(hostname); err != nil {
		return err
	}

	needsTLS, err := repositoryWorkloadLooksTLS(repoRoot, resolved.Manifest)
	if err != nil {
		return err
	}
	if tlsMode == "" && needsTLS {
		if interactive {
			tlsMode, err = promptTLSMode(reader, out)
			if err != nil {
				return err
			}
		} else if hostname == "localhost" {
			tlsMode = "openbao-pki"
		} else {
			tlsMode = "acme"
		}
	}
	if tlsMode == "" {
		tlsMode = "acme"
	}

	if tlsMode == "existing" {
		if certDir == "" && interactive {
			certDir, err = promptLine(reader, out, "Certificate directory", "")
			if err != nil {
				return err
			}
		}
		certDir = strings.TrimSpace(certDir)
		if certDir == "" {
			return usageError("existing TLS mode requires a certificate directory", "Re-run with --cert-dir /path/to/certificates or run interactively.")
		}
		absolute, err := filepath.Abs(certDir)
		if err != nil {
			return err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return fmt.Errorf("inspect certificate directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("certificate path %s is not a directory", absolute)
		}
		certDir = absolute
	}

	state := repositoryInitState{Hostname: hostname, TLSMode: tlsMode, CertDir: certDir}
	if err := writeRepositoryInitState(repoRoot, state); err != nil {
		return err
	}
	fmt.Fprintf(out, "Application runtime initialization saved for %s (%s).\n", resolved.Manifest.Name, resolved.Manifest.Environment)
	fmt.Fprintf(out, "Hostname: %s\n", hostname)
	fmt.Fprintf(out, "TLS: %s\n", tlsMode)
	if certDir != "" {
		fmt.Fprintf(out, "Certificate directory: %s\n", certDir)
	}
	fmt.Fprintln(out, "next: run 'baha up'")
	return nil
}

func promptTLSMode(reader *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "TLS:")
	fmt.Fprintln(out, "  1. ACME / automatic public certificate")
	fmt.Fprintln(out, "  2. Existing certificate directory")
	fmt.Fprintln(out, "  3. BaseHarbor PKI (OpenBao-backed)")
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(line) {
	case "", "1", "acme":
		return "acme", nil
	case "2", "existing":
		return "existing", nil
	case "3", "openbao-pki", "pki":
		return "openbao-pki", nil
	default:
		return "", fmt.Errorf("invalid TLS selection %q", line)
	}
}

func validateRuntimeHostname(hostname string) error {
	if hostname == "localhost" {
		return nil
	}
	if len(hostname) == 0 || len(hostname) > 253 || strings.ContainsAny(hostname, "/:@ \\") {
		return fmt.Errorf("invalid hostname %q", hostname)
	}
	for _, label := range strings.Split(hostname, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("invalid hostname %q", hostname)
		}
		for _, r := range label {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
				return fmt.Errorf("invalid hostname %q", hostname)
			}
		}
	}
	return nil
}

func repositoryWorkloadLooksTLS(repoRoot string, m application.Manifest) (bool, error) {
	composePath, found, err := application.ResolveWorkloadCompose(repoRoot, m)
	if err != nil || !found {
		return false, err
	}
	data, err := os.ReadFile(composePath)
	if err != nil {
		return false, err
	}
	text := string(data)
	return strings.Contains(text, "HTTPS_PORT") || strings.Contains(text, ":443") || strings.Contains(text, "443:") || strings.Contains(text, "https://"), nil
}

func repositoryInitEnvPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".baseharbor", repositoryInitEnvName)
}

func loadRepositoryInitState(repoRoot string) (repositoryInitState, error) {
	values, err := readSimpleEnvFile(repositoryInitEnvPath(repoRoot))
	if errors.Is(err, os.ErrNotExist) {
		return repositoryInitState{}, nil
	}
	if err != nil {
		return repositoryInitState{}, err
	}
	return repositoryInitState{
		Hostname: strings.TrimSpace(values["BASEHARBOR_HOSTNAME"]),
		TLSMode:  strings.TrimSpace(values["BASEHARBOR_TLS_MODE"]),
		CertDir:  strings.TrimSpace(values["BASEHARBOR_TLS_CERT_DIR"]),
	}, nil
}

func writeRepositoryInitState(repoRoot string, state repositoryInitState) error {
	path := repositoryInitEnvPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	values := map[string]string{
		"BASEHARBOR_HOSTNAME":     state.Hostname,
		"BASEHARBOR_TLS_MODE":     state.TLSMode,
		"BASEHARBOR_TLS_CERT_DIR": state.CertDir,
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func readSimpleEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values, scanner.Err()
}
