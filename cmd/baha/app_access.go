package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appPSQLCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "sql",
		Summary: "Open the managed SQL console for the current application",
		Usage:   "baha app sql [INSTANCE] [--app NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			appName, instance, err := parseAccessTarget(args, "sql")
			if err != nil {
				return err
			}
			resolved, binding, err := resolveAccessBinding(ctx, store, appName, "postgres", instance)
			if err != nil {
				return err
			}
			path, err := exec.LookPath("psql")
			if err != nil {
				if err := requireManagedClientConsent(ctx, os.Stdin, errOut, resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, "postgres", binding.Instance, "managed-runtime"); err != nil {
					return err
				}
				return runManagedBackendConsole(ctx, resolved, binding, "postgres", out, errOut)
			}
			if err := requireManagedClientConsent(ctx, os.Stdin, errOut, resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, "postgres", binding.Instance, "host-psql"); err != nil {
				return err
			}
			user := binding.Username
			if user == "" {
				user = "baseharbor"
			}
			cmd := exec.CommandContext(ctx, path, "-h", binding.Host, "-p", binding.Port, "-U", user, "-d", binding.Database)
			env := replaceProcessEnv("PGPASSWORD", binding.Password)
			env = replaceEnvIn(env, "PGSSLMODE", "verify-ca")
			env = replaceEnvIn(env, "PGSSLROOTCERT", binding.CertificatesPath)
			cmd.Env = env
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, errOut
			fmt.Fprintf(errOut, "Connecting to %s PostgreSQL instance %s...\n", resolved.Manifest.Name, binding.Instance)
			return cmd.Run()
		},
	}
}

func appRedisCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "cache",
		Summary: "Open the managed cache console for the current application",
		Usage:   "baha app cache [INSTANCE] [--app NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			appName, instance, err := parseAccessTarget(args, "cache")
			if err != nil {
				return err
			}
			resolved, binding, err := resolveAccessBinding(ctx, store, appName, "valkey", instance)
			if err != nil {
				return err
			}
			path, err := exec.LookPath("valkey-cli")
			if err != nil {
				path, err = exec.LookPath("redis-cli")
			}
			if err != nil {
				if err := requireManagedClientConsent(ctx, os.Stdin, errOut, resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, "valkey", binding.Instance, "managed-runtime"); err != nil {
					return err
				}
				return runManagedBackendConsole(ctx, resolved, binding, "valkey", out, errOut)
			}
			if err := requireManagedClientConsent(ctx, os.Stdin, errOut, resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, "valkey", binding.Instance, "host-client"); err != nil {
				return err
			}
			cmd := exec.CommandContext(ctx, path, "--tls", "--cacert", binding.CertificatesPath, "-h", binding.Host, "-p", binding.Port)
			if binding.Username != "" {
				cmd.Args = append(cmd.Args, "--user", binding.Username)
			}
			cmd.Env = replaceProcessEnv("REDISCLI_AUTH", binding.Password)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, errOut
			fmt.Fprintf(errOut, "Connecting to %s Valkey instance %s...\n", resolved.Manifest.Name, binding.Instance)
			return cmd.Run()
		},
	}
}

func appCredsCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "creds",
		Summary: "Show connection metadata without revealing credentials by default",
		Usage:   "baha app creds postgres|valkey [INSTANCE] [--app NAME] [--reveal]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app creds")
			if err != nil {
				return err
			}
			args = filtered
			kind, appName, instance, reveal, err := parseCredsArgs(args)
			if err != nil {
				return err
			}
			resolved, binding, err := resolveAccessBinding(ctx, store, appName, kind, instance)
			if err != nil {
				return err
			}
			if format == outputJSON {
				if reveal {
					return usageError("JSON connection metadata never reveals credentials", "Load protected credential material with an operator-owned client instead.")
				}
				return writeJSON(out, publicApplicationConnection(kind, binding))
			}
			fmt.Fprintf(out, "RESOURCE=%s\nINSTANCE=%s\nHOST=%s\nPORT=%s\n", kind, binding.Instance, binding.Host, binding.Port)
			if binding.Database != "" {
				fmt.Fprintf(out, "DATABASE=%s\n", binding.Database)
			}
			if binding.Username != "" {
				fmt.Fprintf(out, "USERNAME=%s\n", binding.Username)
			}
			if reveal {
				// Revealing the password/URI is a human-only operation: require
				// an authenticated operator on the separate secret.reveal seam.
				if err := authorizeHumanReveal(ctx, resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.ApplicationID); err != nil {
					return err
				}
				// Record the audit event before releasing credential values.
				if err := recordConnectionRevealAudit(ctx, resolved, kind, binding.Instance); err != nil {
					return fmt.Errorf("record connection reveal audit: %w", err)
				}
				fmt.Fprintf(out, "PASSWORD=%s\nURI=%s\n", binding.Password, binding.URI)
			} else {
				fmt.Fprintln(out, "PASSWORD=<masked>")
				fmt.Fprintln(out, "URI=<masked>")
			}
			return nil
		},
	}
}

func appLogsCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "logs",
		Summary: "Show logs for the current application workload",
		Usage:   "baha app logs [COMPONENT] [--follow] [--tail N] [-c NAME]... [--app NAME] [-e ENV|--environment ENV] [--json]",
		Long:    "Reads application logs through the same read-view logic 'baha app status' renders: the deployment log policy decides the source and the application contract decides the streamable components. A missing COMPONENT is guided - a unique declared component is derived, ambiguity lists the options in a terminal, and --no-input or scripts never block and fail closed with the options. --tail N bounds the output to the N most recent lines and finishes. -c filters the stream to the named components. JSON output never follows: unbounded streaming has no JSON contract; bounded --tail reads do. Managed (Loki) collection is reported explicitly, never silently rerouted.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app logs")
			if err != nil {
				return err
			}
			options, err := parseLogsOptions(filtered)
			if err != nil {
				return err
			}
			// Usage validation fails closed before any operator-auth gate: a
			// --follow/--json conflict or an invalid component selection is a
			// caller error, never a runtime authorization failure. Defer the
			// operator-auth boundary past this validation; re-enforce it below.
			if format == outputJSON && options.follow {
				return usageError("--follow cannot be combined with JSON output", "Unbounded streams have no JSON contract. Use --tail N for a bounded machine result or drop --json.")
			}
			deferredAuthCtx := operatorauth.WithSkipResolveOperatorBoundary(ctx)
			resolved, declared, err := resolveWorkloadComponentsForCommand(deferredAuthCtx, store, options.nameArgs, "logs")
			if err != nil {
				return err
			}
			workloadComponents, managed, err := readViewLogsSelection(resolved)
			if err != nil {
				return err
			}
			candidates := workloadComponents
			if len(candidates) == 0 {
				candidates = declared
			}
			selected := options.service
			selectedComponents := append([]string(nil), options.components...)
			if len(selectedComponents) == 0 && selected == "" {
				selected, err = chooseWorkloadComponentSource(ctx, os.Stdin, errOut, "logs", workloadComponents, candidates, "")
				if err != nil {
					return err
				}
			}
			if len(selectedComponents) == 0 && selected != "" {
				selectedComponents = []string{selected}
			}
			if err := validateWorkloadLogComponents(selectedComponents, candidates); err != nil {
				return err
			}
			// The operator-auth boundary was deferred past usage validation;
			// re-enforce it explicitly before anything runs.
			ctx = operatorauth.ClearResolveOperatorBoundary(ctx)
			if err := authorizeApplicationOperation(ctx, "app.logs", resolved); err != nil {
				return err
			}
			if managed {
				return reportManagedLogGuidance(format, out, resolved, selectedComponents)
			}
			compose, workload, environment, composeFiles, err := resolveWorkloadAccessFromResolution(ctx, resolved)
			if err != nil {
				return err
			}
			cmdArgs := workloadLogsCommandArgs(options, selectedComponents)
			// Compose implementations disagree on whether workload logs are emitted on stdout or stderr.
			// The BaseHarbor CLI contract normalizes both streams to stdout for `baha app logs`.
			return compose.RunProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, os.Stdin, out, out, composeFiles, cmdArgs...)
		},
	}
}

func validateWorkloadLogComponents(selected, candidates []string) error {
	if len(candidates) == 0 {
		if len(selected) > 0 {
			return usageError("application contract does not declare workload components", "Run 'baha app status' to inspect the application workload before requesting component logs.")
		}
		return nil
	}
	for _, component := range selected {
		if !containsString(candidates, component) {
			return usageError(fmt.Sprintf("workload component %q is not selected by the application contract", component), "Choose one of the declared components: "+componentList(candidates)+"\n  Run 'baha app logs --help' for usage.")
		}
	}
	return nil
}

func workloadLogsCommandArgs(options logsOptions, selected []string) []string {
	args := []string{"logs"}
	if options.tail > 0 {
		args = append(args, "--tail", strconv.Itoa(options.tail))
	}
	if options.follow {
		args = append(args, "--follow")
	}
	return append(args, selected...)
}

