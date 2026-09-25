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

type applicationUpExecution struct {
	resolved         resolvedApplication
	manifest         application.Manifest
	term             *cli.Terminal
	out              io.Writer
	files            application.RuntimeFiles
	compose          bhruntime.Compose
	before           []bhruntime.ProjectResource
	platformFiles    bhruntime.Files
	issuer           serviceaccess.Issuer
	providers        *managedProviderPreflightState
	workloadSecurity application.WorkloadSecurityReport
}

func executeApplicationUpLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	execution, err := newApplicationUpExecution(ctx, store, args, out, errOut)
	if err != nil {
		return err
	}
	if err := execution.runPreflight(ctx); err != nil {
		return err
	}
	if err := execution.startManagedRuntime(ctx); err != nil {
		return err
	}
	if err := execution.verifyManagedRuntime(ctx); err != nil {
		return err
	}
	if err := execution.convergeApplicationRuntime(ctx); err != nil {
		return err
	}
	return execution.finalize(ctx)
}

func newApplicationUpExecution(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) (*applicationUpExecution, error) {
	resolved, err := resolveApplication(ctx, store, args, "up")
	if err != nil {
		return nil, err
	}
	m := resolved.Manifest
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(m.Name, m.Environment)
	term.Info("target", resolved.Target.Name)
	if err := printResolvedTracesPlacement(out, m); err != nil {
		return nil, err
	}
	if err := printResolvedMetricsPlacement(out, m); err != nil {
		return nil, err
	}
	if err := printResolvedLogsPlacement(out, resolved); err != nil {
		return nil, err
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		return nil, err
	}
	return &applicationUpExecution{
		resolved:  resolved,
		manifest:  m,
		term:      term,
		out:       out,
		files:     files,
		providers: &managedProviderPreflightState{},
	}, nil
}

func (e *applicationUpExecution) runPreflight(ctx context.Context) error {
	checks := e.preflightChecks()
	if application.RequiresRuntimeBroker(e.manifest) {
		checks = append(checks, preflight.Check{Name: "runtime PKI prerequisites", Run: func(ctx context.Context) error {
			identity := openbao.ApplicationIdentity{Name: e.manifest.Name, Environment: e.manifest.Environment}
			return openbao.CheckApplicationProvisioning(ctx, e.compose, e.platformFiles, identity)
		}})
		if e.manifest.Services.Secrets {
			checks = append(checks,
				preflight.Check{Name: "OpenBao application scope", Run: func(ctx context.Context) error {
					identity := openbao.ApplicationIdentity{Name: e.manifest.Name, Environment: e.manifest.Environment}
					return openbao.InspectApplicationScope(ctx, e.compose, e.platformFiles, identity, openbao.ApplicationCredentialsPath(e.files.Dir))
				}},
				preflight.Check{Name: "required application secrets", Run: func(ctx context.Context) error {
					return checkRequiredApplicationSecrets(ctx, e.compose, e.platformFiles, e.manifest, e.files)
				}},
			)
		}
	}

	checks = appendManagedProviderPreflights(checks, &e.compose, e.resolved, e.providers, &e.issuer)

	var results []preflight.Result
	var ok bool
	if err := activity(ctx, e.term, "Checking application prerequisites", func(io.Writer) error {
		results, ok = preflight.RunWithTimeout(ctx, checks, 30*time.Second)
		return nil
	}); err != nil {
		return err
	}
	renderPreflightUX(e.term, results)
	if e.term.Verbose() {
		printWorkloadSecurityFindings(e.out, e.workloadSecurity)
	}
	if !ok {
		return errors.New("application up preflight failed")
	}
	if err := prepareUndeclaredProviderCleanup(ctx, e.compose, e.resolved, e.providers, e.issuer); err != nil {
		return fmt.Errorf("prepare obsolete provider cleanup: %w", err)
	}
	return nil
}

