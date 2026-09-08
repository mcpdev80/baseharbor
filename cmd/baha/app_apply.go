package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

func appApplyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Summary: "Converge and verify an application's backend runtime",
		Usage:   "baha app apply [NAME]",
		Long:    "Runs plan, preflight, apply and verification. Without NAME it resolves the nearest baseharbor.yaml in the current repository, synchronizes a protected internal copy for runtime services, and treats the repository manifest as the source of truth. When an unambiguous application Compose workload exists, BaseHarbor generates a protected override, attaches it to the application backend network and injects container-routable native service URLs. Declared secrets.required entries are readiness gates.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(store, args, "apply")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			plan, err := application.BuildPlan(m)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Plan for %s (%s): %d actions\n", plan.Application, plan.Environment, len(plan.Actions))
			if resolved.FromRepository {
				fmt.Fprintf(out, "Manifest: %s (repository source of truth)\n", resolved.ManifestPath)
			}

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var compose bhruntime.Compose
			var platformFiles bhruntime.Files
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "application workload", Run: func(context.Context) error {
					return preflightRepositoryWorkload(resolved)
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

			files, err := application.EnsureRuntime(resolved.Store, m)
			if err != nil {
				return err
			}
			project := application.RuntimeProjectName(m)
			if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
				return err
			}

			var brokerFiles runtimebroker.Files
			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				credentialsPath := openbao.ApplicationCredentialsPath(files.Dir)
				if err := openbao.EnsureApplicationScope(ctx, compose, platformFiles, identity, credentialsPath); err != nil {
					return fmt.Errorf("converge OpenBao application secret scope: %w", err)
				}
				if err := checkRequiredApplicationSecrets(ctx, compose, platformFiles, m, files); err != nil {
					return fmt.Errorf("required secrets check failed: %w", err)
				}
				mtlsFiles, err := openbao.EnsureRuntimeMTLSIdentity(ctx, compose, platformFiles, identity, files)
				if err != nil {
					return fmt.Errorf("converge runtime mTLS identity: %w", err)
				}
				brokerFiles, err = runtimebroker.Ensure(m, files, mtlsFiles)
				if err != nil {
					return fmt.Errorf("materialize runtime secret broker: %w", err)
				}
				if err := compose.ConfigProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env); err != nil {
					return fmt.Errorf("validate runtime secret broker: %w", err)
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
					if verifyErr == nil {
						verifyErr = checkRequiredApplicationSecrets(verifyCtx, compose, platformFiles, m, files)
					}
				}
				if verifyErr == nil {
					break
				}
				select {
				case <-verifyCtx.Done():
				case <-time.After(time.Second):
				}
			}
			if verifyErr != nil {
				return fmt.Errorf("application verification failed: %w", verifyErr)
			}

			if m.Services.Secrets {
				if err := compose.UpProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env); err != nil {
					return fmt.Errorf("start runtime secret broker: %w", err)
				}
			}

			printRuntimeReady(out, m)
			if _, err := applyRepositoryWorkload(ctx, out, compose, resolved, files); err != nil {
				return err
			}
			fmt.Fprintf(out, "Application %s is ready.\n", m.Name)
			fmt.Fprintln(out, "Environment contract: baha app env --path")
			return nil
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
		fmt.Fprintln(out, "[OK] secret-broker     per-application mTLS runtime broker started")
		if len(m.Secrets.Required) > 0 {
			fmt.Fprintf(out, "[OK] required-secrets  %d declared secret(s) present\n", len(m.Secrets.Required))
		}
	}
}