// logsOptions is the parsed option set of `baha app logs`.
type logsOptions struct {
	nameArgs   []string
	service    string
	components []string
	follow     bool
	tail       int
}

func parseLogsOptions(args []string) (logsOptions, error) {
	options := logsOptions{nameArgs: []string{}}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--follow" || arg == "-f":
			options.follow = true
		case arg == "-c" || arg == "--component":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return logsOptions{}, usageError(arg+" requires NAME", "Use -c NAME; repeat for more than one component or pass a comma-separated list.")
			}
			i++
			if err := appendComponentSelection(&options.components, args[i]); err != nil {
				return logsOptions{}, err
			}
		case strings.HasPrefix(arg, "--component="):
			if err := appendComponentSelection(&options.components, strings.TrimPrefix(arg, "--component=")); err != nil {
				return logsOptions{}, err
			}
		case arg == "--tail":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return logsOptions{}, usageError("--tail requires N", "Use --tail N with a positive line count, for example --tail 100.")
			}
			i++
			value, parseErr := strconv.Atoi(strings.TrimSpace(args[i]))
			if parseErr != nil || value < 1 {
				return logsOptions{}, usageError("invalid --tail value "+args[i], "Use a positive line count, for example --tail 100.")
			}
			options.tail = value
		case strings.HasPrefix(arg, "--tail="):
			value, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--tail=")))
			if parseErr != nil || value < 1 {
				return logsOptions{}, usageError("invalid --tail value "+arg, "Use a positive line count, for example --tail=100.")
			}
			options.tail = value
		case arg == "--app" || arg == "-e" || arg == "--environment" || arg == "--uuid":
			options.nameArgs = append(options.nameArgs, arg)
			if i+1 >= len(args) {
				return logsOptions{}, usageError(arg+" requires a value", "Run 'baha app logs --help' for usage.")
			}
			i++
			options.nameArgs = append(options.nameArgs, args[i])
		case strings.HasPrefix(arg, "--app=") || strings.HasPrefix(arg, "--environment=") || strings.HasPrefix(arg, "--uuid="):
			options.nameArgs = append(options.nameArgs, arg)
		case strings.HasPrefix(arg, "-"):
			return logsOptions{}, usageError("unknown option "+arg, "Run 'baha app logs --help' for usage.")
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 {
		return logsOptions{}, usageError("too many arguments", "Specify at most one workload COMPONENT.")
	}
	if len(positional) == 1 {
		options.service = positional[0]
	}
	if options.service != "" && len(options.components) > 0 && !containsString(options.components, options.service) {
		return logsOptions{}, usageError("-c filters conflict with the named COMPONENT "+options.service, "Remove the -c filters or name the same component.")
	}
	return options, nil
}

func appendComponentSelection(components *[]string, value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return usageError("empty component name", "Use -c NAME with a workload component name.")
		}
		duplicate := false
		for _, existing := range *components {
			duplicate = duplicate || existing == part
		}
		if !duplicate {
			*components = append(*components, part)
		}
	}
	return nil
}

// reportManagedLogGuidance keeps the managed (Loki) collection path explicit:
// this command does not read or stream logs from the provider. Both output
// modes point to the verified status read view without changing deployment
// policy or silently falling back to another source.
func reportManagedLogGuidance(format cliOutputFormat, out io.Writer, resolved resolvedApplication, components []string) error {
	if format == outputJSON {
		result := map[string]any{
			"machine":     "unimplemented",
			"operation":   "app.logs.managed-stream",
			"application": resolved.Manifest.Name,
			"environment": resolved.Manifest.Environment,
			"target":      resolved.Target.Name,
			"components":  components,
			"collection":  "managed-loki",
			"next":        "Use 'baha app status --json' to inspect verified provider log surfaces and their accessibility.",
			"guidance":    "Managed Loki-backed streaming is not implemented on this path; nothing was streamed and no policy was weakened.",
		}
		if len(components) == 1 {
			result["component"] = components[0]
		} else if len(components) == 0 {
			result["component"] = "COMPONENT"
		}
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "Logs are collected through the managed Loki provider for application %s (%s).\n", resolved.Manifest.Name, resolved.Manifest.Environment)
	fmt.Fprintln(out, "Managed Loki streaming is not available on this path; no logs were read. Use 'baha app status --json' to inspect verified provider log surfaces and their accessibility.")
	return usageError("managed Loki streaming is not exposed on this path", "Use 'baha app status --json' to inspect the verified provider log surfaces.")
}