func (e *applicationUpExecution) preflightChecks() []preflight.Check {
	m := e.manifest
	return []preflight.Check{
		{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
		{Name: "supported desired services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
		{Name: "manifest permissions", Run: func(context.Context) error {
			return checkManifestPermissions(e.resolved.ManifestPath, e.resolved.FromRepository)
		}},
		{Name: "application workload", Run: func(context.Context) error { return preflightRepositoryWorkload(e.resolved) }},
		{Name: "runtime permissions", Run: func(context.Context) error { return application.CheckRuntimePermissions(e.files) }},
		{Name: "managed runtime definition", Run: func(context.Context) error { return application.CheckManagedRuntimeDefinition(e.files, m) }},
		{Name: "runtime provider capabilities", Run: func(ctx context.Context) error {
			required := []bhruntime.RuntimeCapability{
				bhruntime.CapabilityWorkloadLifecycle,
				bhruntime.CapabilityResourceOwnership,
			}
			if m.Services.Secrets || requiresObjectStorageProviderAdmin(m) {
				required = append(required, bhruntime.CapabilityServiceExec)
			}
			var err error
			e.compose, err = detectComposeForApplication(ctx, e.resolved, required...)
			return err
		}},
		{Name: "BaseHarbor control-plane runtime", Run: func(ctx context.Context) error {
			var err error
			e.platformFiles, err = existingTargetRuntimeFiles(ctx)
			if err != nil {
				return errors.New("BaseHarbor control-plane runtime is not materialized; run 'baha up' first")
			}
			return nil
		}},
		{Name: "managed service PKI", Run: func(ctx context.Context) error {
			e.issuer = openbao.NewServiceIssuer(e.compose, e.platformFiles)
			status, err := e.issuer.Status(ctx)
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
			e.workloadSecurity, err = preflightRepositoryWorkloadSecurity(ctx, e.compose, e.resolved)
			return err
		}},
		{Name: "connectivity policy", Run: func(context.Context) error {
			_, err := application.LoadConnectivityRulesAt(e.resolved.TargetStateRoot)
			return err
		}},
		{Name: "provider registry", Run: func(context.Context) error {
			return application.CheckReferenceProviderRegistryAt(e.resolved.TargetStateRoot, m)
		}},
		{Name: "runtime configuration", Run: func(ctx context.Context) error {
			if !application.HasManagedRuntimeServices(m) {
				return nil
			}
			return e.compose.ConfigProject(ctx, e.files.Project, e.files.Compose, e.files.Env)
		}},
		{Name: "runtime ownership", Run: func(ctx context.Context) error {
			var err error
			e.before, err = application.InspectOwnedRuntimeResourcesForFiles(ctx, e.compose, m, e.files)
			return err
		}},
		{Name: "persistent data volumes", Run: func(context.Context) error {
			for _, volume := range application.ExpectedPersistentRuntimeResourcesForProject(m, e.files.Project) {
				if !application.ResourceNamedExists(e.before, volume) {
					return fmt.Errorf("managed %s is missing; refusing to recreate persistent state during app up", volume.Name)
				}
			}
			return nil
		}},
	}
}

func (e *applicationUpExecution) startManagedRuntime(ctx context.Context) error {
	if err := application.EnsureBackendServiceAccess(ctx, e.issuer, e.files, e.manifest); err != nil {
		return fmt.Errorf("reconcile managed backend service access: %w", err)
	}
	if application.HasManagedRuntimeServices(e.manifest) {
		if err := activity(ctx, e.term, "Starting managed application services", func(progress io.Writer) error {
			return e.compose.UpProjectProgress(ctx, e.files.Project, e.files.Compose, e.files.Env, func(detail string) {
				cli.ReportActivityDetail(progress, detail)
			})
		}); err != nil {
			return err
		}
	}
	if err := activity(ctx, e.term, "Reconciling object storage", func(progress io.Writer) error {
		return convergeManagedObjectStorage(ctx, progress, e.providers.objectStorage)
	}); err != nil {
		return err
	}
	return nil
}

func (e *applicationUpExecution) verifyManagedRuntime(ctx context.Context) error {
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 60*time.Second)
	defer verifyCancel()

	return activity(ctx, e.term, "Waiting for backend readiness", func(progress io.Writer) error {
		cli.ReportActivityDetail(progress, "checking managed service readiness")
		var verifyErr error
		for verifyCtx.Err() == nil {
			verifyErr = verifyDesiredRuntimeServices(verifyCtx, e.compose, e.manifest, e.files)
			if verifyErr == nil && e.manifest.Services.Secrets {
				identity := openbao.ApplicationIdentity{Name: e.manifest.Name, Environment: e.manifest.Environment}
				verifyErr = openbao.CheckApplicationScope(verifyCtx, e.compose, e.platformFiles, identity, openbao.ApplicationCredentialsPath(e.files.Dir))
				if verifyErr == nil {
					verifyErr = checkRequiredApplicationSecrets(verifyCtx, e.compose, e.platformFiles, e.manifest, e.files)
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
	})
}

func (e *applicationUpExecution) convergeApplicationRuntime(ctx context.Context) error {
	renderRuntimeReady(e.term, e.manifest)
	if err := activity(ctx, e.term, "Reconciling trace storage", func(progress io.Writer) error {
		return convergeManagedTracesBeforeTelemetry(ctx, progress, e.providers.traces)
	}); err != nil {
		return err
	}
	if err := activity(ctx, e.term, "Reconciling telemetry transport", func(progress io.Writer) error {
		return convergeManagedTelemetry(ctx, progress, e.providers.telemetry)
	}); err != nil {
		return err
	}
	if application.RequiresRuntimeBroker(e.manifest) {
		if err := activity(ctx, e.term, "Starting secure runtime broker", func(progress io.Writer) error {
			return ensureAndStartRuntimeBroker(ctx, progress, e.compose, e.platformFiles, e.manifest, e.files)
		}); err != nil {
			return err
		}
		printRuntimeBrokerDocs(e.out, e.files)
	}
	if err := activity(ctx, e.term, "Verifying trace ingestion", func(progress io.Writer) error {
		return verifyManagedTracesAfterTelemetry(ctx, progress, e.providers.traces)
	}); err != nil {
		return err
	}
	if err := activity(ctx, e.term, "Reconciling log collection", func(progress io.Writer) error {
		return convergeManagedLogsBeforeWorkload(ctx, progress, e.files, e.providers.logs)
	}); err != nil {
		return err
	}
	if err := activity(ctx, e.term, "Reconciling metrics collection", func(progress io.Writer) error {
		return convergeManagedMetricsBeforeWorkload(ctx, progress, e.providers.metrics)
	}); err != nil {
		return err
	}
	if err := activity(ctx, e.term, "Starting repository workload", func(progress io.Writer) error {
		_, err := applyRepositoryWorkload(ctx, progress, e.compose, e.resolved, e.files)
		return err
	}); err != nil {
		return err
	}
	if err := reconcileConnectivityForManifest(ctx, e.out, e.compose, e.resolved); err != nil {
		return fmt.Errorf("reconcile cross-application connectivity: %w", err)
	}
	if err := activity(ctx, e.term, "Verifying metrics ingestion", func(progress io.Writer) error {
		return verifyManagedMetricsAfterWorkload(ctx, progress, e.providers.metrics)
	}); err != nil {
		return err
	}
	if err := activity(ctx, e.term, "Verifying log ingestion", func(progress io.Writer) error {
		return verifyManagedLogsAfterWorkload(ctx, progress, e.providers.logs)
	}); err != nil {
		return err
	}
	if err := activity(ctx, e.term, "Verifying application exposure", func(progress io.Writer) error {
		return convergeManagedExposure(ctx, progress, e.providers.exposure)
	}); err != nil {
		return err
	}
	return nil
}

func (e *applicationUpExecution) finalize(ctx context.Context) error {
	if err := application.ReconcileReferenceProviderRegistryAt(e.resolved.TargetStateRoot, e.manifest, managedLogsRegistryResources(e.providers.logs)...); err != nil {
		return fmt.Errorf("record provider registry after successful restart: %w", err)
	}
	if err := recordRepositoryAppliedFingerprint(ctx, e.resolved, e.files); err != nil {
		return fmt.Errorf("record successfully started repository desired state: %w", err)
	}
	e.term.Section("Application")
	e.term.Result("READY", "application", "runtime and requested infrastructure verified")
	fmt.Fprintln(e.out, "\nREADY")
	return nil
}
