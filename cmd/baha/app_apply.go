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
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func appApplyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Summary: "Converge and verify an application's backend runtime",
		Usage:   "baha app apply NAME",
		Long:    "Runs plan, preflight, apply and verification for the requested application. PostgreSQL and Valkey are supported, along with managed OpenBao secret scopes. Secret scope convergence requires an initialized, unsealed BaseHarbor OpenBao trust plane.",
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
			var platformFiles bhruntime.Files
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
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
			}
			results, ok := preflight.Run(checkCtx, checks)
			preflight.Format(out, results)
			if !ok {
				return errors.New("application preflight failed")
			}

			files, err := application.EnsureRuntime(store, m)
			if err != nil {
				return err
			}
			project := application.RuntimeProjectName(m)
			if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
				return err
			}
			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				credentialsPath := openbao.ApplicationCredentialsPath(files.Dir)
				if err := openbao.EnsureApplicationScope(ctx, compose, platformFiles, identity, credentialsPath); err != nil {
					return fmt.Errorf("converge OpenBao application secret scope: %w", err)
				}
			}
			if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
				return err
			}

			verifyCtx, verifyCancel := context.WithTimeout(ctx, 60*time.Second)
			defer verifyCancel()
			var verifyErr error
			for verifyCtx.Err() == nil {
				verifyErr = verifyDesiredRuntimeServices(verifyCtx, compose, m, files)
				if verifyErr == nil && m.Services.Secrets {
					identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
					verifyErr = openbao.CheckApplicationScope(verifyCtx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
				}
				if verifyErr == nil {
					printRuntimeReady(out, m)
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

func verifyDesiredRuntimeServices(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if m.Services.Postgres {
		if err := application.VerifyPostgresRuntime(ctx, compose, m, files); err != nil {
			return err
		}
	}
	if m.Services.Redis {
		if err := application.VerifyValkeyRuntime(ctx, compose, m, files); err != nil {
			return err
		}
	}
	return nil
}

func printRuntimeReady(out io.Writer, m application.Manifest) {
	if m.Services.Postgres {
		fmt.Fprintln(out, "[OK] postgres          authenticated SELECT 1 succeeded")
	}
	if m.Services.Redis {
		fmt.Fprintln(out, "[OK] valkey            authenticated PING returned PONG")
	}
	if m.Services.Secrets {
		fmt.Fprintln(out, "[OK] secrets           isolated OpenBao AppRole and secret scope verified")
	}
}
