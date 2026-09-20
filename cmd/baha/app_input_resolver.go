package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationinput"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

const (
	inputHostname = "hostname"
	inputTLSMode  = "tls_mode"
	inputCertDir  = "cert_dir"
)

func appInitWithInputResolverCommand(store application.Store) *cli.Command {
	base := appInitOrConfigureCommand(store)
	base.Usage = "baha app init [--agents] [--input NAME=VALUE]... [--hostname HOST] [--tls acme|existing|local] [--cert-dir DIR] [--yes] | baha app init [--agents] [NAME] [manifest options]"
	base.Long += " Deployment inputs are resolved through the reusable input resolver. --input supports automation-safe injection for declared non-secret inputs such as hostname, tls_mode and cert_dir. --agents creates or idempotently updates only the bounded BaseHarbor section in AGENTS.md."
	baseRun := base.Run
	base.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		filtered, agents, err := extractAgentsOption(args)
		if err != nil {
			return err
		}
		forwarded, injected, err := extractDeclaredInputArgs(filtered)
		if err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		manifestPath, err := application.FindRepositoryManifest(cwd)
		if err != nil {
			if len(injected) != 0 {
				return usageError("--input is available after an application contract exists", "Create baseharbor.yaml first with guided/quick init or deterministic manifest flags, then inject deployment inputs.")
			}
			if err := baseRun(ctx, forwarded, out, errOut); err != nil {
				return err
			}
			if agents {
				changed, err := ensureBaseHarborAgentsSection(cwd)
				if err != nil {
					return err
				}
				if changed {
					fmt.Fprintln(out, "AGENTS.md: BaseHarbor guidance added")
				} else {
					fmt.Fprintln(out, "AGENTS.md: BaseHarbor guidance already current")
				}
			}
			return nil
		}

		opts, err := parseRepositoryInitOptions(forwarded)
		if err != nil {
			return err
		}
		if err := applyInjectedRepositoryInputs(&opts, injected); err != nil {
			return err
		}
		resolved, err := resolveApplication(store, nil, "init")
		if err != nil {
			return err
		}
		if err := runRepositoryRuntimeInitResolved(ctx, resolved, filepath.Dir(manifestPath), opts, out); err != nil {
			return err
		}
		if agents {
			changed, err := ensureBaseHarborAgentsSection(filepath.Dir(manifestPath))
			if err != nil {
				return err
			}
			if changed {
				fmt.Fprintln(out, "AGENTS.md: BaseHarbor guidance added")
			} else {
				fmt.Fprintln(out, "AGENTS.md: BaseHarbor guidance already current")
			}
		}
		return nil
	}
	return base
}

func extractDeclaredInputArgs(args []string) ([]string, map[string]string, error) {
	forwarded := make([]string, 0, len(args))
	injected := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var raw string
		switch {
		case arg == "--input":
			if i+1 >= len(args) {
				return nil, nil, usageError("--input requires NAME=VALUE", "Example: baha app init --input hostname=app.example.com")
			}
			i++
			raw = args[i]
		case strings.HasPrefix(arg, "--input="):
			raw = strings.TrimPrefix(arg, "--input=")
		default:
			forwarded = append(forwarded, arg)
			continue
		}
		name, value, ok := strings.Cut(raw, "=")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || name == "" || value == "" {
			return nil, nil, usageError("--input requires non-empty NAME=VALUE", "Example: baha app init --input tls_mode=existing")
		}
		if _, exists := injected[name]; exists {
			return nil, nil, usageError("duplicate --input "+name, "Supply each input at most once.")
		}
		injected[name] = value
	}
	return forwarded, injected, nil
}