// recordConnectionRevealAudit appends a per-reveal audit event for the human
// `baha app creds --reveal` path. It records who revealed what, on which
// target/app/environment/capability, without writing the secret into the event.
func recordConnectionRevealAudit(ctx context.Context, resolved resolvedApplication, kind, instance string) error {
	event := evidence.NewAuditEvent(
		ctx,
		resolved.Target.Name,
		resolved.Manifest.Name,
		resolved.Manifest.Environment,
		"connection-reveal",
		"allow",
	)
	event.Capability = "app.connection"
	event.Resource = kind + "/" + instance
	event.ApplicationID = resolved.Manifest.ApplicationID
	event.DeploymentID = resolved.DeploymentIdentity.DeploymentID
	if decision, ok := operatorauth.AuthorizationDecisionFromContext(ctx); ok && decision.Allowed {
		event.Actor.Identity = decision.Actor.Subject
		event.Actor.Issuer = decision.Actor.Issuer
		event.Actor.Subject = decision.Actor.Subject
		event.Actor.Assurance = decision.Actor.Assurance
		event.Actor.Methods = append([]string(nil), decision.Actor.Methods...)
		event.AuthorizationResult = "allow"
	} else if principal, ok := operatorauth.PrincipalFromContext(ctx); ok {
		event.Actor.Identity = principal.Subject
		event.Actor.Issuer = principal.Issuer
		event.Actor.Subject = principal.Subject
		event.Actor.Assurance = principal.Assurance
		event.Actor.Methods = append([]string(nil), principal.Methods...)
		event.AuthorizationResult = "allow"
	}
	return evidence.Append(resolved.TargetStateRoot, event)
}

func resolveAccessBinding(ctx context.Context, store application.Store, appName, kind, instance string) (resolvedApplication, application.ServiceBinding, error) {
	var appArgs []string
	if appName != "" {
		appArgs = []string{appName}
	}
	resolved, err := resolveApplication(ctx, store, appArgs, kind)
	if err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	if err := authorizeApplicationOperation(ctx, "app.connection", resolved); err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	instances := application.ValkeyInstanceNames(resolved.Manifest)
	if kind == "postgres" {
		instances = application.SQLInstanceNames(resolved.Manifest)
	}
	selected, err := selectAccessInstance(instances, instance, kind)
	if err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	binding, err := application.ResolveServiceBinding(files, kind, selected)
	if err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	if kind == "postgres" {
		switch strings.ToLower(strings.TrimSpace(binding.Username)) {
		case "postgres", "baseharbor_admin", "root":
			return resolvedApplication{}, application.ServiceBinding{}, usageError("privileged database identities cannot be opened by the developer console", "Configure an application-scoped, least-privilege PostgreSQL role before opening baha app sql.")
		}
	}
	return resolved, binding, nil
}

func selectAccessInstance(instances []string, requested, kind string) (string, error) {
	if len(instances) == 0 {
		return "", fmt.Errorf("application does not declare a %s resource", kind)
	}
	if requested != "" {
		if containsString(instances, requested) {
			return requested, nil
		}
		return "", fmt.Errorf("unknown %s instance %q; available: %s", kind, requested, strings.Join(instances, ", "))
	}
	if len(instances) == 1 {
		return instances[0], nil
	}
	sorted := append([]string(nil), instances...)
	sort.Strings(sorted)
	return "", fmt.Errorf("multiple %s instances exist; choose one: %s", kind, strings.Join(sorted, ", "))
}

func resolveWorkloadAccess(ctx context.Context, store application.Store, appName string) (bhruntime.RuntimeProvider, application.WorkloadFiles, map[string]string, []string, error) {
	var appArgs []string
	if appName != "" {
		appArgs = []string{appName}
	}
	resolved, err := resolveApplication(ctx, store, appArgs, "workload access")
	if err != nil {
		return nil, application.WorkloadFiles{}, nil, nil, err
	}
	return resolveWorkloadAccessFromResolution(ctx, resolved)
}

