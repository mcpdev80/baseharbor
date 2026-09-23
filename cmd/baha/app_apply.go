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
)

func appApplyCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Summary: "Converge and verify an application's backend runtime",
		Usage:   "baha app apply [NAME]",
		Long:    "Runs plan, preflight, apply and verification. Without NAME it resolves the nearest baseharbor.yaml in the current repository, synchronizes a protected internal copy for runtime services, and treats the repository manifest as the source of truth. When an unambiguous application Compose workload exists, BaseHarbor generates a protected override, attaches it to the application backend network when managed backend services exist and injects container-routable native service URLs. Workload-only applications remain valid without inventing a managed database or cache. Declared secrets.required entries are readiness gates. Explicit secrets.required[].generate entries are created only when missing and are stored directly in OpenBao without printing their values. Managed-secret workloads start only after the per-application mTLS broker has proven app-scoped OpenBao readiness.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(store, args, "apply")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			term := cli.NewTerminal(ctx, out, errOut)
			term.Header(m.Name, m.Environment)
			plan, err := application.BuildPlan(m)
			if err != nil {
				return err
			}
			term.Section("Plan")
			term.Info("desired actions", fmt.Sprintf("%d action(s) resolved", len(plan.Actions)))
			if resolved.FromRepository {
				fmt.Fprintf(out, "Manifest: %s (repository source of truth)\n", resolved.ManifestPath)
			}
			if err := printResolvedTracesPlacement(out, m); err != nil {
				return err
			}
			if err := printResolvedMetricsPlacement(out, m); err != nil {
				return err
			}
			if err := printResolvedLogsPlacement(out, resolved); err != nil {
				return err
			}
			var compose bhruntime.Compose
			var platformFiles bhruntime.Files
			providers := &managedProviderPreflightState{}
			var workloadSecurity application.WorkloadSecurityReport
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "application workload", Run: func(context.Context) error {
					return preflightRepositoryWorkload(resolved)
				}},
				{Name: "runtime provider capabilities", Run: func(ctx context.Context) error {
					required := []bhruntime.RuntimeCapability{bhruntime.CapabilityWorkloadLifecycle}
					if m.Services.Secrets || requiresObjectStorageProviderAdmin(m) {
						required = append(required, bhruntime.CapabilityServiceExec)
					}
					var err error
					compose, err = detectComposeForApplication(ctx, resolved, required...)
					return err
				}},
				{Name: "workload security", Run: func(ctx context.Context) error {
					var err error
					workloadSecurity, err = preflightRepositoryWorkloadSecurity(ctx, compose, resolved)
					return err
				}},
				{Name: "connectivity policy", Run: func(context.Context) error {
					_, err := application.LoadConnectivityRules()
					return err
				}},
				{Name: "provider registry", Run: func(context.Context) error {
					return application.CheckReferenceProviderRegistry(m)
				}},
			}
			checks = appendManagedProviderPreflights(checks, &compose, resolved, providers)
			if application.RequiresRuntimeBroker(m) {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				checks = append(checks,
					preflight.Check{Name: "BaseHarbor control-plane runtime", Run: func(context.Context) error {
						var err error
						platformFiles, err = bhruntime.ExistingFiles("")
						return err
					}},
					preflight.Check{Name: "runtime PKI prerequisites", Run: func(ctx context.Context) error {
						if platformFiles.Compose == "" {
							return errors.New("BaseHarbor control-plane runtime is not materialized; run 'baha up' first")
						}
						return openbao.CheckApplicationProvisioning(ctx, compose, platformFiles, identity)
					}},
				)
			}
			var results []preflight.Result
			var ok bool
			if err := activity(ctx, term, "Checking application prerequisites", func(io.Writer) error {
				results, ok = preflight.RunWithTimeout(ctx, checks, 30*time.Second)
				return nil
			}); err != nil {
				return err
			}
			renderPreflightUX(term, results)
			if term.Verbose() {
				printWorkloadSecurityFindings(out, workloadSecurity)
			}
			if !ok {
				return errors.New("application preflight failed")
			}
			if err := prepareUndeclaredProviderCleanup(ctx, compose, resolved, providers); err != nil {
				return fmt.Errorf("prepare obsolete provider cleanup: %w", err)
			}

			files, err := application.EnsureRuntime(resolved.Store, m)
			if err != nil {
				return err
			}
			if application.HasManagedRuntimeServices(m) {
				project := application.RuntimeProjectName(m)
				if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
					return err
				}
			}
			if err := activity(ctx, term, "Reconciling object storage", func(progress io.Writer) error {
				return convergeManagedObjectStorage(ctx, progress, providers.objectStorage)
			}); err != nil {
				return err
			}

			if m.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
				credentialsPath := openbao.ApplicationCredentialsPath(files.Dir)
				if err := activity(ctx, term, "Preparing application secret scope", func(io.Writer) error {
					return openbao.EnsureApplicationScope(ctx, compose, platformFiles, identity, credentialsPath)
				}); err != nil {
					return err
				}
				generated, err := reconcileGeneratedApplicationSecrets(ctx, compose, platformFiles, m, files)
				if err != nil {
					return fmt.Errorf("generated secrets reconciliation failed: %w", err)
				}
				for _, name := range generated {
					fmt.Fprintf(out, "[OK] generated-secret  %s materialized in managed secret storage\n", name)
				}
				if err := checkRequiredApplicationSecrets(ctx, compose, platformFiles, m, files); err != nil {
					return fmt.Errorf("required secrets check failed: %w", err)
				}
			}

			if err := activity(ctx, term, "Starting managed application services", func(progress io.Writer) error {
				return startManagedRuntime(ctx, progress, compose, m, files)
			}); err != nil {
				return err
			}

			verifyCtx, verifyCancel := context.WithTimeout(ctx, 60*time.Second)
			defer verifyCancel()
			var verifyErr error
			if err := activity(ctx, term, "Waiting for backend readiness", func(progress io.Writer) error {
				cli.ReportActivityDetail(progress, "checking managed service readiness")
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
						cli.ReportActivityDetail(progress, "managed services ready")
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
				return nil
			}); err != nil {
				return err
			}

			if application.RequiresRuntimeBroker(m) {
				if err := activity(ctx, term, "Starting secure runtime broker", func(progress io.Writer) error {
					return ensureAndStartRuntimeBroker(ctx, progress, compose, platformFiles, m, files)
				}); err != nil {
					return err
				}
				printRuntimeBrokerDocs(out, files)
			}

			renderRuntimeReady(term, m)
			if err := activity(ctx, term, "Reconciling trace storage", func(progress io.Writer) error {
				return convergeManagedTracesBeforeTelemetry(ctx, progress, providers.traces)
			}); err != nil {
				return err
			}
			if err := activity(ctx, term, "Reconciling telemetry transport", func(progress io.Writer) error {
				return convergeManagedTelemetry(ctx, progress, providers.telemetry)
			}); err != nil {
				return err
			}
			if err := activity(ctx, term, "Verifying trace ingestion", func(progress io.Writer) error {
				return verifyManagedTracesAfterTelemetry(ctx, progress, providers.traces)
			}); err != nil {
				return err
			}
			if err := activity(ctx, term, "Reconciling log collection", func(progress io.Writer) error {
				return convergeManagedLogsBeforeWorkload(ctx, progress, files, providers.logs)
			}); err != nil {
				return err
			}
			if err := activity(ctx, term, "Reconciling metrics collection", func(progress io.Writer) error {
				return convergeManagedMetricsBeforeWorkload(ctx, progress, providers.metrics)
			}); err != nil {
				return err
			}
			if err := activity(ctx, term, "Starting repository workload", func(progress io.Writer) error {
				_, err := applyRepositoryWorkload(ctx, progress, compose, resolved, files)
				return err
			}); err != nil {
				return err
			}
			if err := reconcileConnectivityForManifest(ctx, out, compose, m); err != nil {
				return fmt.Errorf("reconcile cross-application connectivity: %w", err)
			}
			if err := activity(ctx, term, "Verifying metrics ingestion", func(progress io.Writer) error {
				return verifyManagedMetricsAfterWorkload(ctx, progress, providers.metrics)
			}); err != nil {
				return err
			}
			if err := activity(ctx, term, "Verifying log ingestion", func(progress io.Writer) error {
				return verifyManagedLogsAfterWorkload(ctx, progress, providers.logs)
			}); err != nil {
				return err
			}
			if err := activity(ctx, term, "Verifying application exposure", func(progress io.Writer) error {
				return convergeManagedExposure(ctx, progress, providers.exposure)
			}); err != nil {
				return err
			}
			registryResources := managedLogsRegistryResources(providers.logs)
			registryResources = append(registryResources, managedTracesRegistryResources(providers.traces)...)
			if err := application.ReconcileReferenceProviderRegistry(m, registryResources...); err != nil {
				return fmt.Errorf("record provider registry after successful convergence: %w", err)
			}
			if err := recordRepositoryAppliedFingerprint(ctx, resolved, files); err != nil {
				return fmt.Errorf("record successfully applied repository desired state: %w", err)
			}
			term.Section("Application")
			if resolved.FromRepository && !term.Quiet() {
				fmt.Fprintln(out, "  Environment contract: baha app env --path")
			}
			term.Success("READY", "application and requested infrastructure verified")
			return nil
		},
	}
}

