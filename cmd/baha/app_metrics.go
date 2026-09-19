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
	enabled := policy.Enabled

	if policy.ProviderScope == capability.ScopeExternal && enabled && len(m.Metrics.Sources) > 0 {
		return nil, fmt.Errorf("external metrics scope is selected but no external collection adapter is configured")
	}

	if len(m.Metrics.Sources) == 0 || !enabled {
		if _, err := metricsprovider.ExistingProviderFiles(m); err == nil {
			return &managedMetricsExecution{
				driver:   metricsprovider.NewDriver(compose, m),
				manifest: m,
				enabled:  enabled,
			}, nil
		}
		return nil, nil
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
	if !prepared.enabled || prepared.execution == nil {
		if err := metricsprovider.PruneApplicationTargets(prepared.manifest, nil); err != nil {
			return err
		}
		if len(prepared.manifest.Metrics.Sources) > 0 && !prepared.enabled {
			fmt.Fprintf(out, "[SKIP] metrics            collection disabled by deployment policy for %s\n", prepared.manifest.Name)
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
