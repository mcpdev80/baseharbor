package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appUpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "up",
		Summary: "Start an existing application runtime and verify readiness",
		Usage:   "baha app up NAME",
		Long:    "Starts a previously materialized BaseHarbor application runtime using its existing runtime definition, credentials and persistent data. It refuses to recreate a missing persistent PostgreSQL volume and reports success only after an authenticated PostgreSQL verification query succeeds.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app up requires exactly one NAME", "Example: baha app up demo")
			}
			m, manifestPath, err := store.Load(args[0])
			if err != nil {
				return err
			}
			files, err := application.ExistingRuntimeFiles(store, m)
			if err != nil {
				return err
			}

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var compose bhruntime.Compose
			var before []bhruntime.ProjectResource
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error {
					if !m.Services.Postgres || m.Services.Redis || m.Services.Secrets {
						return application.ErrUnsupportedService
					}
					return nil
				}},
				{Name: "manifest permissions", Run: func(context.Context) error { return ownerOnly(manifestPath) }},
				{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(files) }},
				{Name: "managed runtime definition", Run: func(context.Context) error { return application.CheckManagedRuntimeDefinition(files) }},
				{Name: "container runtime + compose", Run: func(ctx context.Context) error {
					var err error
					compose, err = bhruntime.DetectCompose(ctx)
					return err
				}},
				{Name: "compose configuration", Run: func(ctx context.Context) error {
					return compose.ConfigProject(ctx, application.RuntimeProjectName(m), files.Compose, files.Env)
				}},
				{Name: "runtime ownership", Run: func(ctx context.Context) error {
					var err error
					before, err = application.InspectOwnedRuntimeResources(ctx, compose, m)
					return err
				}},
				{Name: "persistent PostgreSQL volume", Run: func(context.Context) error {
					if !application.ResourceExists(before, "volume") {
						return errors.New("managed PostgreSQL volume is missing; refusing to recreate persistent state during app up")
					}
					return nil
				}},
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			if !ok {
				return errors.New("application up preflight failed")
			}

			project := application.RuntimeProjectName(m)
			if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
				return err
			}

			verifyCtx, verifyCancel := context.WithTimeout(ctx, 60*time.Second)
			defer verifyCancel()
			var verifyErr error
			for verifyCtx.Err() == nil {
				verifyErr = application.VerifyPostgresRuntime(verifyCtx, compose, m, files)
				if verifyErr == nil {
					fmt.Fprintln(out, "[OK] postgres          authenticated SELECT 1 succeeded")
					fmt.Fprintf(out, "Application %s is running and ready.\n", m.Name)
					return nil
				}
				select {
				case <-verifyCtx.Done():
				case <-time.After(time.Second):
				}
			}
			return fmt.Errorf("application verification failed: %w", verifyErr)
		},
	}
}
