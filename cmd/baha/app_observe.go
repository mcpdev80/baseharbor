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
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appStatusCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "status",
		Summary: "Show application runtime and readiness status",
		Usage:   "baha app status NAME",
		Long:    "Reports materialized runtime state, running services and actual PostgreSQL readiness. A running container is not considered ready unless the authenticated verification query succeeds.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app status requires exactly one NAME", "Example: baha app status demo")
			}
			m, _, err := store.Load(args[0])
			if err != nil {
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
			postgresRunning := containsString(services, "postgres")
			fmt.Fprintf(out, "Application %s (%s)\n", m.Name, m.Environment)
			fmt.Fprintf(out, "Project: %s\n", project)
			if !postgresRunning {
				fmt.Fprintln(out, "[FAIL] postgres          not running")
				return errors.New("application is not ready")
			}
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := application.VerifyPostgresRuntime(checkCtx, compose, m, files); err != nil {
				fmt.Fprintln(out, "[FAIL] postgres          running but readiness query failed")
				return errors.New("application is not ready")
			}
			fmt.Fprintln(out, "[OK] postgres          running and authenticated SELECT 1 succeeded")
			return nil
		},
	}
}

func appDoctorCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "doctor",
		Summary: "Diagnose an application's runtime",
		Usage:   "baha app doctor NAME",
		Long:    "Checks desired state, secure local runtime files, Compose configuration, service state and authenticated PostgreSQL readiness without mutating the application.",
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
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error {
					if !m.Services.Postgres || m.Services.Redis || m.Services.Secrets {
						return application.ErrUnsupportedService
					}
					return nil
				}},
				{Name: "manifest permissions", Run: func(context.Context) error { return ownerOnly(manifestPath) }},
				{Name: "runtime state", Run: func(context.Context) error { return runtimeErr }},
				{Name: "runtime permissions", Run: func(context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					return application.CheckRuntimePermissions(files)
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
				{Name: "postgres running", Run: func(ctx context.Context) error {
					if runtimeErr != nil {
						return runtimeErr
					}
					var err error
					running, err = compose.RunningServicesProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
					if err != nil {
						return err
					}
					if !containsString(running, "postgres") {
						return errors.New("postgres service is not running")
					}
					return nil
				}},
				{Name: "postgres readiness", Run: func(ctx context.Context) error {
					if !containsString(running, "postgres") {
						return errors.New("postgres service is not running")
					}
					return application.VerifyPostgresRuntime(ctx, compose, m, files)
				}},
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
