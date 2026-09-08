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
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appCommand(store application.Store) *cli.Command {
	app := &cli.Command{
		Name:    "app",
		Summary: "Manage declarative application backend runtimes",
		Usage:   "baha app <command> [options]",
		Long:    "Applications are independent consumers of BaseHarbor. Their manifests contain desired backend services and required secret names, never application business logic or plaintext credentials.",
	}

	app.Children = []*cli.Command{
		{
			Name:    "create",
			Summary: "Create an application manifest",
			Usage:   "baha app create NAME [--environment ENV] [--postgres] [--redis] [--secrets] [--require-secret NAME]...",
			Long:    "Creates declarative application state only; it does not start containers. If no service flag is supplied, PostgreSQL is enabled by default. Required secret declarations automatically enable managed secrets.\n\nOptions:\n  --environment ENV      Application environment (default: dev)\n  --postgres             Enable PostgreSQL\n  --redis                Enable Redis/Valkey\n  --secrets              Enable managed application secrets\n  --require-secret NAME  Declare a required secret; repeat for multiple names",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				name, environment, postgres, redis, secrets, required, err := parseCreateArgs(args)
				if err != nil {
					return err
				}
				m := application.WithRequiredSecrets(application.New(name, environment, postgres, redis, secrets), required...)
				if err := m.Validate(); err != nil {
					return err
				}
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
		{
			Name:    "list",
			Summary: "List configured applications",
			Usage:   "baha app list",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
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
			},
		},
		{
			Name:    "show",
			Summary: "Show an application manifest",
			Usage:   "baha app show NAME",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if len(args) != 1 {
					return usageError("baha app show requires exactly one NAME", "Example: baha app show demo")
				}
				m, _, err := store.Load(args[0])
				if err != nil {
					return err
				}
				fmt.Fprint(out, m.YAML())
				return nil
			},
		},
		{
			Name:    "plan",
			Summary: "Show desired resources without changing anything",
			Usage:   "baha app plan NAME",
			Long:    "Builds a deterministic desired-state plan, including required secret gates. This command is read-only and is the basis for the apply/convergence engine.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
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
			},
		},
		{
			Name:    "preflight",
			Summary: "Validate an application before mutation",
			Usage:   "baha app preflight NAME",
			Long:    "Checks manifest integrity, supported desired services, secure local state, the container runtime, OpenBao provisioning prerequisites and, when an application secret scope already exists, required-secret readiness. It never mutates application resources.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if len(args) != 1 {
					return usageError("baha app preflight requires exactly one NAME", "Example: baha app preflight demo")
				}
				m, path, err := store.Load(args[0])
				if err != nil {
					return err
				}
				checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				var compose bhruntime.Compose
				var platformFiles bhruntime.Files
				var requiredStatuses []openbao.RequiredSecretStatus
				requiredKnown := false
				checks := []preflight.Check{
					{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
					{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
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
					{Name: "container runtime + compose", Run: func(ctx context.Context) error {
						var err error
						compose, err = bhruntime.DetectCompose(ctx)
						return err
					}},
					{Name: "desired-state plan", Run: func(context.Context) error {
						_, err := application.BuildPlan(m)
						return err
					}},
				}
				if m.Services.Secrets {
					identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
					checks = append(checks,
						preflight.Check{Name: "OpenBao control-plane runtime", Run: func(context.Context) error {
							var err error
							platformFiles, err = bhruntime.ExistingFiles("")
							return err
						}},
						preflight.Check{Name: "OpenBao application provisioning", Run: func(ctx context.Context) error {
							if platformFiles.Compose == "" {
								return errors.New("BaseHarbor control-plane runtime is not materialized; run 'baha up' first")
							}
							return openbao.CheckApplicationProvisioning(ctx, compose, platformFiles, identity)
						}},
					)
					if len(application.RequiredSecretNames(m)) > 0 {
						checks = append(checks, preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
							files, err := application.ExistingRuntimeFiles(store, m)
							if errors.Is(err, application.ErrRuntimeNotApplied) {
								return nil
							}
							if err != nil {
								return err
							}
							requiredStatuses, err = inspectRequiredSecrets(ctx, compose, platformFiles, m, files)
							if err != nil {
								return err
							}
							requiredKnown = true
							return requireReadySecrets(requiredStatuses)
						}})
					}
				}
				results, ok := preflight.Run(checkCtx, checks)
				preflight.Format(out, results)
				if len(application.RequiredSecretNames(m)) > 0 {
					fmt.Fprintln(out, "REQUIRED SECRET\tPRESENT\tUSABLE")
					if requiredKnown {
						for _, status := range requiredStatuses {
							fmt.Fprintf(out, "%s\t%s\t%s\n", status.Name, yesNo(status.Present), yesNo(status.Usable))
						}
					} else {
						printRequiredSecretUnknown(out, m)
						fmt.Fprintln(out, "Presence will be checked after the managed secret scope is materialized by 'baha app apply'.")
					}
				}
				if !ok {
					return errors.New("application preflight failed")
				}
				fmt.Fprintln(out, "Preflight passed. No changes were made.")
				return nil
			},
		},
	}
	return app
}

func parseCreateArgs(args []string) (name, environment string, postgres, redis, secrets bool, required []string, err error) {
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
		case arg == "--require-secret":
			if i+1 >= len(args) {
				return "", "", false, false, false, nil, usageError("--require-secret requires a name", "Example: --require-secret OPENAI_API_KEY")
			}
			i++
			required = append(required, args[i])
			secrets = true
		case strings.HasPrefix(arg, "--require-secret="):
			required = append(required, strings.TrimPrefix(arg, "--require-secret="))
			secrets = true
		case arg == "--environment":
			if i+1 >= len(args) {
				return "", "", false, false, false, nil, usageError("--environment requires a value", "Example: --environment prod")
			}
			i++
			environment = args[i]
		case strings.HasPrefix(arg, "--environment="):
			environment = strings.TrimPrefix(arg, "--environment=")
		case strings.HasPrefix(arg, "-"):
			return "", "", false, false, false, nil, usageError("unknown option "+arg, "Run 'baha app create --help' for available options.")
		default:
			if name != "" {
				return "", "", false, false, false, nil, usageError("baha app create accepts exactly one NAME", "Example: baha app create demo --postgres")
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", false, false, false, nil, usageError("baha app create requires NAME", "Example: baha app create demo --postgres")
	}
	return name, environment, postgres, redis, secrets, required, nil
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
