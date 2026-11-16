package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
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
				if err := authorizeHumanReveal(ctx, resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.Name); err != nil {
					return err
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
			resolved, declared, err := selectWorkloadComponentsForCommand(ctx, store, options.nameArgs, "logs")
			if err != nil {
				return err
			}
			if format == outputJSON && options.follow {
				return usageError("--follow cannot be combined with JSON output", "Unbounded streams have no JSON contract. Use --tail N for a bounded machine result or drop --json.")
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
			if selected == "" {
				selected, err = chooseWorkloadComponentSource(ctx, os.Stdin, errOut, "logs", workloadComponents, candidates, "")
				if err != nil {
					return err
				}
			} else if len(candidates) > 0 && !containsString(candidates, selected) {
				return usageError(fmt.Sprintf("workload component %q is not selected by the application contract", selected), "Choose one of the declared components: "+componentList(candidates)+"\n  Run 'baha app logs --help' for usage.")
			}
			if managed {
				return reportManagedLogGuidance(format, out, resolved, selected)
			}
			compose, workload, environment, composeFiles, err := resolveWorkloadAccessFromResolution(ctx, resolved)
			if err != nil {
				return err
			}
			cmdArgs := []string{"logs"}
			if options.tail > 0 {
				cmdArgs = append(cmdArgs, "--tail", strconv.Itoa(options.tail))
			}
			if options.follow {
				cmdArgs = append(cmdArgs, "--follow")
			}
			if selected != "" {
				cmdArgs = append(cmdArgs, selected)
			}
			// Compose implementations disagree on whether workload logs are emitted on stdout or stderr.
			// The BaseHarbor CLI contract normalizes both streams to stdout for `baha app logs`.
			return compose.RunProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, os.Stdin, out, out, composeFiles, cmdArgs...)
		},
	}
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
// the streamed source is bounded to the compose workload; when the deployment
// policy collects through the Loki provider, the command says so, names the
// verified surfaces of the status read view, and never silently reroutes the
// policy or streams an unverified provider endpoint. Machine output keeps the
// same contract: --json emits the explicit machine.unimplemented state instead
// of a prose hint.
func reportManagedLogGuidance(format cliOutputFormat, out io.Writer, resolved resolvedApplication, component string) error {
	name := component
	if name == "" {
		name = "COMPONENT"
	}
	if format == outputJSON {
		return writeJSON(out, map[string]any{
			"machine":     "unimplemented",
			"operation":   "app.logs.managed-stream",
			"application": resolved.Manifest.Name,
			"environment": resolved.Manifest.Environment,
			"target":      resolved.Target.Name,
			"component":   name,
			"collection":  "managed-loki",
			"next":        "Use 'baha app status --json' for the verified provider log surfaces, or select deployment policy logs.collect.application=false for the compose workload stream.",
			"guidance":    "Managed Loki-backed streaming is not implemented on this path; nothing was streamed and no policy was weakened.",
		})
	}
	fmt.Fprintf(out, "Logs are collected through the managed Loki provider for application %s (%s).\n", resolved.Manifest.Name, resolved.Manifest.Environment)
	fmt.Fprintf(out, "Machine-safe listing runs over the collected streams: 'baha app logs %s --tail N --json' reads bounded recent workload lines and 'baha app status --json' lists the verified provider log surfaces with their accessibility.\n", name)
	return usageError("managed Loki streaming is not exposed on this path", "Use --tail with --json for a bounded machine result, or 'baha app status' to verify the collected provider surfaces.")
}

func appShellCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "shell",
		Summary: "Open a shell in an application workload service",
		Usage:   "baha app shell [COMPONENT] [-c NAME] [--app NAME] [-e ENV|--environment ENV]",
		Long:    "Opens the default interactive shell in the guided workload component. -c names the component; without it the same guidance as 'baha app logs' applies: a unique declared component is derived, a terminal lists the options, and --no-input or scripts fail closed with the options instead of blocking. The shell's exit status is preserved.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runGuidedWorkloadExec(ctx, store, args, out, errOut, "shell", workloadInvocation{shell: true})
		},
	}
}

func appExecCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "exec",
		Summary: "Execute a command in an application workload service",
		Usage:   "baha app exec [COMPONENT] [COMMAND [ARG...]] [-c NAME] [--cmd CMD] [--app NAME] [-e ENV|--environment ENV]",
		Long:    "Runs COMMAND in the guided workload component; with no COMMAND the default interactive shell opens, same as 'baha app shell'. -c NAME selects the component (owner mapping); a missing COMPONENT follows the same guidance and never blocks scripts. --cmd takes exactly one command string, delivered to the owned runtime as a single safely quoted unit; it cannot be combined with positional command arguments. The command's exit status is preserved: baha exits with the same code the command returned.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return runGuidedWorkloadExec(ctx, store, args, out, errOut, "exec", workloadInvocation{})
		},
	}
}

// workloadInvocation is the parsed execution shape of app exec / app shell.
type workloadInvocation struct {
	shell bool
	cmd   string
	argv  []string
}

// runGuidedWorkloadExec is the shared guided exec path (card 0428-063):
// authorization for the app.exec operation category runs fail-closed BEFORE
// any workload materialization or execution, the component choice comes from
// the same read view status renders, and the executed command's exit status is
// preserved to the caller.
func runGuidedWorkloadExec(ctx context.Context, store application.Store, args []string, out, errOut io.Writer, commandName string, invocation workloadInvocation) error {
	selectorArgs, service, commandArgs, err := parseWorkloadExecArgs(args, commandName)
	if err != nil {
		return err
	}
	invocation.argv = commandArgs
	resolved, declared, err := selectWorkloadComponentsForCommand(ctx, store, selectorArgs, "exec")
	if err != nil {
		return err
	}
	selected, err := chooseWorkloadComponentSource(ctx, os.Stdin, errOut, commandName, nil, declared, service)
	if err != nil {
		return err
	}
	compose, workload, environment, composeFiles, err := resolveWorkloadAccessFromResolution(ctx, resolved)
	if err != nil {
		return err
	}
	if len(declared) > 0 && !containsString(declared, selected) {
		return usageError(fmt.Sprintf("workload component %q is not selected by the application contract", selected), "Choose one of the declared components: "+componentList(declared)+"\n  Run 'baha app "+commandName+" --help' for usage.")
	}
	cmdArgs := []string{"exec"}
	if !invocation.shell {
		// Non-interactive exec must not allocate a TTY; -T keeps the exec
		// contract stable for piped machine output.
		cmdArgs = append(cmdArgs, "-T")
	}
	cmdArgs = append(cmdArgs, selected)
	switch {
	case invocation.shell:
		cmdArgs = append(cmdArgs, "sh")
	case invocation.cmd != "":
		// Safe quoting: exactly one command string, delivered as a single
		// argv element for the owned runtime to execute without any local
		// shell re-parsing (a single argument can never be split again).
		cmdArgs = append(cmdArgs, invocation.cmd)
	default:
		cmdArgs = append(cmdArgs, invocation.argv...)
	}
	runErr := compose.RunProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, os.Stdin, out, errOut, composeFiles, cmdArgs...)
	return preserveWorkloadExitStatus(runErr)
}