func applyInjectedRepositoryInputs(opts *repositoryInitOptions, injected map[string]string) error {
	for name, value := range injected {
		switch name {
		case inputHostname:
			if strings.TrimSpace(opts.Hostname) != "" && opts.Hostname != value {
				return usageError("hostname was supplied by both --hostname and --input", "Choose one injection mechanism for hostname.")
			}
			opts.Hostname = value
		case inputTLSMode:
			if strings.TrimSpace(opts.TLSMode) != "" && opts.TLSMode != value {
				return usageError("TLS mode was supplied by both --tls and --input", "Choose one injection mechanism for tls_mode.")
			}
			opts.TLSMode = value
		case inputCertDir:
			if strings.TrimSpace(opts.CertDir) != "" && opts.CertDir != value {
				return usageError("certificate directory was supplied by both --cert-dir and --input", "Choose one injection mechanism for cert_dir.")
			}
			opts.CertDir = value
		default:
			return usageError("unknown application input "+name, "Declared deployment inputs are hostname, tls_mode and cert_dir.")
		}
	}
	return nil
}

func repositoryDeploymentInputDefinitions(needsTLS bool) []applicationinput.Definition {
	definitions := []applicationinput.Definition{
		{Name: inputHostname, Label: "Public FQDN", Class: applicationinput.ClassExternal, Required: true},
		{Name: inputTLSMode, Label: "TLS mode", Class: applicationinput.ClassDefault, Default: "local", Required: true},
		{Name: inputCertDir, Label: "Certificate directory", Class: applicationinput.ClassExternal, RequiredIf: &applicationinput.Condition{Input: inputTLSMode, Equals: "existing"}},
	}
	if needsTLS {
		definitions[1].Class = applicationinput.ClassExternal
		definitions[1].Default = ""
	}
	return definitions
}

func runRepositoryRuntimeInitResolved(ctx context.Context, resolved resolvedApplication, repoRoot string, opts repositoryInitOptions, out io.Writer) error {
	current, err := loadRepositoryInitState(repoRoot)
	if err != nil {
		return err
	}
	needsTLS, err := repositoryWorkloadLooksTLS(repoRoot, resolved.Manifest)
	if err != nil {
		return err
	}
	interactive := appInitReaderIsTerminal(appInitInput) && !opts.Yes && !noInput(ctx)
	supplied := map[string]string{
		inputHostname: firstNonEmpty(strings.TrimSpace(opts.Hostname), current.Hostname),
		inputTLSMode:  firstNonEmpty(strings.TrimSpace(opts.TLSMode), current.TLSMode),
		inputCertDir:  firstNonEmpty(strings.TrimSpace(opts.CertDir), current.CertDir),
	}
	if !interactive {
		if supplied[inputHostname] == "" {
			supplied[inputHostname] = "localhost"
		}
		if supplied[inputTLSMode] == "" {
			if needsTLS && supplied[inputHostname] != "localhost" {
				supplied[inputTLSMode] = "acme"
			} else {
				supplied[inputTLSMode] = "local"
			}
		}
	}

	definitions := repositoryDeploymentInputDefinitions(needsTLS)
	result, err := applicationinput.Resolve(definitions, supplied, nil)
	if err != nil {
		return err
	}
	if interactive && len(result.Unresolved) != 0 {
		reader := bufio.NewReader(appInitInput)
		for _, input := range result.Unresolved {
			switch input.Name {
			case inputHostname:
				value, err := promptLine(reader, out, "Public FQDN (example: mailflow.example.com)", "localhost")
				if err != nil {
					return err
				}
				supplied[inputHostname] = value
			case inputTLSMode:
				value, err := promptTLSMode(reader, out)
				if err != nil {
					return err
				}
				supplied[inputTLSMode] = value
			}
		}
		result, err = applicationinput.Resolve(definitions, supplied, nil)
		if err != nil {
			return err
		}
		for _, input := range result.Unresolved {
			if input.Name != inputCertDir {
				continue
			}
			value, err := promptDirectoryPath(reader, out, "Certificate directory")
			if err != nil {
				return err
			}
			supplied[inputCertDir] = value
		}
		result, err = applicationinput.Resolve(definitions, supplied, nil)
		if err != nil {
			return err
		}
	}
	if len(result.Unresolved) != 0 {
		names := make([]string, 0, len(result.Unresolved))
		for _, input := range result.Unresolved {
			names = append(names, input.Name)
		}
		return usageError("required application deployment inputs are unresolved: "+strings.Join(names, ", "), "Provide them with app init flags or --input NAME=VALUE in non-interactive automation.")
	}
	values := applicationinput.PersistableValues(result)
	resolvedOpts := repositoryInitOptions{
		Hostname: values[inputHostname],
		TLSMode:  values[inputTLSMode],
		CertDir:  values[inputCertDir],
		Yes:      true,
	}
	if err := validateRuntimeHostname(resolvedOpts.Hostname); err != nil {
		return err
	}
	if resolvedOpts.TLSMode != "acme" && resolvedOpts.TLSMode != "existing" && resolvedOpts.TLSMode != "local" {
		return usageError("unsupported TLS input value "+resolvedOpts.TLSMode, "Use acme, existing or local.")
	}
	return runRepositoryRuntimeInit(ctx, resolved, resolvedOpts, out)
}

