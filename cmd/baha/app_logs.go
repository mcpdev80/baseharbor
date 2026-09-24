package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type managedLogsExecution struct {
	execution                   *capability.Execution
	issuer                      serviceaccess.Issuer
	driver                      *logsprovider.Driver
	runtime                     bhruntime.Compose
	manifest                    application.Manifest
	services                    []string
	resources                   []capability.Resource
	providerSources             []observability.SignalSource
	enabled                     bool
	workloadEnabled             bool
	includeApplicationProviders bool
	includePlatformProviders    bool
}

func prepareManagedLogs(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, issuer serviceaccess.Issuer) (*managedLogsExecution, error) {
	policy, err := application.LogsPolicy(resolved.Manifest)
	if err != nil {
		return nil, err
	}
	prepared := &managedLogsExecution{
		runtime:                     compose,
		issuer:                      issuer,
		manifest:                    resolved.Manifest,
		includeApplicationProviders: policy.Enabled && policy.Collect[application.LogsSourceApplicationProvider],
		includePlatformProviders:    policy.Enabled && policy.Collect[application.LogsSourcePlatformProvider],
	}
	if resolved.FromRepository {
		repositoryRoot := resolved.repositoryRoot()
		services, _, found, err := application.SelectedWorkloadServices(repositoryRoot, resolved.Manifest)
		if err != nil {
			return nil, err
		}
		if found {
			prepared.services = services
			prepared.workloadEnabled = policy.Enabled && policy.Collect[application.LogsSourceApplication] && len(services) > 0
		}
	}
	prepared.enabled = prepared.workloadEnabled || prepared.includeApplicationProviders || prepared.includePlatformProviders
	if !prepared.enabled {
		return prepared, nil
	}
	placement, err := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderLoki)
	if err != nil {
		return nil, err
	}
	if placement.Scope == capability.ScopeExternal {
		return nil, fmt.Errorf("external Loki placement is selected but no external Compose log collector adapter is configured")
	}
	prepared.driver = logsprovider.NewDriver(compose, resolved.Manifest, issuer)
	if !prepared.workloadEnabled {
		return prepared, nil
	}

	requests := make([]capability.Request, 0, len(prepared.services))
	resources := make([]capability.Resource, 0, len(prepared.services))
	for _, service := range prepared.services {
		resources = append(resources, capability.Resource{
			Application: resolved.Manifest.Name,
			Kind:        capability.Logs,
			Name:        service,
			Provider:    capability.ProviderLoki,
		})
		requests = append(requests, capability.Request{
			Requirement: capability.Requirement{Kind: capability.Logs, Name: service},
			Workload:    "service/" + service,
			Logs: &capability.LogsBinding{
				Direction: "collect",
				Format:    "syslog-rfc5424",
				Service:   service,
			},
			Driver: prepared.driver,
		})
	}
	if err := application.CheckAdditionalProviderResources(resolved.Manifest, resources); err != nil {
		return nil, fmt.Errorf("logs provider registry preflight: %w", err)
	}
	execution, _, err := capability.Prepare(ctx, resolved.Manifest.Name, requests)
	if err != nil {
		return nil, err
	}
	prepared.execution = execution
	prepared.resources = resources
	return prepared, nil
}

func convergeManagedLogsBeforeWorkload(ctx context.Context, out io.Writer, files application.RuntimeFiles, prepared *managedLogsExecution) error {
	if prepared == nil {
		return nil
	}
	if !prepared.enabled {
		if err := logsprovider.RemoveWorkloadOverride(files); err != nil {
			return err
		}
		if err := logsprovider.RemoveProviderSourceOverride(files); err != nil {
			return err
		}
		if err := logsprovider.UnregisterApplication(ctx, prepared.runtime, prepared.issuer, prepared.manifest); err != nil {
			return err
		}
		if err := reconcileRuntimeComponentLogOverrides(ctx, prepared.runtime, prepared.manifest, files); err != nil {
			return err
		}
		fmt.Fprintf(out, "[SKIPPED] logs             log collection disabled by deployment policy for %s\n", prepared.manifest.Name)
		return nil
	}

	placement, err := application.ResolveProviderPlacement(prepared.manifest, capability.ProviderLoki)
	if err != nil {
		return err
	}
	providerSources, err := observability.ListLogs(
		placement,
		[]string{prepared.manifest.Name},
		prepared.includeApplicationProviders,
		prepared.includePlatformProviders,
	)
	if err != nil {
		return fmt.Errorf("resolve provider log sources: %w", err)
	}
	prepared.providerSources = providerSources

	if prepared.execution != nil {
		if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
			return err
		}
	} else if len(providerSources) > 0 {
		if err := prepared.driver.Provision(ctx, capability.Resource{}, capability.Binding{}); err != nil {
			return err
		}
	} else {
		if err := logsprovider.RemoveWorkloadOverride(files); err != nil {
			return err
		}
		if err := logsprovider.RemoveProviderSourceOverride(files); err != nil {
			return err
		}
		return nil
	}

	if prepared.workloadEnabled {
		if _, err := logsprovider.EnsureWorkloadOverrideForRuntime(prepared.manifest, files, prepared.services, prepared.runtime.Engine()); err != nil {
			return err
		}
	} else if err := logsprovider.RemoveWorkloadOverride(files); err != nil {
		return err
	}
	if _, _, err := logsprovider.EnsureProviderSourceOverrideForRuntime(prepared.manifest, files, prepared.runtime.Engine()); err != nil {
		return err
	}
	if err := reconcileRuntimeComponentLogOverrides(ctx, prepared.runtime, prepared.manifest, files); err != nil {
		return err
	}
	fmt.Fprintf(out, "[READY] logs-provider   Loki/Alloy collector state converged for %s (%d provider source(s) authorized)\n", prepared.manifest.Name, len(providerSources))
	return nil
}

