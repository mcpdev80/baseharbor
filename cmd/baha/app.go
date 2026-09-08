package main

import (
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
		Long:    "Applications are independent consumers of BaseHarbor. Put baseharbor.yaml in the application repository and run app commands without NAME, or pass NAME explicitly for compatibility with stored application state. Manifests contain desired backend services and required secret names, never plaintext credentials.",
	}

	app.Children = []*cli.Command{
		appInitCommand(),
		{
			Name:    "create",
			Summary: "Create an application manifest in BaseHarbor state",
			Usage:   "baha app create NAME [--environment ENV] [--postgres] [--postgres-instance NAME]... [--redis] [--redis-instance NAME]... [--secrets] [--require-secret NAME]...",
			Long:    "Creates legacy/BaseHarbor-managed declarative application state only; it does not start containers. For a repository-owned source-of-truth manifest prefer 'baha app init'. If no service flag is supplied, one default PostgreSQL instance is enabled.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				m, err := manifestFromCreateArgs(args)
				if err != nil {
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
			Summary: "Show the resolved application manifest",
			Usage:   "baha app show [NAME]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				resolved, err := resolveApplication(store, args, "show")
				if err != nil {
					return err
				}
				fmt.Fprint(out, resolved.Manifest.YAML())
				return nil
			},
		},
		{
			Name:    "plan",
			Summary: "Show desired resources without changing anything",
			Usage:   "baha app plan [NAME]",
			Long:    "Builds a deterministic desired-state plan, including required secret readiness gates. Without NAME it resolves the nearest baseharbor.yaml from the current repository.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				resolved, err := resolveApplication(store, args, "plan")
				if err != nil {
					return err
				}
				plan, err := application.BuildPlan(resolved.Manifest)
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
			Usage:   "baha app preflight [NAME]",
			Long:    "Checks manifest integrity, supported desired services, local state, the container runtime, OpenBao application-provisioning prerequisites and required-secret readiness. Without NAME it resolves the nearest repository baseharbor.yaml.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				resolved, err := resolveApplication(store, args, "preflight")
				if err != nil {
					return err
				}
				m := resolved.Manifest
				checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				var compose bhruntime.Compose
				var platformFiles bhruntime.Files
				var requiredStatuses []openbao.RequiredSecretStatus
				requiredKnown := false
				checks := []preflight.Check{
					{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
					{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
					{Name: "manifest permissions", Run: func(context.Context) error {
						return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
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
							files, err := application.ExistingRuntimeFiles(resolved.Store, m)
							if errors.Is(err, application.ErrRuntimeNotApplied) {
								return nil
							}
							if err != nil {
								return err
							}
							requiredStatuses, err = inspectRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
							if err != nil {
								return err
							}
							requiredKnown = true
							return openbao.RequireApplicationSecrets(requiredStatuses)
						}})
					}
				}
				results, ok := preflight.Run(checkCtx, checks)
				preflight.Format(out, results)
				if len(application.RequiredSecretNames(m)) > 0 {
					if requiredKnown {
						printRequiredSecretStatus(out, requiredStatuses)
					} else {
						fmt.Fprintln(out, "REQUIRED SECRET\tPRESENT\tUSABLE")
						for _, name := range application.RequiredSecretNames(m) {
							fmt.Fprintf(out, "%s\tunknown\tunknown\n", name)
						}
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

func appInitCommand() *cli.Command {
	return &cli.Command{
		Name:    "init",
		Summary: "Create a repository-owned baseharbor.yaml",
		Usage:   "baha app init [NAME] [--environment ENV] [--postgres] [--postgres-instance NAME]... [--redis] [--redis-instance NAME]... [--secrets] [--require-secret NAME]...",
		Long:    "Creates baseharbor.yaml in the current directory for committing with the application source. The interactive checkbox-based capability picker will build on this same manifest generator; flags already provide a deterministic non-interactive path for scripts and CI.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			prepared := append([]string(nil), args...)
			if !hasCreateName(prepared) {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				prepared = append([]string{filepath.Base(cwd)}, prepared...)
			}
			m, err := manifestFromCreateArgs(prepared)
			if err != nil {
				return err
			}
			path := application.RepositoryManifestName
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				if errors.Is(err, os.ErrExist) {
					return fmt.Errorf("%s already exists; edit the existing application contract instead", path)
				}
				return err
			}
			if _, err := file.WriteString(m.YAML()); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			absolute, _ := filepath.Abs(path)
			fmt.Fprintf(out, "created repository manifest for %s (%s)\n", m.Name, m.Environment)
			fmt.Fprintf(out, "manifest: %s\n", absolute)
			fmt.Fprintln(out, "next: review baseharbor.yaml, commit it, then run 'baha app apply'")
			return nil
		},
	}
}

func hasCreateName(args []string) bool {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		switch arg {
		case "--environment", "--postgres-instance", "--redis-instance", "--require-secret":
			skipNext = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

func manifestFromCreateArgs(args []string) (application.Manifest, error) {
	name, environment, postgres, redis, secrets, postgresInstances, redisInstances, required, err := parseCreateArgs(args)
	if err != nil {
		return application.Manifest{}, err
	}
	m := application.New(name, environment, postgres || len(postgresInstances) > 0, redis || len(redisInstances) > 0, secrets)
	if len(postgresInstances) > 0 {
		if postgres {
			postgresInstances = append(postgresInstances, "default")
		}
		m = application.WithPostgresInstances(m, postgresInstances...)
	}
	if len(redisInstances) > 0 {
		if redis {
			redisInstances = append(redisInstances, "default")
		}
		m = application.WithRedisInstances(m, redisInstances...)
	}
	m = application.WithRequiredSecrets(m, required...)
	if err := m.Validate(); err != nil {
		return application.Manifest{}, err
	}
	return m, nil
}

func parseCreateArgs(args []string) (name, environment string, postgres, redis, secrets bool, postgresInstances, redisInstances, required []string, err error) {
	environment = "dev"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--postgres":
			postgres = true
		case arg == "--postgres-instance":
			if i+1 >= len(args) {
				return "", "", false, false, false, nil, nil, nil, usageError("--postgres-instance requires a name", "Example: --postgres-instance analytics")
			}
			i++
			postgresInstances = append(postgresInstances, args[i])
		case strings.HasPrefix(arg, "--postgres-instance="):
			postgresInstances = append(postgresInstances, strings.TrimPrefix(arg, "--postgres-instance="))
		case arg == "--redis":
			redis = true
		case arg == "--redis-instance":
			if i+1 >= len(args) {
				return "", "", false, false, false, nil, nil, nil, usageError("--redis-instance requires a name", "Example: --redis-instance sessions")
			}
			i++
			redisInstances = append(redisInstances, args[i])
		case strings.HasPrefix(arg, "--redis-instance="):
			redisInstances = append(redisInstances, strings.TrimPrefix(arg, "--redis-instance="))
		case arg == "--secrets":
			secrets = true
		case arg == "--require-secret":
			if i+1 >= len(args) {
				return "", "", false, false, false, nil, nil, nil, usageError("--require-secret requires a name", "Example: --require-secret OPENAI_API_KEY")
			}
			i++
			required = append(required, args[i])
			secrets = true
		case strings.HasPrefix(arg, "--require-secret="):
			required = append(required, strings.TrimPrefix(arg, "--require-secret="))
			secrets = true
		case arg == "--environment":
			if i+1 >= len(args) {
				return "", "", false, false, false, nil, nil, nil, usageError("--environment requires a value", "Example: --environment prod")
			}
			i++
			environment = args[i]
		case strings.HasPrefix(arg, "--environment="):
			environment = strings.TrimPrefix(arg, "--environment=")
		case strings.HasPrefix(arg, "-"):
			return "", "", false, false, false, nil, nil, nil, usageError("unknown option "+arg, "Run 'baha app create --help' for available options.")
		default:
			if name != "" {
				return "", "", false, false, false, nil, nil, nil, usageError("application manifest generation accepts exactly one NAME", "Example: baha app init demo --postgres")
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", false, false, false, nil, nil, nil, usageError("application name is required", "Pass NAME or run 'baha app init' from a directory whose name is a valid application slug.")
	}
	return name, environment, postgres, redis, secrets, postgresInstances, redisInstances, required, nil
}

func serviceNames(m application.Manifest) string {
	var names []string
	if count := len(application.PostgresInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "postgres")
		} else {
			names = append(names, fmt.Sprintf("postgres(%d)", count))
		}
	}
	if count := len(application.RedisInstanceNames(m)); count > 0 {
		if count == 1 {
			names = append(names, "redis")
		} else {
			names = append(names, fmt.Sprintf("redis(%d)", count))
		}
	}
	if m.Services.Secrets {
		names = append(names, "secrets")
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
