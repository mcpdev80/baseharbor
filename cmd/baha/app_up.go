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
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func appUpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "up",
		Summary: "Start an existing application runtime and verify readiness",
		Usage:   "baha app up [NAME]",
		Long:    "Starts a previously materialized BaseHarbor application runtime using its existing runtime definition, credentials and persistent data; required application secrets are verified before workload start and missing values fail closed. Repository workloads are started after their BaseHarbor backend and per-application secret broker are ready. Workload-only applications skip the empty managed-runtime start and resume their repository Compose workload directly. Without NAME it resolves the nearest repository baseharbor.yaml.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			resolved, err := resolveApplication(ctx, store, args, "up")
			if err != nil {
				return err
			}
			m := resolved.Manifest
			term := cli.NewTerminal(ctx, out, errOut)
			term.Header(m.Name, m.Environment)
			if err := printResolvedTracesPlacement(out, m); err != nil {
				return err
			}
			if err := printResolvedMetricsPlacement(out, m); err != nil {
				return err
			}
			if err := printResolvedLogsPlacement(out, resolved); err != nil {
				return err
			}
			files, err := application.ExistingRuntimeFiles(resolved.Store, m)
			if err != nil {
				return err
			}
			var compose bhruntime.Compose
			var before []bhruntime.ProjectResource
			var platformFiles bhruntime.Files
			var issuer serviceaccess.Issuer
			providers := &managedProviderPreflightState{}
			var workloadSecurity application.WorkloadSecurityReport
			checks := []preflight.Check{
				{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
				{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
				{Name: "manifest permissions", Run: func(context.Context) error {
					return checkManifestPermissions(resolved.ManifestPath, resolved.FromRepository)
				}},
				{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(resolved) }},
				{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(files) }},
				{Name: "managed runtime definition", Run: func(context.Context) error { return application.CheckManagedRuntimeDefinition(files, m) }},
				{Name: "runtime provider capabilities", Run: func(ctx context.Context) error {
					required := []bhruntime.RuntimeCapability{
						bhruntime.CapabilityWorkloadLifecycle,
						bhruntime.CapabilityResourceOwnership,
					}
					if m.Services.Secrets || requiresObjectStorageProviderAdmin(m) {
						required = append(required, bhruntime.CapabilityServiceExec)
					}
					var err error
					compose, err = detectComposeForApplication(ctx, resolved, required...)
					return err
				}},
				{Name: "BaseHarbor control-plane runtime", Run: func(context.Context) error {
					var err error
					platformFiles, err = existingTargetRuntimeFiles(ctx)
					if err != nil {
						return errors.New("BaseHarbor control-plane runtime is not materialized; run 'baha up' first")
					}
					return nil
				}},
				{Name: "managed service PKI", Run: func(ctx context.Context) error {
					issuer = openbao.NewServiceIssuer(compose, platformFiles)
					status, err := issuer.Status(ctx)
					if err != nil {
						return fmt.Errorf("managed service PKI is not ready: %w", err)
					}
					if !status.Ready {
						return errors.New("managed service PKI is not ready")
					}
					return nil
				}},
				{Name: "workload security", Run: func(ctx context.Context) error {
					var err error
					workloadSecurity, err = preflightRepositoryWorkloadSecurity(ctx, compose, resolved)
					return err
				}},
				{Name: "connectivity policy", Run: func(context.Context) error {
					_, err := application.LoadConnectivityRulesAt(resolved.TargetStateRoot)
					return err
				}},
				{Name: "provider registry", Run: func(context.Context) error {
					return application.CheckReferenceProviderRegistryAt(resolved.TargetStateRoot, m)
				}},

				{Name: "runtime configuration", Run: func(ctx context.Context) error {
					if !application.HasManagedRuntimeServices(m) {
						return nil
					}
					return compose.ConfigProject(ctx, files.Project, files.Compose, files.Env)
				}},
				{Name: "runtime ownership", Run: func(ctx context.Context) error {
					var err error
					before, err = application.InspectOwnedRuntimeResourcesForFiles(ctx, compose, m, files)
					return err
				}},
				{Name: "persistent data volumes", Run: func(context.Context) error {
					for _, volume := range application.ExpectedPersistentRuntimeResourcesForProject(m, files.Project) {
						if !application.ResourceNamedExists(before, volume) {
							return fmt.Errorf("managed %s is missing; refusing to recreate persistent state during app up", volume.Name)
						}
					}
					return nil
				}},
			}
			if application.RequiresRuntimeBroker(m) {
				checks = append(checks,
					preflight.Check{Name: "runtime PKI prerequisites", Run: func(ctx context.Context) error {
						identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
						return openbao.CheckApplicationProvisioning(ctx, compose, platformFiles, identity)
					}},
				)
				if m.Services.Secrets {
					checks = append(checks,
						preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
							identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
							return openbao.InspectApplicationScope(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
						}},
						preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
							return checkRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
						}},
					)
				}
			}
			checks = appendManagedProviderPreflights(checks, &compose, resolved, providers, &issuer)
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
				return errors.New("application up preflight failed")
			}
			if err := prepareUndeclaredProviderCleanup(ctx, compose, resolved, providers, issuer); err != nil {
				return fmt.Errorf("prepare obsolete provider cleanup: %w", err)
			}

			if err := application.EnsureBackendServiceAccess(ctx, issuer, files, m); err != nil {
				return fmt.Errorf("reconcile managed backend service access: %w", err)
			}

			if application.HasManagedRuntimeServices(m) {
				project := files.Project
				if err := activity(ctx, term, "Starting managed application services", func(progress io.Writer) error {
					return compose.UpProjectProgress(ctx, project, files.Compose, files.Env, func(detail string) {
						cli.ReportActivityDetail(progress, detail)
					})
				}); err != nil {
					return err
				}
			}
			if err := activity(ctx, term, "Reconciling object storage", func(progress io.Writer) error {
				return convergeManagedObjectStorage(ctx, progress, providers.objectStorage)
			}); err != nil {
				return err
			}

			verifyCtx, verifyCancel := context.WithTimeout(ctx, 60*time.Second)
			defer verifyCancel()
			if err := activity(ctx, term, "Waiting for backend readiness", func(progress io.Writer) error {
				cli.ReportActivityDetail(progress, "checking managed service readiness")
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
						cli.ReportActivityDetail(progress, "managed services ready")
						return nil
					}
					select {
					case <-verifyCtx.Done():
						return fmt.Errorf("application verification failed: %w", verifyErr)
					case <-time.After(time.Second):
					}
				}
				return fmt.Errorf("application verification timed out: %w", verifyCtx.Err())
			}); err != nil {
				return err
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
			if application.RequiresRuntimeBroker(m) {
				if err := activity(ctx, term, "Starting secure runtime broker", func(progress io.Writer) error {
					return ensureAndStartRuntimeBroker(ctx, progress, compose, platformFiles, m, files)
				}); err != nil {
					return err
				}
				printRuntimeBrokerDocs(out, files)
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
			if err := reconcileConnectivityForManifest(ctx, out, compose, resolved); err != nil {
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
			if err := application.ReconcileReferenceProviderRegistryAt(resolved.TargetStateRoot, m, managedLogsRegistryResources(providers.logs)...); err != nil {
				return fmt.Errorf("record provider registry after successful restart: %w", err)
			}
			if err := recordRepositoryAppliedFingerprint(ctx, resolved, files); err != nil {
				return fmt.Errorf("record successfully started repository desired state: %w", err)
			}
			term.Section("Application")
			term.Result("READY", "application", "runtime and requested infrastructure verified")
			fmt.Fprintln(out, "\nREADY")
			return nil
		},
	}
}