// resolveWorkloadAccessFromResolution materializes the workload access
// surface for an already-resolved application so guided commands authorize
// and resolve once, then reuse the same resolution for the runtime call.
func resolveWorkloadAccessFromResolution(ctx context.Context, resolved resolvedApplication) (bhruntime.RuntimeProvider, application.WorkloadFiles, map[string]string, []string, error) {
	if !resolved.FromRepository {
		return nil, application.WorkloadFiles{}, nil, nil, fmt.Errorf("workload access requires a repository-owned baseharbor.yaml")
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return nil, application.WorkloadFiles{}, nil, nil, err
	}
	compose, err := detectRuntimeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle, bhruntime.CapabilityServiceExec)
	if err != nil {
		return nil, application.WorkloadFiles{}, nil, nil, err
	}
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil {
		return nil, application.WorkloadFiles{}, nil, nil, err
	}
	if !found {
		return nil, application.WorkloadFiles{}, nil, nil, fmt.Errorf("application does not declare a Compose workload")
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return nil, application.WorkloadFiles{}, nil, nil, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return nil, application.WorkloadFiles{}, nil, nil, err
	}
	return compose, workload, environment, composeFiles, nil
}

func parseAccessTarget(args []string, command string) (appName, instance string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--app" {
			if i+1 >= len(args) {
				return "", "", usageError("--app requires a name", "Run 'baha app "+command+" --help' for usage.")
			}
			i++
			appName = args[i]
			continue
		}
		if strings.HasPrefix(arg, "--app=") {
			appName = strings.TrimPrefix(arg, "--app=")
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return "", "", usageError("unknown option "+arg, "Run 'baha app "+command+" --help' for usage.")
		}
		if instance != "" {
			return "", "", usageError("too many arguments", "Specify at most one resource instance.")
		}
		instance = arg
	}
	return appName, instance, nil
}

func parseCredsArgs(args []string) (kind, appName, instance string, reveal bool, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--reveal":
			reveal = true
		case arg == "--app":
			if i+1 >= len(args) {
				return "", "", "", false, usageError("--app requires a name", "Run 'baha app creds --help' for usage.")
			}
			i++
			appName = args[i]
		case strings.HasPrefix(arg, "--app="):
			appName = strings.TrimPrefix(arg, "--app=")
		case strings.HasPrefix(arg, "-"):
			return "", "", "", false, usageError("unknown option "+arg, "Run 'baha app creds --help' for usage.")
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) < 1 || len(positional) > 2 {
		return "", "", "", false, usageError("baha app creds requires postgres|valkey and optional INSTANCE", "Example: baha app creds postgres primary")
	}
	kind = positional[0]
	if kind == "redis" {
		kind = "valkey"
	}
	if kind != "postgres" && kind != "valkey" {
		return "", "", "", false, usageError("unsupported resource "+kind, "Use postgres or valkey.")
	}
	if len(positional) == 2 {
		instance = positional[1]
	}
	return kind, appName, instance, reveal, nil
}

func parseLogsArgs(args []string) (appName, service string, follow bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--follow" || arg == "-f":
			follow = true
		case arg == "--app":
			if i+1 >= len(args) {
				return "", "", false, usageError("--app requires a name", "Run 'baha app logs --help' for usage.")
			}
			i++
			appName = args[i]
		case strings.HasPrefix(arg, "--app="):
			appName = strings.TrimPrefix(arg, "--app=")
		case strings.HasPrefix(arg, "-"):
			return "", "", false, usageError("unknown option "+arg, "Run 'baha app logs --help' for usage.")
		default:
			if service != "" {
				return "", "", false, usageError("too many arguments", "Specify at most one workload service.")
			}
			service = arg
		}
	}
	return appName, service, follow, nil
}

func parseServiceTarget(args []string, command string) (appName, service string, err error) {
	appName, service, err = parseAccessTarget(args, command)
	if err == nil && service == "" {
		err = usageError("SERVICE is required", "Example: baha app "+command+" api")
	}
	return appName, service, err
}

func parseExecArgs(args []string) (appName, service string, command []string, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--app" {
			if i+1 >= len(args) {
				return "", "", nil, usageError("--app requires a name", "Run 'baha app exec --help' for usage.")
			}
			i++
			appName = args[i]
			continue
		}
		if strings.HasPrefix(args[i], "--app=") {
			appName = strings.TrimPrefix(args[i], "--app=")
			continue
		}
		positional = append(positional, args[i])
	}
	if len(positional) < 2 {
		return "", "", nil, usageError("SERVICE and COMMAND are required", "Example: baha app exec api env")
	}
	return appName, positional[0], positional[1:], nil
}

func replaceEnvIn(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return append(out, prefix+value)
}

func replaceProcessEnv(key, value string) []string {
	prefix := key + "="
	env := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, prefix) {
			env = append(env, item)
		}
	}
	return append(env, prefix+value)
}