func ensureRepositoryDeploymentInputsForUp(ctx context.Context, in io.Reader, out io.Writer, opts runtimeUpOptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	path, err := application.FindRepositoryManifest(cwd)
	if err != nil {
		return nil
	}
	resolved, err := resolveApplication(application.DefaultStore(), nil, "up")
	if err != nil {
		return err
	}
	repoRoot := filepath.Dir(path)
	current, err := loadRepositoryInitState(repoRoot)
	if err != nil {
		return err
	}
	needsTLS, err := repositoryWorkloadLooksTLS(repoRoot, resolved.Manifest)
	if err != nil {
		return err
	}
	result, err := applicationinput.Resolve(repositoryDeploymentInputDefinitions(needsTLS), map[string]string{
		inputHostname: current.Hostname,
		inputTLSMode:  current.TLSMode,
		inputCertDir:  current.CertDir,
	}, nil)
	if err != nil {
		return err
	}
	if len(result.Unresolved) == 0 {
		return nil
	}
	if opts.Yes || !readerIsTerminal(in) {
		initOpts := repositoryInitOptions{Yes: true}
		if needsTLS && current.Hostname != "" && current.Hostname != "localhost" {
			initOpts.Hostname = current.Hostname
			initOpts.TLSMode = "acme"
		}
		return runRepositoryRuntimeInitResolved(ctx, resolved, repoRoot, initOpts, out)
	}
	fmt.Fprintln(out, "Application deployment inputs are incomplete; resolving only the missing values...")
	previous := appInitInput
	appInitInput = in
	defer func() { appInitInput = previous }()
	return runRepositoryRuntimeInitResolved(ctx, resolved, repoRoot, repositoryInitOptions{}, out)
}

func runtimeUpCommandWithInputResolver(ctx context.Context, args []string, out, errOut io.Writer) error {
	opts, err := parseRuntimeUpOptions(args)
	if err != nil {
		return err
	}
	restoreEnvironment := pushApplicationEnvironmentOverride(opts.Environment)
	defer restoreEnvironment()
	if err := runtimeUpGuided(ctx, runtimeInput, out, opts); err != nil {
		return err
	}
	if opts.ControlPlaneOnly {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := application.FindRepositoryManifest(cwd); err != nil {
		if !errors.Is(err, application.ErrRepositoryManifestNotFound) {
			return err
		}
		initialized, err := initializeRepositoryManifestForUp(ctx, runtimeInput, out, errOut, opts)
		if err != nil {
			return err
		}
		if !initialized {
			return nil
		}
	}
	if err := ensureRepositoryDeploymentInputsForUp(ctx, runtimeInput, out, opts); err != nil {
		return err
	}
	return repositoryApplicationUp(ctx, runtimeInput, out, errOut, opts)
}
