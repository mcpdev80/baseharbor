package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/config"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	err := runWithIO(context.Background(), os.Args[1:], os.Stdout, os.Stderr)
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	var usage *cli.UsageError
	if errors.As(err, &usage) && usage.Hint != "" {
		fmt.Fprintln(os.Stderr, "hint:", usage.Hint)
	}
	os.Exit(cli.ExitCode(err))
}

func run(args []string) error {
	return runWithIO(context.Background(), args, os.Stdout, os.Stderr)
}

func runWithIO(ctx context.Context, args []string, out, errOut io.Writer) error {
	return rootCommand().Execute(ctx, args, out, errOut)
}

func rootCommand() *cli.Command {
	store := application.DefaultStore()
	root := &cli.Command{
		Name:    "baha",
		Summary: "BaseHarbor command-line interface",
		Usage:   "baha <command> [options]",
		Long:    "Manage BaseHarbor and isolated application backend runtimes from one binary. Commands are fail-closed: validation and preflight happen before mutation.",
	}

	root.Children = []*cli.Command{
		{Name: "init", Summary: "Create a minimal BaseHarbor configuration", Usage: "baha init", Run: noArgs("baha init", initConfig)},
		{Name: "up", Summary: "Start the local BaseHarbor control-plane runtime", Usage: "baha up", Run: noArgsCtx("baha up", runtimeUp)},
		{Name: "down", Summary: "Stop the local BaseHarbor control-plane runtime", Usage: "baha down", Run: noArgsCtx("baha down", runtimeDown)},
		{Name: "status", Summary: "Show control-plane container and readiness status", Usage: "baha status", Run: noArgsCtx("baha status", runtimeStatus)},
		{Name: "doctor", Summary: "Verify host and control-plane prerequisites", Usage: "baha doctor", Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha doctor does not accept arguments", "Run 'baha doctor --help' for usage.")
			}
			formatted, ok := health.Format(health.Doctor())
			fmt.Fprint(out, formatted)
			if !ok {
				return errors.New("one or more checks failed")
			}
			return nil
		}},
		appCommand(store),
		{Name: "version", Aliases: []string{"--version", "-v"}, Summary: "Print build version", Usage: "baha version", Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha version does not accept arguments", "Run 'baha version --help' for usage.")
			}
			fmt.Fprintf(out, "baha %s (commit %s, built %s)\n", version, commit, date)
			return nil
		}},
	}
	return root
}

func appCommand(store application.Store) *cli.Command {
	app := &cli.Command{
		Name:    "app",
		Summary: "Manage declarative application backend runtimes",
		Usage:   "baha app <command> [options]",
		Long:    "Applications are independent consumers of BaseHarbor. Their manifests contain desired backend services, never application business logic or plaintext credentials.",
	}
	app.Children = []*cli.Command{
		{
			Name:    "create",
			Summary: "Create an application manifest",
			Usage:   "baha app create NAME [--environment ENV] [--postgres] [--redis] [--secrets]",
			Long: "Creates declarative application state only; it does not start containers. If no service flag is supplied, PostgreSQL is enabled by default.\n\nOptions:\n  --environment ENV  Application environment (default: dev)\n  --postgres         Enable PostgreSQL\n  --redis            Enable Redis/Valkey\n  --secrets          Enable managed application secrets",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				name, environment, postgres, redis, secrets, err := parseCreateArgs(args)
				if err != nil {
					return err
				}
				m := application.New(name, environment, postgres, redis, secrets)
				path, err := store.Create(m)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "created application %s (%s)\n", m.Name, m.Environment)
				fmt.Fprintf(out, "manifest: %s\n", path)
				fmt.Fprintln(out, "next: run 'baha app plan "+m.Name+"' and 'baha app preflight "+m.Name+"'")
				return nil
			},
		},
		{Name: "list", Summary: "List configured applications", Usage: "baha app list", Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha app list does not accept arguments", "Run 'baha app list --help' for usage.")
			}
			items, err := store.List()
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Fprintln(out, "No applications configured.")
				return nil
			}
			fmt.Fprintln(out, "NAME\tENVIRONMENT\tSERVICES")
			for _, item := range items {
				fmt.Fprintf(out, "%s\t%s\t%s\n", item.Name, item.Environment, serviceNames(item))
			}
			return nil
		}},
		{Name: "show", Summary: "Show an application manifest", Usage: "baha app show NAME", Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app show requires exactly one NAME", "Example: baha app show demo")
			}
			m, _, err := store.Load(args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(out, m.YAML())
			return nil
		}},
		{Name: "plan", Summary: "Show desired resources without changing anything", Usage: "baha app plan NAME", Long: "Builds a deterministic desired-state plan. This command is read-only and is the basis for the apply/convergence engine.", Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app plan requires exactly one NAME", "Example: baha app plan demo")
			}
			m, _, err := store.Load(args[0])
			if err != nil {
				return err
			}
			plan, err := application.BuildPlan(m)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Plan for %s (%s)\n", plan.Application, plan.Environment)
			for i, action := range plan.Actions {
				fmt.Fprintf(out, "%d. %s %s - %s\n", i+1, action.Kind, action.Resource, action.Description)
			}
			fmt.Fprintln(out, "No changes were made.")
			return nil
		}},
		{Name: "preflight", Summary: "Validate an application before future mutation", Usage: "baha app preflight NAME", Long: "Checks manifest integrity, secure local state and the container runtime. It never mutates application resources.", Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app preflight requires exactly one NAME", "Example: baha app preflight demo")
			}
			m, path, err := store.Load(args[0])
			if err != nil {
				return err
			}
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "application state permissions", Run: func(context.Context) error {
					info, err := os.Stat(path)
					if err != nil {
						return err
					}
					if info.Mode().Perm()&0o077 != 0 {
						return fmt.Errorf("%s is accessible by group or others (%o)", path, info.Mode().Perm())
					}
					return nil
				}},
				{Name: "container runtime + compose", Run: func(ctx context.Context) error { _, err := bhruntime.DetectCompose(ctx); return err }},
				{Name: "desired-state plan", Run: func(context.Context) error { _, err := application.BuildPlan(m); return err }},
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			if !ok {
				return errors.New("application preflight failed")
			}
			fmt.Fprintln(out, "Preflight passed. No changes were made.")
			return nil
		}},
	}
	return app
}

