package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
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
	dataDir         string
	namespace       string
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
		dataDir:      resolved.TargetStateRoot,
		namespace:    resolved.Target.Name,
	}
	if !enabled {
		return prepared, nil
	}
	placement, err := tracesprovider.PlacementForAt(resolved.TargetStateRoot, resolved.Target.Name, m)
	if err != nil {
		return nil, err
	}
	prepared.placement = placement
	driver := tracesprovider.NewDriverAt(compose, m, issuer, resolved.TargetStateRoot, resolved.Target.Name)
	resource := capability.Resource{
		Application: m.Name,
		Kind:        capability.Traces,
		Name:        "default",
		Provider:    capability.ProviderTempo,
	}
	if err := application.CheckAdditionalProviderResourcesAt(resolved.TargetStateRoot, m, []capability.Resource{resource}); err != nil {
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
	if err := application.VerifyManagedProviderInteractions(ctx, prepared.runtime, prepared.manifest, prepared.runtimeFiles, prepared.providerSources); err != nil {
		return err
	}
	for _, source := range prepared.providerSources {
		if source.Verification != capability.ObservabilityVerifySpan {
			return fmt.Errorf("provider trace source %s has unsupported verification %q", source.ID, source.Verification)
		}
		var traceID string
		var err error
		switch {
		case source.Mode == capability.ObservabilityInteraction && source.Protocol == "interaction":
			traceID, err = telemetry.ExportProviderInteractionTrace(ctx, prepared.manifest, source)
		case source.Mode == capability.ObservabilityNative && source.Protocol == "otlp":
			traceID, err = runtimeComponentTraceProbe(ctx, prepared, source)
		default:
			return fmt.Errorf("provider trace source %s has unsupported realization %q/%q", source.ID, source.Mode, source.Protocol)
		}
		if err != nil {
			return err
		}
		if err := tracesprovider.VerifyTraceAt(ctx, prepared.manifest, traceID, prepared.dataDir, prepared.namespace); err != nil {
			return fmt.Errorf("verify provider trace %s: %w", source.ID, err)
		}
	}
	fmt.Fprintf(out, "[VERIFIED] traces         application verification trace + %d provider trace(s) queryable for %s\n", len(prepared.providerSources), prepared.manifest.Name)
	return nil
}

func runtimeComponentTraceProbe(ctx context.Context, prepared *managedTracesExecution, source observability.SignalSource) (string, error) {
	switch source.Provider {
	case capability.ProviderRuntimeBroker:
		brokerFiles, err := runtimebroker.Existing(prepared.runtimeFiles)
		if err != nil {
			return "", fmt.Errorf("load runtime broker for trace verification: %w", err)
		}
		raw, err := prepared.runtime.ExecProject(
			ctx,
			runtimebroker.ProjectName(prepared.manifest),
			brokerFiles.Compose,
			prepared.runtimeFiles.Env,
			runtimebroker.ServiceName,
			"curl", "--include", "--silent", "--show-error",
			"--resolve", "baseharbor-runtime:8443:127.0.0.1",
			"--cacert", "/run/baseharbor/identity/ca.pem",
			"--cert", "/run/secrets/probe-client-cert",
			"--key", "/run/secrets/probe-client-key",
			runtimebroker.RuntimeURL+"/runtime/__baseharbor_observability_probe",
		)
		if err != nil {
			return "", fmt.Errorf("trigger runtime broker trace: %w", err)
		}
		return traceIDFromHTTPResponse(raw)
	case capability.ProviderRuntimeExecutor:
		executorFiles, err := runtimeexecutor.ExistingFiles(prepared.dataDir)
		if err != nil {
			return "", fmt.Errorf("load runtime executor for trace verification: %w", err)
		}
		raw, err := prepared.runtime.ExecProject(
			ctx,
			runtimeexecutor.ProjectName,
			executorFiles.Compose,
			executorFiles.Env,
			runtimeexecutor.ServiceName,
			"curl", "--include", "--silent", "--show-error",
			"--resolve", "baseharbor-runtime-executor:9443:127.0.0.1",
			"--cacert", "/run/baseharbor/identity/ca.pem",
			"--cert", "/run/baseharbor/observability/client-cert.pem",
			"--key", "/run/secrets/observer-client-key",
			runtimeexecutor.ExecutorURL+"/internal/__baseharbor_observability_probe",
		)
		if err != nil {
			return "", fmt.Errorf("trigger runtime executor trace: %w", err)
		}
		return traceIDFromHTTPResponse(raw)
	default:
		return "", fmt.Errorf("native OTLP trace source %s has no runtime probe adapter", source.ID)
	}
}

func traceIDFromHTTPResponse(raw string) (string, error) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "traceparent") {
			continue
		}
		parts := strings.Split(strings.TrimSpace(value), "-")
		if len(parts) != 4 || len(parts[1]) != 32 || len(parts[2]) != 16 {
			return "", errors.New("runtime traceparent response is invalid")
		}
		if parts[1] == strings.Repeat("0", 32) {
			return "", errors.New("runtime traceparent contains an invalid zero trace id")
		}
		return strings.ToLower(parts[1]), nil
	}
	return "", errors.New("runtime traceparent response header is missing")
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
