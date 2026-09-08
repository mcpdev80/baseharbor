package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appStatusCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show application runtime and readiness status",
		Usage:   "baha app status NAME",
		Long:    "Reports materialized runtime state, running services and protocol-level readiness. Running containers are not considered ready unless each enabled service passes its authenticated verification; managed secrets also require a working isolated OpenBao AppRole scope and every declared required secret.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app status requires exactly one NAME", "Example: baha app status demo")
			}
			m, _, err := store.Load(args[0])
			if err != nil {
				return err
			}
			if err := application.CheckSupportedRuntimeServices(m); err != nil {
				return err
			}
			files, err := application.ExistingRuntimeFiles(store, m)
			if err != nil {
				return err
			}
			compose, err := bhruntime.DetectCompose(ctx)
			if err != nil {
				return err
			}
			project := application.RuntimeProjectName(m)
			services, err := compose.RunningServicesProject(ctx, project, files.Compose, files.Env)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Application %s (%s)\n", m.Name, m.Environment)
			fmt.Fprintf(out, "Project: %s\n", project)

			ready := true
			if m.Services.Postgres {
				if !containsString(services, "postgres") {
					fmt.Fprintln(out, "[FAIL] postgres          not running")
					ready = false
				} else {
					checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					err := application.VerifyPostgresRuntime(checkCtx, compose, m, files)
					cancel()
					if err != nil {
						fmt.Fprintln(out, "[FAIL] postgres          running but readiness query failed")
						ready = false
					} else {
						fmt.Fprintln(out, "[OK] postgres          running and authenticated SELECT 1 succeeded")
					}
				}
			}
			if m.Services.Redis {
				if !containsString(services, "valkey") {
					fmt.Fprintln(out, "[FAIL] valkey            not running")
					ready = false
				} else {
					checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					err := application.VerifyValkeyRuntime(checkCtx, compose, m, files)
					cancel()
					if err != nil {
						fmt.Fprintln(out, "[FAIL] valkey            running but authenticated PING failed")
						ready = false
					} else {
						fmt.Fprintln(out, "[OK] valkey            running and authenticated PING returned PONG")
					}
				}
			}
			if m.Services.Secrets {
				platformFiles, platformErr := bhruntime.ExistingFiles("")
				if platformErr != nil {
					fmt.Fprintln(out, "[FAIL] secrets           BaseHarbor OpenBao runtime is not materialized")
					ready = false
				} else {
					checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
					err := openbao.InspectApplicationScope(checkCtx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					if err == nil {
						err = checkRequiredApplicationSecrets(checkCtx, compose, platformFiles, m, files)
					}
					cancel()
					if err != nil {
						fmt.Fprintln(out, "[FAIL] secrets           application secret requirements are not satisfied")
						ready = false
					} else {
						fmt.Fprintln(out, "[OK] secrets           isolated OpenBao scope and required secrets verified")
					}
				}
			}
			if !ready {
				return errors.New("application is not ready")
			}
			return nil
		},
	}
}

func appDoctorCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "doctor",
		Summary: "Diagnose an application's runtime",
		Usage:   "baha app doctor NAME",
		Long:    "Checks desired state, secure local runtime files, Compose configuration, service state, authenticated protocol readiness, managed OpenBao secret scope health and required application secrets without mutating the application.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app doctor requires exactly one NAME", "Example: baha app doctor demo")
			}
			m, manifestPath, err := store.Load(args[0])
			if err != nil {
				return err
			}
			files, runtimeErr := application.ExistingRuntimeFiles(store, m)
			var compose bhruntime.Compose
			var running []string
			var platformFiles bhruntime.Files
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error { return ownerOnly(manifestPath) }},
				{Name: "runtime state", Run: func(context.Context) error { return runtimeErr }},
				{Name: "runtime permissions", Run: func(context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return application.CheckRuntimePermissions(files)
				}},
				{Name: "managed runtime definition", Run: func(context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return application.CheckManagedRuntimeDefinition(files, m)
				}},
				{Name: "container runtime + compose", Run: func(ctx context.Context) error {
					var err error
					compose, err = bhruntime.DetectCompose(ctx)
					return err
				}},
				{Name: "compose configuration", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
				}},
				{Name: "running services", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					var err error
					running, err = compose.RunningServicesProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
					return err
				}},
			}
			if m.Services.Postgres {
				checks = append(checks,
					preflight.Check{Name: "postgres running", Run: func(context.Context) error {
						if !containsString(running, "postgres") {
							return errors.New("postgres service is not running")
						}
						return nil
					}},
					preflight.Check{Name: "postgres readiness", Run: func(ctx context.Context) error {
						if !containsString(running, "postgres") {
							return errors.New("postgres service is not running")
						}
						return application.VerifyPostgresRuntime(ctx, compose, m, files)
					}},
				)
			}
			if m.Services.Redis {
				checks = append(checks,
					preflight.Check{Name: "valkey running", Run: func(context.Context) error {
						if !containsString(running, "valkey") {
							return errors.New("valkey service is not running")
						}
						return nil
					}},
					preflight.Check{Name: "valkey readiness", Run: func(ctx context.Context) error {
						if !containsString(running, "valkey") {
							return errors.New("valkey service is not running")
						}
						return application.VerifyValkeyRuntime(ctx, compose, m, files)
					}},
				)
			}
			if m.Services.Secrets {
				checks = append(checks,
					preflight.Check{Name: "OpenBao control-plane runtime", Run: func(context.Context) error {
						var err error
						platformFiles, err = bhruntime.ExistingFiles("")
						return err
					}},
					preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
						if runtimeErr != nil {
							return runtimeErr
						}
						if platformFiles.Compose == "" {
							return errors.New("BaseHarbor OpenBao runtime is not materialized")
						}
						identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
						return openbao.InspectApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
					}},
					preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
						if runtimeErr != nil {
							return runtimeErr
						}
						return checkRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
					}},
				)
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			if !ok {
				return errors.New("application doctor found one or more failures")
			}
			fmt.Fprintln(out, "Application runtime is healthy.")
			return nil
		},
	}
}

func ownerOnly(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is accessible by group or others (%o)", path, info.Mode().Perm())
	}
	return nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}