func parseCreateArgs(args []string) (name, environment string, postgres, redis, secrets bool, err error) {
	environment = "dev"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--postgres":
			postgres = true
		case arg == "--redis":
			redis = true
		case arg == "--secrets":
			secrets = true
		case arg == "--environment":
			if i+1 >= len(args) {
				return "", "", false, false, false, usageError("--environment requires a value", "Example: --environment prod")
			}
			i++
			environment = args[i]
		case strings.HasPrefix(arg, "--environment="):
			environment = strings.TrimPrefix(arg, "--environment=")
		case strings.HasPrefix(arg, "-"):
			return "", "", false, false, false, usageError("unknown option "+arg, "Run 'baha app create --help' for available options.")
		default:
			if name != "" {
				return "", "", false, false, false, usageError("baha app create accepts exactly one NAME", "Example: baha app create demo --postgres")
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", false, false, false, usageError("baha app create requires NAME", "Example: baha app create demo --postgres")
	}
	return name, environment, postgres, redis, secrets, nil
}

func serviceNames(m application.Manifest) string {
	var names []string
	if m.Services.Postgres {
		names = append(names, "postgres")
	}
	if m.Services.Redis {
		names = append(names, "redis")
	}
	if m.Services.Secrets {
		names = append(names, "secrets")
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func usageError(message, hint string) error { return &cli.UsageError{Message: message, Hint: hint} }

type noArgsHandler func(io.Writer) error

func noArgs(name string, fn noArgsHandler) cli.RunFunc {
	return func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if len(args) != 0 {
			return usageError(name+" does not accept arguments", "Run '"+name+" --help' for usage.")
		}
		return fn(out)
	}
}

type noArgsCtxHandler func(context.Context, io.Writer) error

func noArgsCtx(name string, fn noArgsCtxHandler) cli.RunFunc {
	return func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if len(args) != 0 {
			return usageError(name+" does not accept arguments", "Run '"+name+" --help' for usage.")
		}
		return fn(ctx, out)
	}
}

func runtimeUp(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.EnsureFiles("")
	if err != nil {
		return err
	}
	if err := compose.Config(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	if err := compose.Up(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime started")
	fmt.Fprintln(out, "next: run 'baha status' and 'baha doctor'")
	return nil
}

func runtimeDown(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	if err := compose.Down(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Fprintln(out, "BaseHarbor control-plane runtime stopped")
	return nil
}

func runtimeStatus(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	status, err := compose.Status(ctx, files.Compose, files.Env)
	if err != nil {
		return err
	}
	fmt.Fprint(out, status)
	checks := health.RuntimeChecks()
	if len(checks) == 0 {
		return nil
	}
	formatted, ok := health.Format(checks)
	fmt.Fprint(out, formatted)
	if !ok {
		return errors.New("runtime is running but not ready")
	}
	return nil
}

func initConfig(out io.Writer) error {
	const path = config.DefaultFile
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cfg := config.Default()
	if err := os.WriteFile(path, []byte(cfg.YAML()), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s\n", path)
	fmt.Fprintln(out, "next: run 'baha doctor'")
	return nil
}
