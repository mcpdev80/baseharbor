package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

type managedTracesExecution struct {
	execution       *capability.Execution
	driver          *tracesprovider.Driver
	runtime         bhruntime.Compose
	manifest        application.Manifest
	enabled         bool
	placement       tracesprovider.Placement
	resources       []capability.Resource
	providerSources []observability.SignalSource
	runtimeFiles    application.RuntimeFiles
}

func prepareManagedTraces(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication, issuer serviceaccess.Issuer) (*managedTracesExecution, error) {
	m := resolved.Manifest
	if !application.HasTraceSignal(m) {
		return nil, nil
	}
	policy, err := application.TracesPolicy(m)
	if err != nil {
		return nil, err
	}
	enabled := policy.Enabled && (policy.Collect[application.TracesSourceApplication] || policy.Collect[application.TracesSourceApplicationProvider] || policy.Collect[application.TracesSourcePlatformProvider])
	prepared := &managedTracesExecution{
		runtime:      compose,
		manifest:     m,
		enabled:      enabled,
		runtimeFiles: application.RuntimeFilesFor(resolved.Store, m),
	}
	if !enabled {
		return prepared, nil
	}
	placement, err := tracesprovider.PlacementFor(m)
	if err != nil {
		return nil, err
	}
	prepared.placement = placement
	driver := tracesprovider.NewDriver(compose, m, issuer)
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
	if err := refreshProviderTraceSources(prepared); err != nil {
		return err
	}
	fmt.Fprintf(out, "[READY] traces-provider  Tempo state converged for %s (%d provider source(s) authorized)\n", prepared.manifest.Name, len(prepared.providerSources))
	return nil
}

func verifyManagedTracesAfterTelemetry(ctx context.Context, out io.Writer, prepared *managedTracesExecution) error {
	if prepared == nil || !prepared.enabled || prepared.execution == nil {
		return nil
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		return err
	}
	if err := refreshProviderTraceSources(prepared); err != nil {
		return err
	}
	verifiedPostgreSQL := false
	verifiedValkey := false
	for _, source := range prepared.providerSources {
		if source.Protocol != "interaction" {
			return fmt.Errorf("provider trace source %s uses unsupported verification protocol %q", source.ID, source.Protocol)
		}
		switch source.Provider {
		case capability.ProviderPostgreSQL:
			if !verifiedPostgreSQL {
				if err := application.VerifyPostgresRuntime(ctx, prepared.runtime, prepared.manifest, prepared.runtimeFiles); err != nil {
					return fmt.Errorf("verify PostgreSQL interaction before trace export: %w", err)
				}
				verifiedPostgreSQL = true
			}
		case capability.ProviderValkey:
			if !verifiedValkey {
				if err := application.VerifyValkeyRuntime(ctx, prepared.runtime, prepared.manifest, prepared.runtimeFiles); err != nil {
					return fmt.Errorf("verify Valkey interaction before trace export: %w", err)
				}
				verifiedValkey = true
			}
		default:
			return fmt.Errorf("provider trace source %s has no interaction verification adapter", source.ID)
		}
		traceID, err := telemetry.ExportProviderInteractionTrace(ctx, prepared.manifest, source)
		if err != nil {
			return err
		}
		if err := tracesprovider.VerifyTrace(ctx, prepared.manifest, traceID); err != nil {
			return fmt.Errorf("verify provider trace %s: %w", source.ID, err)
		}
	}
	fmt.Fprintf(out, "[VERIFIED] traces         application verification trace + %d provider interaction trace(s) queryable for %s\n", len(prepared.providerSources), prepared.manifest.Name)
	return nil
}

func refreshProviderTraceSources(prepared *managedTracesExecution) error {
	if prepared == nil || !prepared.enabled {
		return nil
	}
	policy, err := application.TracesPolicy(prepared.manifest)
	if err != nil {
		return err
	}
	sources, err := observability.ListTraces(
		capability.ProviderPlacement{
			Scope:           prepared.placement.Scope,
			SharingBoundary: prepared.placement.SharingBoundary,
			Ownership:       capability.OwnershipBaseHarbor,
		},
		[]string{prepared.manifest.Name},
		policy.Collect[application.TracesSourceApplicationProvider],
		policy.Collect[application.TracesSourcePlatformProvider],
	)
	if err != nil {
		return fmt.Errorf("resolve provider trace sources: %w", err)
	}
	prepared.providerSources = sources
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
