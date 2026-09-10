package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appPSQLCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "psql",
		Summary: "Open PostgreSQL for the current application",
		Usage:   "baha app psql [INSTANCE] [--app NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			appName, instance, err := parseAccessTarget(args, "psql")
			if err != nil {
				return err
			}
			resolved, binding, err := resolveAccessBinding(store, appName, "postgres", instance)
			if err != nil {
				return err
			}
			path, err := exec.LookPath("psql")
			if err != nil {
				return fmt.Errorf("psql client not found in PATH")
			}
			user := binding.Username
			if user == "" {
				user = "baseharbor"
			}
			cmd := exec.CommandContext(ctx, path, "-h", binding.Host, "-p", binding.Port, "-U", user, "-d", binding.Database)
			cmd.Env = replaceProcessEnv("PGPASSWORD", binding.Password)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, errOut
			fmt.Fprintf(errOut, "Connecting to %s PostgreSQL instance %s...\n", resolved.Manifest.Name, binding.Instance)
			return cmd.Run()
		},
	}
}

func appRedisCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "redis",
		Aliases: []string{"valkey"},
		Summary: "Open Valkey/Redis for the current application",
		Usage:   "baha app redis [INSTANCE] [--app NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			appName, instance, err := parseAccessTarget(args, "redis")
			if err != nil {
				return err
			}
			resolved, binding, err := resolveAccessBinding(store, appName, "valkey", instance)
			if err != nil {
				return err
			}
			path, err := exec.LookPath("valkey-cli")
			if err != nil {
				path, err = exec.LookPath("redis-cli")
			}
			if err != nil {
				return fmt.Errorf("valkey-cli or redis-cli client not found in PATH")
			}
			cmd := exec.CommandContext(ctx, path, "-h", binding.Host, "-p", binding.Port)
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
			kind, appName, instance, reveal, err := parseCredsArgs(args)
			if err != nil {
				return err
			}
			_, binding, err := resolveAccessBinding(store, appName, kind, instance)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "RESOURCE=%s\nINSTANCE=%s\nHOST=%s\nPORT=%s\n", kind, binding.Instance, binding.Host, binding.Port)
			if binding.Database != "" {
				fmt.Fprintf(out, "DATABASE=%s\n", binding.Database)
			}
			if binding.Username != "" {
				fmt.Fprintf(out, "USERNAME=%s\n", binding.Username)
			}
			if reveal {
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
		Usage:   "baha app logs [SERVICE] [--follow] [--app NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			appName, service, follow, err := parseLogsArgs(args)
			if err != nil {
				return err
			}
			compose, workload, environment, composeFiles, err := resolveWorkloadAccess(ctx, store, appName)
			if err != nil {
				return err
			}
			if service != "" && !containsString(workload.Services, service) {
				return fmt.Errorf("workload service %q is not selected by the application contract", service)
			}
			cmdArgs := []string{"logs"}
			if follow {
				cmdArgs = append(cmdArgs, "--follow")
			}
			if service != "" {
				cmdArgs = append(cmdArgs, service)
			}
			return compose.RunProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, os.Stdin, out, errOut, composeFiles, cmdArgs...)
		},
	}
}

func appShellCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "shell",
		Summary: "Open a shell in an application workload service",
		Usage:   "baha app shell SERVICE [--app NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			appName, service, err := parseServiceTarget(args, "shell")
			if err != nil {
				return err
			}
			compose, workload, environment, composeFiles, err := resolveWorkloadAccess(ctx, store, appName)
			if err != nil {
				return err
			}
			if !containsString(workload.Services, service) {
				return fmt.Errorf("workload service %q is not selected by the application contract", service)
			}
			return compose.RunProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, os.Stdin, out, errOut, composeFiles, "exec", service, "sh")
		},
	}
}

func appExecCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "exec",
		Summary: "Execute a command in an application workload service",
		Usage:   "baha app exec SERVICE COMMAND [ARG...] [--app NAME]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			appName, service, command, err := parseExecArgs(args)
			if err != nil {
				return err
			}
			compose, workload, environment, composeFiles, err := resolveWorkloadAccess(ctx, store, appName)
			if err != nil {
				return err
			}
			if !containsString(workload.Services, service) {
				return fmt.Errorf("workload service %q is not selected by the application contract", service)
			}
			cmdArgs := append([]string{"exec", "-T", service}, command...)
			return compose.RunProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, os.Stdin, out, errOut, composeFiles, cmdArgs...)
		},
	}
}

func resolveAccessBinding(store application.Store, appName, kind, instance string) (resolvedApplication, application.ServiceBinding, error) {
	var appArgs []string
	if appName != "" {
		appArgs = []string{appName}
	}
	resolved, err := resolveApplication(store, appArgs, kind)
	if err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	instances := application.RedisInstanceNames(resolved.Manifest)
	if kind == "postgres" {
		instances = application.PostgresInstanceNames(resolved.Manifest)
	}
	selected, err := selectAccessInstance(instances, instance, kind)
	if err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	files, err := application.ExistingRuntimeFiles(store, resolved.Manifest)
	if err != nil {
		return resolvedApplication{}, application.ServiceBinding{}, err
	}
	binding, err := application.ResolveServiceBinding(files, kind, selected)
	return resolved, binding, err
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

func resolveWorkloadAccess(ctx context.Context, store application.Store, appName string) (bhruntime.Compose, application.WorkloadFiles, map[string]string, []string, error) {
	var appArgs []string
	if appName != "" {
		appArgs = []string{appName}
	}
	resolved, err := resolveApplication(store, appArgs, "workload access")
	if err != nil {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, err
	}
	if !resolved.FromRepository {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, fmt.Errorf("workload access requires a repository-owned baseharbor.yaml")
	}
	files, err := application.ExistingRuntimeFiles(store, resolved.Manifest)
	if err != nil {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, err
	}
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, err
	}
	workload, found, err := materializeRepositoryWorkload(resolved, files)
	if err != nil {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, err
	}
	if !found {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, fmt.Errorf("application does not declare a Compose workload")
	}
	environment, err := repositoryWorkloadEnvironment(ctx, resolved, files)
	if err != nil {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, err
	}
	composeFiles, err := repositoryWorkloadComposeFiles(ctx, compose, resolved, workload, files, environment)
	if err != nil {
		return bhruntime.Compose{}, application.WorkloadFiles{}, nil, nil, err
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
