package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type managedProviderPreflightState struct {
	exposure      *managedExposureExecution
	objectStorage *managedObjectStorageExecution
	traces        *managedTracesExecution
	telemetry     *managedTelemetryExecution
	metrics       *managedMetricsExecution
	logs          *managedLogsExecution
}

func appendManagedProviderPreflights(
	checks []preflight.Check,
	compose *bhruntime.Compose,
	resolved resolvedApplication,
	state *managedProviderPreflightState,
	issuer *serviceaccess.Issuer,
) []preflight.Check {
	m := resolved.Manifest

	if application.HasObjectStorage(m) || requiresRuntimeObjectStorageExecutor(m) {
		checks = append(checks, preflight.Check{Name: "managed object storage provider", Run: func(ctx context.Context) error {
			var err error
			state.objectStorage, err = prepareManagedObjectStorage(ctx, *compose, resolved, *issuer)
			return err
		}})
	}
	if application.HasTraceSignal(m) {
		checks = append(checks, preflight.Check{Name: "managed traces provider", Run: func(ctx context.Context) error {
			var err error
			state.traces, err = prepareManagedTraces(ctx, *compose, resolved, *issuer)
			return err
		}})
	}
	if application.HasOTLPTelemetry(m) {
		checks = append(checks, preflight.Check{Name: "managed telemetry provider", Run: func(ctx context.Context) error {
			var err error
			state.telemetry, err = prepareManagedTelemetry(ctx, *compose, resolved, state.traces, *issuer)
			return err
		}})
	}
	if application.HasMetricsSources(m) || application.HasRuntimeMetricsPermissions(m) {
		checks = append(checks, preflight.Check{Name: "managed metrics provider", Run: func(ctx context.Context) error {
			var err error
			state.metrics, err = prepareManagedMetrics(ctx, *compose, resolved, *issuer)
			return err
		}})
	}
	if application.HasLogsCollection(m) {
		checks = append(checks, preflight.Check{Name: "managed logs provider", Run: func(ctx context.Context) error {
			var err error
			state.logs, err = prepareManagedLogs(ctx, *compose, resolved, *issuer)
			return err
		}})
	}
	if len(m.Exposures) > 0 {
		checks = append(checks, preflight.Check{Name: "managed exposure provider", Run: func(ctx context.Context) error {
			var err error
			state.exposure, err = prepareManagedExposure(ctx, *compose, resolved)
			return err
		}})
	}
	return checks
}

// prepareUndeclaredProviderCleanup discovers stale provider state that must be
// removed after a capability was deleted from the manifest. It performs no
// mutation and deliberately does not create visible capability preflight rows.
func prepareUndeclaredProviderCleanup(
	ctx context.Context,
	compose bhruntime.Compose,
	resolved resolvedApplication,
	state *managedProviderPreflightState,
	issuer serviceaccess.Issuer,
) error {
	m := resolved.Manifest
	if !application.HasMetricsSources(m) && !application.HasRuntimeMetricsPermissions(m) {
		prepared, err := prepareManagedMetrics(ctx, compose, resolved, issuer)
		if err != nil {
			return err
		}
		state.metrics = prepared
	}
	if !application.HasLogsCollection(m) {
		prepared, err := prepareManagedLogs(ctx, compose, resolved, issuer)
		if err != nil {
			return err
		}
		state.logs = prepared
	}
	return nil
}

func hasProviderCapabilityIntent(m application.Manifest, kind capability.Kind) bool {
	switch kind {
	case capability.ObjectStorageS3:
		return application.HasObjectStorage(m) || requiresRuntimeObjectStorageExecutor(m)
	case capability.Traces:
		return application.HasTraceSignal(m)
	case capability.TelemetryOTLP:
		return application.HasOTLPTelemetry(m)
	case capability.Metrics:
		return application.HasMetricsSources(m) || application.HasRuntimeMetricsPermissions(m)
	case capability.Logs:
		return application.HasLogsCollection(m)
	case capability.ExposureHTTP:
		return len(m.Exposures) > 0
	default:
		return false
	}
}
