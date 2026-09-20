package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

type managedTracesExecution struct {
	execution *capability.Execution
	driver    *tracesprovider.Driver
	runtime   bhruntime.Compose
	manifest  application.Manifest
	enabled   bool
	placement tracesprovider.Placement
	resources []capability.Resource
}

func prepareManagedTraces(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (*managedTracesExecution, error) {
	m := resolved.Manifest
	if !application.HasTraceSignal(m) {
		return nil, nil
	}
	enabled, err := application.TracesCollectionEnabled(m)
	if err != nil {
		return nil, err
	}
	prepared := &managedTracesExecution{runtime: compose, manifest: m, enabled: enabled}
	if !enabled {
		return prepared, nil
	}
	placement, err := tracesprovider.PlacementFor(m)
	if err != nil {
		return nil, err
	}
	prepared.placement = placement
	driver := tracesprovider.NewDriver(compose, m)
	resource := capability.Resource{
		Application: m.Name,
		Kind:        capability.Traces,
		Name:        "default",
		Provider:    capability.ProviderTempo,
	}
	if err := application.CheckAdditionalProviderResources(m, []capability.Resource{resource}); err != nil {
		return nil, fmt.Errorf("trace provider registry preflight: %w", err)
	}
	request := capability.Request{
		Requirement: capability.Requirement{Kind: capability.Traces, Name: "default"},
		Workload:    "platform/traces",
		Driver:      driver,
	}
	execution, _, err := capability.Prepare(ctx, m.Name, []capability.Request{request})
	if err != nil {
		return nil, err
	}
	prepared.execution = execution
	prepared.driver = driver
	prepared.resources = []capability.Resource{resource}
	return prepared, nil
}

func convergeManagedTracesBeforeTelemetry(ctx context.Context, out io.Writer, prepared *managedTracesExecution) error {
	if prepared == nil {
		return nil
	}
	if !prepared.enabled {
		fmt.Fprintf(out, "[SKIPPED] traces           trace storage disabled by deployment policy for %s\n", prepared.manifest.Name)
		return nil
	}
	if prepared.execution == nil {
		return nil
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "[READY] traces-provider  Tempo state converged for %s\n", prepared.manifest.Name)
	return nil
}

func verifyManagedTracesAfterTelemetry(ctx context.Context, out io.Writer, prepared *managedTracesExecution) error {
	if prepared == nil || !prepared.enabled || prepared.execution == nil {
		return nil
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "[VERIFIED] traces         verification trace ingested and queryable for %s\n", prepared.manifest.Name)
	return nil
}

func managedTracesRegistryResources(prepared *managedTracesExecution) []capability.Resource {
	if prepared == nil || !prepared.enabled {
		return nil
	}
	return append([]capability.Resource(nil), prepared.resources...)
}

func printResolvedTracesPlacement(out io.Writer, m application.Manifest) error {
	if !application.HasTraceSignal(m) {
		return nil
	}
	enabled, err := application.TracesCollectionEnabled(m)
	if err != nil {
		return err
	}
	if !enabled {
		fmt.Fprintln(out, "Provider placement: tempo -> not resolved (trace storage disabled by deployment policy)")
		return nil
	}
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderTempo)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Provider placement: tempo -> %s", placement.Scope)
	if placement.SharingBoundary != "" {
		fmt.Fprintf(out, " (sharing-boundary=%s)", placement.SharingBoundary)
	}
	fmt.Fprintln(out)
	return nil
}
