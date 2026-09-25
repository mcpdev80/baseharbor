package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type applicationApplyExecution struct {
	resolved         resolvedApplication
	manifest         application.Manifest
	term             *cli.Terminal
	out              io.Writer
	errOut           io.Writer
	secretService    *applicationsecret.Service
	compose          bhruntime.Compose
	platformFiles    bhruntime.Files
	issuer           serviceaccess.Issuer
	providers        *managedProviderPreflightState
	workloadSecurity application.WorkloadSecurityReport
	files            application.RuntimeFiles
}

func newApplicationApplyExecution(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) (*applicationApplyExecution, error) {
	resolved, err := resolveApplication(ctx, store, args, "apply")
	if err != nil {
		return nil, err
	}
	m := resolved.Manifest
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header(m.Name, m.Environment)
	term.Info("target", resolved.Target.Name)

	plan, err := application.BuildPlan(m)
	if err != nil {
		return nil, err
	}
	term.Section("Plan")
	term.Info("desired actions", fmt.Sprintf("%d action(s) resolved", len(plan.Actions)))
	if resolved.FromRepository {
		fmt.Fprintf(out, "Manifest: %s (repository source of truth)\n", resolved.ManifestPath)
	}
	if err := printResolvedTracesPlacement(out, m); err != nil {
		return nil, err
	}
	if err := printResolvedMetricsPlacement(out, m); err != nil {
		return nil, err
	}
	if err := printResolvedLogsPlacement(out, resolved); err != nil {
		return nil, err
	}

	return &applicationApplyExecution{
		resolved:      resolved,
		manifest:      m,
		term:          term,
		out:           out,
		errOut:        errOut,
		secretService: applicationsecret.New(store),
		providers:     &managedProviderPreflightState{},
	}, nil
}

func (e *applicationApplyExecution) runPreflight(ctx context.Context) error {
	checks := e.preflightChecks()
	needsServiceIssuer := requiresManagedServiceIssuer(e.manifest)

	if needsServiceIssuer || application.RequiresRuntimeBroker(e.manifest) {
		checks = append(checks, preflight.Check{Name: "BaseHarbor control-plane runtime", Run: func(context.Context) error {
			var err error
			e.platformFiles, err = existingTargetRuntimeFiles(ctx)
			if err != nil {
				return errors.New("BaseHarbor control-plane runtime is not materialized; run 'baha up' first")
			}
			return nil
		}})
	}
	if needsServiceIssuer {
		checks = append(checks, preflight.Check{Name: "managed service PKI", Run: func(ctx context.Context) error {
			e.issuer = openbao.NewServiceIssuer(e.compose, e.platformFiles)
			status, err := e.issuer.Status(ctx)
			if err != nil {
				return fmt.Errorf("managed service PKI is not ready: %w", err)
			}
			if !status.Ready {
				return errors.New("managed service PKI is not ready")
			}
			return nil
		}})
	}
	if application.RequiresRuntimeBroker(e.manifest) {
		identity := openbao.ApplicationIdentity{Name: e.manifest.Name, Environment: e.manifest.Environment}
		checks = append(checks, preflight.Check{Name: "runtime PKI prerequisites", Run: func(ctx context.Context) error {
			return openbao.CheckApplicationProvisioning(ctx, e.compose, e.platformFiles, identity)
		}})
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
		return errors.New("application preflight failed")
	}
	if err := prepareUndeclaredProviderCleanup(ctx, e.compose, e.resolved, e.providers, e.issuer); err != nil {
		return fmt.Errorf("prepare obsolete provider cleanup: %w", err)
	}
	return nil
}