func reconcileRuntimeComponentLogOverrides(ctx context.Context, runtime bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if application.RequiresRuntimeBroker(m) {
		brokerFiles, err := runtimebroker.Existing(files)
		if err == nil {
			override, found, err := logsprovider.EnsureRuntimeProjectOverrideForRuntime(
				m,
				files.Dir,
				"broker.logging.override.yaml",
				runtimebroker.ProjectName(m),
				runtime.Engine(),
				observability.SourceApplicationProvider,
			)
			if err != nil {
				return fmt.Errorf("materialize runtime broker log collection: %w", err)
			}
			composeFiles := []string{brokerFiles.Compose}
			if found {
				composeFiles = append(composeFiles, override)
			}
			workdir := filepath.Dir(brokerFiles.Compose)
			if err := runtime.ConfigProjectFiles(ctx, runtimebroker.ProjectName(m), workdir, composeFiles...); err != nil {
				return fmt.Errorf("validate runtime broker log collection: %w", err)
			}
			if err := runtime.UpProjectFiles(ctx, runtimebroker.ProjectName(m), workdir, composeFiles...); err != nil {
				return fmt.Errorf("reconcile runtime broker log collection: %w", err)
			}
		}
	}

	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	if executorFiles, err := runtimeexecutor.ExistingFiles(dataDir); err == nil {
		override, found, err := logsprovider.EnsureRuntimeProjectOverrideForRuntime(
			m,
			executorFiles.Dir,
			"observability.logging.override.yaml",
			runtimeexecutor.ProjectName,
			runtime.Engine(),
			observability.SourcePlatformProvider,
		)
		if err != nil {
			return fmt.Errorf("materialize runtime executor log collection: %w", err)
		}
		composeFiles := []string{executorFiles.Compose}
		if found {
			composeFiles = append(composeFiles, override)
		}
		if err := runtime.ConfigProjectFiles(ctx, runtimeexecutor.ProjectName, executorFiles.Dir, composeFiles...); err != nil {
			return fmt.Errorf("validate runtime executor log collection: %w", err)
		}
		if err := runtime.UpProjectFiles(ctx, runtimeexecutor.ProjectName, executorFiles.Dir, composeFiles...); err != nil {
			return fmt.Errorf("reconcile runtime executor log collection: %w", err)
		}
	}
	return nil
}

func managedLogsRegistryResources(prepared *managedLogsExecution) []capability.Resource {
	if prepared == nil || !prepared.enabled {
		return nil
	}
	return append([]capability.Resource(nil), prepared.resources...)
}

func verifyManagedLogsAfterWorkload(ctx context.Context, out io.Writer, prepared *managedLogsExecution) error {
	if prepared == nil || !prepared.enabled {
		return nil
	}
	if prepared.execution != nil {
		if _, err := prepared.execution.Verify(ctx); err != nil {
			return err
		}
	}
	if err := logsprovider.VerifyProviderSources(ctx, prepared.manifest, prepared.providerSources); err != nil {
		return err
	}
	fmt.Fprintf(out, "[VERIFIED] logs          %d workload stream(s), %d provider stream(s) ingested for %s\n", len(prepared.services), len(prepared.providerSources), prepared.manifest.Name)
	return nil
}

func printResolvedLogsPlacement(out io.Writer, resolved resolvedApplication) error {
	if !resolved.FromRepository || !application.HasLogsCollection(resolved.Manifest) {
		return nil
	}
	repositoryRoot := resolved.repositoryRoot()
	services, _, found, err := application.SelectedWorkloadServices(repositoryRoot, resolved.Manifest)
	if err != nil || !found || len(services) == 0 {
		return err
	}
	policy, err := application.LogsPolicy(resolved.Manifest)
	if err != nil {
		return err
	}
	if !policy.Enabled || !policy.Collect[application.LogsSourceApplication] {
		fmt.Fprintln(out, "Provider placement: loki -> not resolved (log collection disabled by deployment policy)")
		return nil
	}
	placement, err := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderLoki)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Provider placement: loki -> %s", placement.Scope)
	if placement.SharingBoundary != "" {
		fmt.Fprintf(out, " (sharing-boundary=%s)", placement.SharingBoundary)
	}
	fmt.Fprintln(out)
	return nil
}
