package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type managedMetricsExecution struct {
	execution *capability.Execution
	driver    *metricsprovider.Driver
	manifest  application.Manifest
	enabled   bool
}

func prepareManagedMetrics(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (*managedMetricsExecution, error) {
	m := resolved.Manifest
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return nil, err
	}
	enabled := policy.Enabled && policy.Collect[application.MetricsSourceApplication]
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return nil, err
	}

	if placement.Scope == capability.ScopeExternal && enabled && (len(m.Metrics.Sources) > 0 || application.HasRuntimeMetricsPermissions(m)) {
		return nil, fmt.Errorf("external Prometheus placement is selected but no external metrics collection adapter is configured")
	}

	runtimeMetrics := application.HasRuntimeMetricsPermissions(m)
	if (len(m.Metrics.Sources) == 0 && !runtimeMetrics) || !enabled {
		if _, err := metricsprovider.ExistingProviderFiles(m); err == nil {
			return &managedMetricsExecution{
				driver:   metricsprovider.NewDriver(compose, m),
				manifest: m,
				enabled:  enabled,
			}, nil
		}
		return nil, nil
	}
	if len(m.Metrics.Sources) == 0 && runtimeMetrics {
		return &managedMetricsExecution{
			driver:   metricsprovider.NewDriver(compose, m),
			manifest: m,
			enabled:  true,
		}, nil
	}

	driver := metricsprovider.NewDriver(compose, m)
	requests := make([]capability.Request, 0, len(m.Metrics.Sources))
	for _, source := range m.Metrics.Sources {
		requests = append(requests, capability.Request{
			Requirement: capability.Requirement{Kind: capability.Metrics, Name: source.Name},
			Workload:    "service/" + source.Service,
			Metrics: &capability.MetricsBinding{
				Direction: "provide",
				Format:    "openmetrics",
				Service:   source.Service,
				Port:      source.Port,
				Path:      source.Path,
			},
			Driver: driver,
		})
	}
	execution, _, err := capability.Prepare(ctx, m.Name, requests)
	if err != nil {
		return nil, err
	}
	return &managedMetricsExecution{
		execution: execution,
		driver:    driver,
		manifest:  m,
		enabled:   true,
	}, nil
}

func convergeManagedMetricsBeforeWorkload(ctx context.Context, out io.Writer, prepared *managedMetricsExecution) error {
	if prepared == nil {
		return nil
	}
	if !prepared.enabled {
		if err := metricsprovider.PruneApplicationTargets(prepared.manifest, nil); err != nil {
			return err
		}
		if len(prepared.manifest.Metrics.Sources) > 0 || application.HasRuntimeMetricsPermissions(prepared.manifest) {
			fmt.Fprintf(out, "[SKIP] metrics            application-source collection disabled by deployment policy for %s\n", prepared.manifest.Name)
		}
		return nil
	}
	if prepared.execution == nil {
		if application.HasRuntimeMetricsPermissions(prepared.manifest) {
			if err := prepared.driver.Provision(ctx, capability.Resource{}, capability.Binding{}); err != nil {
				return err
			}
			fmt.Fprintf(out, "[OK] metrics-provider    runtime source collection ready for %s\n", prepared.manifest.Name)
		}
		return nil
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	if err := metricsprovider.PruneApplicationTargets(prepared.manifest, metricsprovider.DesiredTargetFiles(prepared.manifest)); err != nil {
		return err
	}
	fmt.Fprintf(out, "[OK] metrics-provider    Prometheus target state converged for %s\n", prepared.manifest.Name)
	return nil
}

func verifyManagedMetricsAfterWorkload(ctx context.Context, out io.Writer, prepared *managedMetricsExecution) error {
	if prepared == nil || !prepared.enabled || prepared.execution == nil {
		return nil
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "[OK] metrics            %d source(s) scraped and ingested for %s\n", len(prepared.manifest.Metrics.Sources), prepared.manifest.Name)
	return nil
}

func printResolvedMetricsPlacement(out io.Writer, m application.Manifest) error {
	if len(m.Metrics.Sources) == 0 && !application.HasRuntimeMetricsPermissions(m) {
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