// parseWorkloadExecArgs splits app exec / app shell arguments into the shared
// application selector arguments, the optional component, and the command
// payload. --cmd accepts exactly one command string; mixing it with
// positional command arguments fails closed.
func parseWorkloadExecArgs(args []string, commandName string) (selectorArgs []string, service string, command []string, err error) {
	selectorArgs = []string{}
	var positional []string
	componentValue := ""
	cmdValue := ""
	cmdSeen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c" || arg == "--component":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, "", nil, usageError(arg+" requires NAME", "Use -c NAME with one workload component name.")
			}
			i++
			if componentValue != "" && componentValue != args[i] {
				return nil, "", nil, usageError("conflicting component selections", "Specify the component only once.")
			}
			componentValue = args[i]
		case strings.HasPrefix(arg, "--component="):
			value := strings.TrimPrefix(arg, "--component=")
			if value == "" {
				return nil, "", nil, usageError("--component requires NAME", "Use --component=NAME with one workload component name.")
			}
			if componentValue != "" && componentValue != value {
				return nil, "", nil, usageError("conflicting component selections", "Specify the component only once.")
			}
			componentValue = value
		case arg == "--cmd":
			if i+1 >= len(args) {
				return nil, "", nil, usageError("--cmd requires one command string", "Use --cmd \"COMMAND\" with exactly one quoted command.")
			}
			if cmdSeen {
				return nil, "", nil, usageError("--cmd was supplied more than once", "Pass exactly one command string to --cmd.")
			}
			i++
			cmdValue = args[i]
			cmdSeen = true
		case strings.HasPrefix(arg, "--cmd="):
			if cmdSeen {
				return nil, "", nil, usageError("--cmd was supplied more than once", "Pass exactly one command string to --cmd.")
			}
			cmdValue = strings.TrimPrefix(arg, "--cmd=")
			cmdSeen = true
		case arg == "--app" || arg == "-e" || arg == "--environment" || arg == "--uuid":
			selectorArgs = append(selectorArgs, arg)
			if i+1 >= len(args) {
				return nil, "", nil, usageError(arg+" requires a value", "Run 'baha app "+commandName+" --help' for usage.")
			}
			i++
			selectorArgs = append(selectorArgs, args[i])
		case strings.HasPrefix(arg, "--app=") || strings.HasPrefix(arg, "--environment=") || strings.HasPrefix(arg, "--uuid="):
			selectorArgs = append(selectorArgs, arg)
		case strings.HasPrefix(arg, "-"):
			return nil, "", nil, usageError("unknown option "+arg, "Run 'baha app "+commandName+" --help' for usage.")
		default:
			positional = append(positional, arg)
		}
	}
	if cmdSeen && len(positional) > 1 {
		return nil, "", nil, usageError("--cmd cannot be combined with positional command arguments", "Use 'baha app "+commandName+" [COMPONENT] --cmd \"COMMAND\"' or the positional form without --cmd.")
	}
	if commandName == "shell" && len(positional) > 1 {
		return nil, "", nil, usageError("app shell accepts at most one COMPONENT", "Run 'baha app shell [COMPONENT]' without command arguments.")
	}
	switch {
	case cmdSeen:
		if cmdValue == "" {
			return nil, "", nil, usageError("--cmd requires one command string", "Use --cmd \"COMMAND\" with exactly one quoted command.")
		}
		if len(positional) == 1 {
			service = positional[0]
		} else if len(positional) > 1 {
			return nil, "", nil, usageError("--cmd accepts at most one COMPONENT before it", "Use 'baha app "+commandName+" [COMPONENT] --cmd \"COMMAND\"'.")
		}
		command = []string{cmdValue}
	default:
		if len(positional) > 0 {
			service = positional[0]
			command = positional[1:]
		}
	}
	if componentValue != "" {
		if service != "" && service != componentValue {
			return nil, "", nil, usageError("-c conflicts with the positional COMPONENT "+service, "Choose one component selection mechanism.")
		}
		service = componentValue
	}
	return selectorArgs, service, command, nil
}

// preserveWorkloadExitStatus keeps the executed command's exit status: a
// non-zero child exit fails the invocation without losing the code, and the
// command tree reports the failure as already presented so main does not
// append a second generic error block. The presented failure is a runtime
// error, never a usage error, so a rejected child command never maps to the
// usage exit code.
func preserveWorkloadExitStatus(runErr error) error {
	if runErr == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Exited() && status.ExitStatus() != 0 {
			return cli.Presented(fmt.Errorf("workload command exited with status %d", status.ExitStatus()))
		}
	}
	return runErr
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
