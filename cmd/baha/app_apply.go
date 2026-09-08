package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appApplyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Summary: "Converge and verify an application's backend runtime",
		Usage:   "baha app apply NAME",
		Long:    "Runs plan, preflight, apply and verification for the requested application. The current milestone supports PostgreSQL-only application runtimes and fails closed when an enabled service is not yet converged.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("baha app apply requires exactly one NAME", "Example: baha app apply demo")
			}
			m, manifestPath, err := store.Load(args[0])
			if err != nil {
				return err
			}
			plan, err := application.BuildPlan(m)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Plan for %s (%s): %d actions\n", plan.Application, plan.Environment, len(plan.Actions))

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var compose bhruntime.Compose
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported services", Run: func(context.Context) error {
					if !m.Services.Postgres || m.Services.Redis || m.Services.Secrets {
						return application.ErrUnsupportedService
					}
					return nil
				}},
				{Name: "application state permissions", Run: func(context.Context) error {
					info, err := os.Stat(manifestPath)
					if err != nil {
						return err
					}
					if info.Mode().Perm()&0o077 != 0 {
						return fmt.Errorf("%s is accessible by group or others (%o)", manifestPath, info.Mode().Perm())
					}
					return nil
				}},
				{Name: "container runtime + compose", Run: func(ctx context.Context) error {
					var err error
					compose, err = bhruntime.DetectCompose(ctx)
					return err
				}},
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			if !ok {
				return errors.New("application preflight failed")
			}

			files, err := application.EnsurePostgresRuntime(store, m)
			if err != nil {
				return err
			}
			project := application.RuntimeProjectName(m)
			if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
				return err
			}
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
					fmt.Fprintf(out, "Application %s is ready.\n", m.Name)
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