func (e *applicationApplyExecution) preflightChecks() []preflight.Check {
	m := e.manifest
	return []preflight.Check{
		{Name: "manifest", Run: func(context.Context) error { return m.Validate() }},
		{Name: "supported services", Run: func(context.Context) error { return application.CheckSupportedRuntimeServices(m) }},
		{Name: "manifest permissions", Run: func(context.Context) error {
			return checkManifestPermissions(e.resolved.ManifestPath, e.resolved.FromRepository)
		}},
		{Name: "application workload", Run: func(context.Context) error {
			return preflightRepositoryWorkload(e.resolved)
		}},
		{Name: "runtime provider capabilities", Run: func(ctx context.Context) error {
			required := []bhruntime.RuntimeCapability{bhruntime.CapabilityWorkloadLifecycle}
			if m.Services.Secrets || requiresObjectStorageProviderAdmin(m) {
				required = append(required, bhruntime.CapabilityServiceExec)
			}
			var err error
			e.compose, err = detectComposeForApplication(ctx, e.resolved, required...)
			return err
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
	}
}

func (e *applicationApplyExecution) prepareManagedRuntime(ctx context.Context) error {
	files, err := application.EnsureRuntime(ctx, e.issuer, e.resolved.Store, e.manifest)
	if err != nil {
		return err
	}
	e.files = files

	if application.HasManagedRuntimeServices(e.manifest) {
		if err := e.compose.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
			return err
		}
	}
	if err := activity(ctx, e.term, "Reconciling object storage", func(progress io.Writer) error {
		return convergeManagedObjectStorage(ctx, progress, e.providers.objectStorage)
	}); err != nil {
		return err
	}
	if err := e.prepareApplicationSecrets(ctx); err != nil {
		return err
	}
	if err := activity(ctx, e.term, "Starting managed application services", func(progress io.Writer) error {
		return startManagedRuntime(ctx, progress, e.compose, e.manifest, e.files)
	}); err != nil {
		return classifyOperationalFailure(err, "managed application services")
	}
	return nil
}

func (e *applicationApplyExecution) prepareApplicationSecrets(ctx context.Context) error {
	if !e.manifest.Services.Secrets {
		return nil
	}
	identity := openbao.ApplicationIdentity{Name: e.manifest.Name, Environment: e.manifest.Environment}
	credentialsPath := openbao.ApplicationCredentialsPath(e.files.Dir)
	if err := activity(ctx, e.term, "Preparing application secret scope", func(io.Writer) error {
		return openbao.EnsureApplicationScope(ctx, e.compose, e.platformFiles, identity, credentialsPath)
	}); err != nil {
		return err
	}
	generated, err := reconcileGeneratedApplicationSecrets(ctx, e.compose, e.platformFiles, e.manifest, e.files)
	if err != nil {
		return fmt.Errorf("generated secrets reconciliation failed: %w", err)
	}
	for _, name := range generated {
		fmt.Fprintf(e.out, "[OK] generated-secret  %s materialized in managed secret storage\n", name)
	}
	if err := resolveMissingRequiredSecretsInteractive(ctx, e.secretService, e.compose, e.platformFiles, e.manifest, e.files, e.out); err != nil {
		return err
	}
	if err := checkRequiredApplicationSecrets(ctx, e.compose, e.platformFiles, e.manifest, e.files); err != nil {
		return fmt.Errorf("required secrets check failed: %w", err)
	}
	return nil
}

func (e *applicationApplyExecution) verifyManagedRuntime(ctx context.Context) error {
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 60*time.Second)
	defer verifyCancel()

	var verifyErr error
	if err := activity(ctx, e.term, "Waiting for backend readiness", func(progress io.Writer) error {
		cli.ReportActivityDetail(progress, "checking managed service readiness")
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
	return nil
}

func (e *applicationApplyExecution) convergeApplicationRuntime(ctx context.Context) error {
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
	if err := e.startRepositoryWorkload(ctx); err != nil {
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

func (e *applicationApplyExecution) startRepositoryWorkload(ctx context.Context) error {
	if err := activity(ctx, e.term, "Starting repository workload", func(progress io.Writer) error {
		_, err := applyRepositoryWorkload(ctx, progress, e.compose, e.resolved, e.files)
		return err
	}); err != nil {
		resource := "repository workload"
		if len(e.manifest.Workload.Services) == 1 {
			resource = e.manifest.Workload.Services[0]
		}
		return classifyOperationalFailure(err, resource)
	}
	return nil
}

func (e *applicationApplyExecution) recordVerifiedDeployment(ctx context.Context) error {
	registryResources := managedLogsRegistryResources(e.providers.logs)
	registryResources = append(registryResources, managedTracesRegistryResources(e.providers.traces)...)
	if err := application.ReconcileReferenceProviderRegistryAt(e.resolved.TargetStateRoot, e.manifest, registryResources...); err != nil {
		return fmt.Errorf("record provider registry after successful convergence: %w", err)
	}
	if err := recordRepositoryAppliedFingerprint(ctx, e.resolved, e.files); err != nil {
		return fmt.Errorf("record successfully applied repository desired state: %w", err)
	}
	if err := recordAppliedDeployment(ctx, e.resolved, e.files); err != nil {
		return fmt.Errorf("record verified target deployment: %w", err)
	}
	e.term.Section("Application")
	if e.resolved.FromRepository && !e.term.Quiet() {
		fmt.Fprintln(e.out, "  Environment contract: baha app env --path")
	}
	e.term.Success("READY", "application and requested infrastructure verified")
	return nil
}