func startManagedRuntime(ctx context.Context, out io.Writer, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.HasManagedRuntimeServices(m) {
		return nil
	}
	const maxAttempts = 3
	project := application.RuntimeProjectName(m)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := compose.UpProjectProgress(ctx, project, files.Compose, files.Env, func(detail string) {
			cli.ReportActivityDetail(out, detail)
		})
		if err == nil {
			return nil
		}
		if !bhruntime.IsPortBindingConflict(err) || attempt == maxAttempts {
			return err
		}

		if downErr := compose.DownProject(ctx, project, files.Compose, files.Env); downErr != nil {
			return errors.Join(err, fmt.Errorf("clean up partially started runtime before host-port retry: %w", downErr))
		}
		if reallocErr := application.ReallocateRuntimePorts(m, files); reallocErr != nil {
			return errors.Join(err, fmt.Errorf("reallocate application host ports: %w", reallocErr))
		}
		if configErr := compose.ConfigProject(ctx, project, files.Compose, files.Env); configErr != nil {
			return errors.Join(err, fmt.Errorf("validate runtime after host-port reallocation: %w", configErr))
		}
		fmt.Fprintf(out, "[RETRY] host-port conflict detected; reassigned loopback ports (attempt %d/%d)\n", attempt+1, maxAttempts)
	}
	return errors.New("application runtime start exhausted host-port retries")
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

func renderRuntimeReady(term *cli.Terminal, m application.Manifest) {
	term.Section("Services")
	if m.Services.Postgres {
		term.Result("READY", "PostgreSQL", "authenticated SELECT 1")
	}
	if m.Services.Redis {
		term.Result("READY", "Valkey", "authenticated PING")
	}
	if m.Services.Secrets {
		term.Result("VERIFIED", "secrets", "isolated OpenBao application scope")
		term.Result("READY", "runtime-broker", "mTLS identity verified")
		if len(m.Secrets.Required) > 0 {
			term.Result("VERIFIED", "required-secrets", fmt.Sprintf("%d declared secret(s) present", len(m.Secrets.Required)))
		}
	}
}
