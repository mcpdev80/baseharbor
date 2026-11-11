package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestVerifyManagedTelemetryBindingRequiresMaterializedEndpoint(t *testing.T) {
	dir := t.TempDir()
	manifest := application.WithOTLPTelemetry(application.Manifest{
		Version: 1, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev",
		Workload: application.WorkloadConfig{Components: []string{"api"}},
	}, "traces")
	files := application.RuntimeFiles{
		Dir: dir, Env: filepath.Join(dir, "runtime.env"),
		ApplicationEnv: filepath.Join(dir, "application.env"), Bindings: filepath.Join(dir, "bindings"),
	}
	for _, path := range []string{files.Env, files.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyManagedTelemetryBinding(manifest, files); err == nil || !strings.Contains(err.Error(), "binding is missing") {
		t.Fatalf("missing OTLP materialization error = %v", err)
	}
	if err := application.MaterializeOTLPBinding(manifest, files, capability.ProviderOTelCollector, "http://127.0.0.1:4318", "http://otel-collector:4318"); err != nil {
		t.Fatal(err)
	}
	if err := verifyManagedTelemetryBinding(manifest, files); err != nil {
		t.Fatalf("materialized OTLP binding rejected: %v", err)
	}
}

func TestRequireManagedTelemetryPreparedWhenRequested(t *testing.T) {
	manifest := application.WithOTLPTelemetry(application.Manifest{
		Version: 1, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev",
	}, "traces")
	if err := requireManagedTelemetryPrepared(manifest, nil); err == nil {
		t.Fatal("requested telemetry must not silently skip provider preparation")
	}
	if err := requireManagedTelemetryPrepared(manifest, &managedTelemetryExecution{}); err != nil {
		t.Fatalf("prepared telemetry rejected: %v", err)
	}
	manifest.Telemetry.OTLP = nil
	if err := requireManagedTelemetryPrepared(manifest, nil); err != nil {
		t.Fatalf("application without telemetry rejected: %v", err)
	}
}
