package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestProviderCapabilityIntentIsAbsentForPlainWorkload(t *testing.T) {
	m := application.New("demo", "dev", false, false, false)
	m.Services.Postgres = false
	m = application.WithWorkload(m, "compose.yaml", "api")

	for _, kind := range []capability.Kind{
		capability.ObjectStorageS3,
		capability.Traces,
		capability.TelemetryOTLP,
		capability.Metrics,
		capability.Logs,
		capability.ExposureHTTP,
	} {
		if hasProviderCapabilityIntent(m, kind) {
			t.Fatalf("plain workload unexpectedly has %s provider intent", kind)
		}
	}
}

func TestProviderCapabilityIntentMatchesManifest(t *testing.T) {
	base := application.New("demo", "dev", false, false, false)
	base.Services.Postgres = false
	base = application.WithWorkload(base, "compose.yaml", "api")

	logs := application.WithLogsCollection(base, "application")
	if !hasProviderCapabilityIntent(logs, capability.Logs) {
		t.Fatal("declared logs intent was not detected")
	}

	metrics := application.WithMetricsSource(base, "application", "api", 8080, "/metrics")
	if !hasProviderCapabilityIntent(metrics, capability.Metrics) {
		t.Fatal("declared metrics intent was not detected")
	}

	telemetry := application.WithOTLPTelemetry(base, "metrics")
	if !hasProviderCapabilityIntent(telemetry, capability.TelemetryOTLP) {
		t.Fatal("declared telemetry intent was not detected")
	}
	if hasProviderCapabilityIntent(telemetry, capability.Traces) {
		t.Fatal("OTLP without traces signal unexpectedly enabled traces provider intent")
	}

	traces := application.WithOTLPTelemetry(base, "traces")
	if !hasProviderCapabilityIntent(traces, capability.Traces) {
		t.Fatal("declared traces signal was not detected")
	}

	exposure := application.WithHTTPExposure(base, "public", "api", 8080, "http")
	if !hasProviderCapabilityIntent(exposure, capability.ExposureHTTP) {
		t.Fatal("declared exposure intent was not detected")
	}

	storage := application.WithObjectStorageBuckets(base, "assets")
	if !hasProviderCapabilityIntent(storage, capability.ObjectStorageS3) {
		t.Fatal("declared object storage intent was not detected")
	}
}
