package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type managedMetricsExecution struct {
	execution           *capability.Execution
	driver              *metricsprovider.Driver
	runtime             bhruntime.Compose
	manifest            application.Manifest
	enabled             bool
	desiredPlacement    capability.ProviderPlacement
	registeredPlacement capability.ProviderPlacement
	registered          bool
	placementChanged    bool
}

func prepareManagedMetrics(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (*managedMetricsExecution, error) {
	m := resolved.Manifest
	hasMetricsIntent := len(m.Metrics.Sources) > 0 || application.HasRuntimeMetricsPermissions(m)

	registeredPlacement, registered, err := application.RegisteredProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return nil, err
	}
	if !hasMetricsIntent {
		if !registered {
			return nil, nil
		}
		return &managedMetricsExecution{
			runtime:             compose,
			manifest:            m,
			registeredPlacement: registeredPlacement,
			registered:          true,
		}, nil
	}

	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return nil, err
	}
	enabled := policy.Enabled && policy.Collect[application.MetricsSourceApplication]
	if !enabled {
		return &managedMetricsExecution{
			runtime:             compose,
			manifest:            m,
			enabled:             false,
			registeredPlacement: registeredPlacement,
			registered:          registered,
		}, nil
	}

	desiredPlacement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return nil, err
	}
	if desiredPlacement.Scope == capability.ScopeExternal {
		return nil, fmt.Errorf("external Prometheus placement is selected but no external metrics collection adapter is configured")
	}

	runtimeFiles := application.RuntimeFilesFor(resolved.Store, m)
	runtimeCA := filepath.Join(application.RuntimeMTLSHostDir(runtimeFiles), "ca.pem")
	prepared := &managedMetricsExecution{
		driver:              metricsprovider.NewDriver(compose, m, runtimeCA),
		runtime:             compose,
		manifest:            m,
		enabled:             true,
		desiredPlacement:    desiredPlacement,
		registeredPlacement: registeredPlacement,
		registered:          registered,
		placementChanged:    registered && desiredPlacement != registeredPlacement,
	}

	runtimeMetrics := application.HasRuntimeMetricsPermissions(m)
	if len(m.Metrics.Sources) == 0 && runtimeMetrics {
		return prepared, nil
	}

	runtimeTLS := make(map[string]struct{})
	for _, service := range application.RuntimeAuthorizedServices(m) {
		runtimeTLS[service] = struct{}{}
	}
	requests := make([]capability.Request, 0, len(m.Metrics.Sources))
	for _, source := range m.Metrics.Sources {
		scheme := "http"
		if _, ok := runtimeTLS[source.Service]; ok {
			scheme = "https"
		}
		requests = append(requests, capability.Request{
			Requirement: capability.Requirement{Kind: capability.Metrics, Name: source.Name},
			Workload:    "service/" + source.Service,
			Metrics: &capability.MetricsBinding{
				Direction: "provide",
				Format:    "openmetrics",
				Service:   source.Service,
				Scheme:    scheme,
				Port:      source.Port,
				Path:      source.Path,
			},
			Driver: prepared.driver,
		})
	}
	execution, _, err := capability.Prepare(ctx, m.Name, requests)
	if err != nil {
		return nil, err
	}
	prepared.execution = execution
	return prepared, nil
}

func convergeManagedMetricsBeforeWorkload(ctx context.Context, out io.Writer, prepared *managedMetricsExecution) error {
	if prepared == nil {
		return nil
	}
	if !prepared.enabled {
		if err := cleanupRegisteredMetricsPlacement(ctx, prepared); err != nil {
			return err
		}
		if len(prepared.manifest.Metrics.Sources) > 0 || application.HasRuntimeMetricsPermissions(prepared.manifest) {
			fmt.Fprintf(out, "[SKIPPED] metrics          application-source collection disabled by deployment policy for %s\n", prepared.manifest.Name)
		} else if prepared.registered {
			fmt.Fprintf(out, "[REMOVED] metrics         obsolete registered metrics provider state for %s\n", prepared.manifest.Name)
		}
		return nil
	}
	if prepared.execution == nil {
		if application.HasRuntimeMetricsPermissions(prepared.manifest) {
			if err := prepared.driver.Provision(ctx, capability.Resource{}, capability.Binding{}); err != nil {
				return err
			}
			fmt.Fprintf(out, "[READY] metrics-provider runtime source collection for %s\n", prepared.manifest.Name)
		}
		return nil
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	if err := metricsprovider.PruneApplicationTargets(prepared.manifest, metricsprovider.DesiredTargetFiles(prepared.manifest)); err != nil {
		return err
	}
	fmt.Fprintf(out, "[UPDATED] metrics-provider Prometheus target state for %s\n", prepared.manifest.Name)
	return nil
}

func verifyManagedMetricsAfterWorkload(ctx context.Context, out io.Writer, prepared *managedMetricsExecution) error {
	if prepared == nil || !prepared.enabled {
		return nil
	}
	if prepared.execution != nil {
		if _, err := prepared.execution.Verify(ctx); err != nil {
			return err
		}
		fmt.Fprintf(out, "[VERIFIED] metrics       %d source(s) scraped and ingested for %s\n", len(prepared.manifest.Metrics.Sources), prepared.manifest.Name)
	}
	if err := metricsprovider.VerifyProviderSources(ctx, prepared.manifest); err != nil {
		return err
	}
	if prepared.placementChanged {
		if err := cleanupRegisteredMetricsPlacement(ctx, prepared); err != nil {
			return fmt.Errorf("remove previous metrics placement after successful convergence: %w", err)
		}
		fmt.Fprintf(out, "[UPDATED] metrics-placement migrated %s from %s to %s\n", prepared.manifest.Name, prepared.registeredPlacement.Scope, prepared.desiredPlacement.Scope)
	}
	return nil
}

func cleanupRegisteredMetricsPlacement(ctx context.Context, prepared *managedMetricsExecution) error {
	if prepared == nil || !prepared.registered {
		return nil
	}
	switch prepared.registeredPlacement.Scope {
	case capability.ScopeShared:
		if err := metricsprovider.PruneRegisteredApplicationTargets(prepared.manifest, nil); err != nil {
			return err
		}
		return metricsprovider.UnregisterSharedApplication(ctx, prepared.runtime, prepared.manifest)
	case capability.ScopeApplication:
		return metricsprovider.DestroyProvider(ctx, prepared.runtime, prepared.manifest)
	case capability.ScopeExternal:
		return nil
	default:
		return fmt.Errorf("unsupported registered metrics provider scope %q", prepared.registeredPlacement.Scope)
	}
}

func printResolvedMetricsPlacement(out io.Writer, m application.Manifest) error {
	if len(m.Metrics.Sources) == 0 && !application.HasRuntimeMetricsPermissions(m) {
		return nil
	}
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return err
	}
	if !policy.Enabled || !policy.Collect[application.MetricsSourceApplication] {
		fmt.Fprintln(out, "Provider placement: prometheus -> not resolved (metrics collection disabled by deployment policy)")
		return nil
	}
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Provider placement: prometheus -> %s", placement.Scope)
	if placement.SharingBoundary != "" {
		fmt.Fprintf(out, " (sharing-boundary=%s)", placement.SharingBoundary)
	}
	if placement.Scope == capability.ScopeExternal && placement.ExternalReference != "" {
		fmt.Fprintf(out, " (reference=%s)", placement.ExternalReference)
	}
	fmt.Fprintln(out)
	return nil
}
