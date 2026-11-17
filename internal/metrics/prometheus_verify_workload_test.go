package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// TestVerifyProviderSourcesWorkloadOnlyPasses asserts a workload-only app (no
// declared metrics sources, no runtime metrics permission) registers no provider
// source in the shared placement. Verification must pass (nothing to verify),
// never fail closed on the empty source set. This is the round-1 gap fix.
func TestVerifyProviderSourcesWorkloadOnlyPasses(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := application.New("workload-only", "dev", false, false, false)

	if err := VerifyProviderSourcesAt(context.Background(), m, dir, ""); err != nil {
		t.Fatalf("workload-only app must verify with zero sources: %v", err)
	}
}

// TestVerifyProviderSourcesWorkloadOnlyWithRuntimePermissionPasses asserts the
// same pass for a workload-only app that requested the runtime metrics
// capability: it still declares no Metrics.Sources and materializes no provider
// source, so verification passes rather than failing closed.
func TestVerifyProviderSourcesWorkloadOnlyWithRuntimePermissionPasses(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := application.New("workload-only-rp", "dev", false, false, false)
	m.Runtime.Permissions = []application.RuntimePermission{
		{Capability: string(capability.MetricsV1.ID), Services: []string{"api"}, Operations: []string{"runtime.get"}},
	}
	if !application.HasRuntimeMetricsPermissions(m) {
		t.Fatal("fixture: app must expose runtime metrics permission")
	}

	if err := VerifyProviderSourcesAt(context.Background(), m, dir, ""); err != nil {
		t.Fatalf("workload-only app with runtime permission must verify with zero sources: %v", err)
	}
}

// TestVerifyProviderSourcesEmptyRegistrySharedPlacement confirms that a
// materialized shared provider backed by an empty (valid) signals registry does
// not itself error and that the workload-only early return is exercised.
func TestVerifyProviderSourcesEmptyRegistrySharedPlacement(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dir)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := application.New("workload-only-registry", "dev", false, false, false)

	regPath := filepath.Join(dir, "observability", "signals.json")
	if err := os.MkdirAll(filepath.Dir(regPath), 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent([]observability.SignalSource{}, "", "  ")
	if err := os.WriteFile(regPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := VerifyProviderSourcesAt(context.Background(), m, dir, ""); err != nil {
		t.Fatalf("workload-only app with empty registry must verify: %v", err)
	}
}
