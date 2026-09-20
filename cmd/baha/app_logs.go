package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type managedLogsExecution struct {
	execution *capability.Execution
	driver *logsprovider.Driver
	runtime bhruntime.Compose
	manifest application.Manifest
	services []string
	resources []capability.Resource
	enabled bool
}

func prepareManagedLogs(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (*managedLogsExecution, error) {
	if !resolved.FromRepository {
		return nil, nil
	}
	repositoryRoot := filepath.Dir(resolved.ManifestPath)
	services, _, found, err := application.SelectedWorkloadServices(repositoryRoot, resolved.Manifest)
	if err != nil || !found {
		return nil, err
	}
	policy, err := application.LogsPolicy(resolved.Manifest)
	if err != nil {
		return nil, err
	}
	prepared := &managedLogsExecution{
		runtime: compose,
		manifest: resolved.Manifest,
		services: services,
		enabled: policy.Enabled && policy.Collect[application.LogsSourceApplication],
	}
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
	prepared.driver = logsprovider.NewDriver(compose, resolved.Manifest)
	requests := make([]capability.Request, 0, len(services))
	resources := make([]capability.Resource, 0, len(services))
	for _, service := range services {
		resources = append(resources, capability.Resource{
			Application: m.Name,
			Kind: capability.Logs,
			Name: service,
			Provider: capability.ProviderLoki,
		})
		requests = append(requests, capability.Request{
			Requirement: capability.Requirement{Kind: capability.Logs, Name: service},
			Workload: "service/" + service,
			Logs: &capability.LogsBinding{
				Direction: "collect",
				Format: "syslog-rfc5424",
				Service: service,
			},
			Driver: prepared.driver,
		})
	}
	if err := application.CheckAdditionalProviderResources(m, resources); err != nil {
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
		if err := logsprovider.UnregisterApplication(ctx, prepared.runtime, prepared.manifest); err != nil {
			return err
		}
		fmt.Fprintf(out, "[SKIP] logs               application log collection disabled by deployment policy for %s\n", prepared.manifest.Name)
		return nil
	}
	if prepared.execution == nil {
		return nil
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	if _, err := logsprovider.EnsureWorkloadOverride(prepared.manifest, files, prepared.services); err != nil {
		return err
	}
	fmt.Fprintf(out, "[OK] logs-provider      Loki/Alloy collector state converged for %s\n", prepared.manifest.Name)
	return nil
}

func managedLogsRegistryResources(prepared *managedLogsExecution) []capability.Resource {
	if prepared == nil || !prepared.enabled {
		return nil
	}
	return append([]capability.Resource(nil), prepared.resources...)
}

func verifyManagedLogsAfterWorkload(ctx context.Context, out io.Writer, prepared *managedLogsExecution) error {
	if prepared == nil || !prepared.enabled || prepared.execution == nil {
		return nil
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "[OK] logs               %d workload service log stream(s) ingested for %s\n", len(prepared.services), prepared.manifest.Name)
	return nil
}

func printResolvedLogsPlacement(out io.Writer, resolved resolvedApplication) error {
	if !resolved.FromRepository {
		return nil
	}
	repositoryRoot := filepath.Dir(resolved.ManifestPath)
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
