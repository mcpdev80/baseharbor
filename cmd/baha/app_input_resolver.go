package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationinput"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
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
		filtered, format, err := parseReadOutputArgs(args, "app init")
		if err != nil {
			return err
		}
		args = filtered
		destination := out
		if format == outputJSON {
			out = io.Discard
			ctx = machineNoninteractiveContext(ctx)
		}

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
		// app init is rooted in the current directory. An ancestor application
		// manifest must not silently capture a nested project that the user is
		// explicitly adopting as its own application.
		manifestPath, hasLocalManifest, err := currentRepositoryManifest(cwd)
		if err != nil {
			return err
		}
		if !hasLocalManifest {
			if len(injected) != 0 {
				return usageError("--input is available after an application contract exists", "Create baseharbor.yaml first with guided/quick init or deterministic manifest flags, then inject deployment inputs.")
			}
			if format == outputJSON {
				if agents {
					return usageError("--agents is a host guidance mode", "Use deterministic initialization JSON separately from host guidance generation.")
				}
				return appInitCommand().Run(ctx, append(forwarded, "--json"), destination, errOut)
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
		resolved, err := resolveApplication(ctx, store, nil, "init")
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
		if format == outputJSON {
			return writeJSON(destination, map[string]any{"application": resolved.Manifest.Name, "environment": resolved.Manifest.Environment, "configured": true})
		}

		return nil
	}
	return base
}

func currentRepositoryManifest(cwd string) (string, bool, error) {
	path := filepath.Join(filepath.Clean(cwd), application.RepositoryManifestName)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return path, false, nil
	}
	if err != nil {
		return path, false, err
	}
	if !info.Mode().IsRegular() {
		return path, false, fmt.Errorf("%s is not a regular file", path)
	}
	return path, true, nil
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
	if err := authorizeApplicationOperation(ctx, "app.configure", resolved); err != nil {
		return err
	}
	current, err := loadRepositoryInitStateFromStateRoot(resolved.stateRoot())
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
	development := devaccess.Enabled(resolved.Manifest.Environment)
	if development {
		if strings.TrimSpace(opts.Hostname) != "" {
			return usageError("development hostnames are derived from the target development domain", "Use 'baha dev domain [DOMAIN]' to change the target-wide development domain.")
		}
		if mode := strings.TrimSpace(opts.TLSMode); mode != "" && mode != "local" {
			return usageError("development TLS is managed locally by BaseHarbor", "Remove --tls or use --tls local.")
		}
		if strings.TrimSpace(opts.CertDir) != "" {
			return usageError("development TLS does not use an external certificate directory", "Remove --cert-dir; BaseHarbor manages local development TLS automatically.")
		}
		host, err := devaccess.ApplicationHost(resolved.Target.Name, resolved.Manifest.Name, "api")
		if err != nil {
			return err
		}
		supplied[inputHostname] = host
		supplied[inputTLSMode] = "local"
		supplied[inputCertDir] = ""
	}
	if !interactive && !development {
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
	if resolved.DeploymentRecord == nil {
		pending, err := recordPendingDeployment(ctx, resolved)
		if err != nil {
			return fmt.Errorf("record deployment before saving deployment inputs: %w", err)
		}
		resolved.DeploymentRecord = &pending
	}
	if development {
		return runRepositoryRuntimeInit(ctx, resolved, repositoryInitOptions{Yes: true}, out)
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
	found, err := application.HasRepositoryApplication(cwd)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	resolved, err := resolveApplicationEnvironment(ctx, application.DefaultStore(), nil, "up", opts.Environment)
	if err != nil {
		return err
	}
	repoRoot := resolved.repositoryRoot()
	current, err := loadRepositoryInitStateFromStateRoot(resolved.stateRoot())
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
		return ensureRepositoryWorkloadPortsForUp(ctx, in, out, resolved, repoRoot)
	}
	if opts.Yes || !readerIsTerminal(in) {
		initOpts := repositoryInitOptions{Yes: true}
		if needsTLS && current.Hostname != "" && current.Hostname != "localhost" {
			initOpts.Hostname = current.Hostname
			initOpts.TLSMode = "acme"
		}
		if err := runRepositoryRuntimeInitResolved(ctx, resolved, repoRoot, initOpts, out); err != nil {
			return err
		}
		return ensureRepositoryWorkloadPortsForUp(ctx, in, out, resolved, repoRoot)
	}
	fmt.Fprintln(out, "Application deployment inputs are incomplete; resolving only the missing values...")
	previous := appInitInput
	appInitInput = in
	defer func() { appInitInput = previous }()
	if err := runRepositoryRuntimeInitResolved(ctx, resolved, repoRoot, repositoryInitOptions{}, out); err != nil {
		return err
	}
	return ensureRepositoryWorkloadPortsForUp(ctx, in, out, resolved, repoRoot)
}

func runtimeUpCommandWithInputResolver(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "up")
	if err != nil {
		return err
	}
	args = filtered
	destination := out
	if format == outputJSON {
		out = io.Discard
		ctx = machineNoninteractiveContext(ctx)
	}
	opts, err := parseRuntimeUpOptions(args)
	if err != nil {
		return err
	}
	restoreEnvironment := pushApplicationEnvironmentOverride(opts.Environment)
	defer restoreEnvironment()
	ctx = withMemoryPreflightOverride(ctx, opts.SkipMemoryPreflight)
	ctx = withAssumeYes(ctx, opts.Yes)
	if err := runtimeUpGuided(ctx, runtimeInput, out, opts); err != nil {
		return err
	}
	if opts.ControlPlaneOnly {
		if err := maybeOfferManagedHostTrustWhenReady(ctx, runtimeInput, out, opts); err != nil {
			return err
		}
		if format == outputJSON {
			report, err := inspectControlPlane(ctx)
			if err != nil {
				return err
			}
			return writeJSON(destination, report)
		}
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	found, err := application.HasRepositoryApplication(cwd)
	if err != nil {
		return err
	}
	if !found {
		initialized, err := initializeRepositoryManifestForUp(ctx, runtimeInput, out, errOut, opts)
		if err != nil {
			return err
		}
		if !initialized {
			if err := maybeOfferManagedHostTrustWhenReady(ctx, runtimeInput, out, opts); err != nil {
				return err
			}
			if format == outputJSON {
				report, err := inspectControlPlane(ctx)
				if err != nil {
					return err
				}
				return writeJSON(destination, report)
			}
			return nil
		}
	}
	if err := ensureRepositoryDeploymentInputsForUp(ctx, runtimeInput, out, opts); err != nil {
		return err
	}
	if err := repositoryApplicationUp(ctx, runtimeInput, out, errOut, opts); err != nil {
		return err
	}
	if format == outputJSON {
		status, err := collectApplicationStatusResult(ctx, application.DefaultStore(), nil)
		if err != nil {
			return err
		}
		return writeJSON(destination, status)
	}
	return nil
}
