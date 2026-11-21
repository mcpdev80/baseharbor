package metrics

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestVerifyProviderAndApplicationSourcesSkipsOnlySnapshotMetadata(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")
	m := application.WithWorkloadComponents(application.New("workload-only", "dev", false, false, false), "api")
	issuer := serviceissuer.New(t)
	store := application.Store{Root: filepath.Join(stateDir, "apps")}
	runtimeFiles, err := application.EnsureRuntime(context.Background(), issuer, store, m)
	if err != nil {
		t.Fatalf("EnsureRuntime: %v", err)
	}

	logicalID := "prometheus:" + runtimeFiles.Project
	realSourceID := "prometheus:test-backend-source"
	if err := observability.Update(observability.MetricsSource{
		ID:               realSourceID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           "backend.test:9090",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatalf("register endpoint-backed verifier fixture: %v", err)
	}
	applicationSourceID := observability.ApplicationMetricsSourceID(m.Name, m.Environment, "declared")
	if err := observability.Update(observability.MetricsSource{
		ID:               applicationSourceID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           application.MetricsTargetAliasFor(m.Name, m.Environment, "api") + ":8080",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatalf("register application metrics source: %v", err)
	}
	otherEnvironmentSourceID := observability.ApplicationMetricsSourceID(m.Name, "prod", "declared")
	if err := observability.Update(observability.MetricsSource{
		ID:               otherEnvironmentSourceID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           application.MetricsTargetAliasFor(m.Name, "prod", "api") + ":8080",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatalf("register other-environment application metrics source: %v", err)
	}
	if _, err := EnsureProviderFiles(context.Background(), issuer, m); err != nil {
		t.Fatalf("EnsureProviderFiles: %v", err)
	}

	var queries []string
	err = verifyProviderSourcesAt(context.Background(), m, stateDir, "", func(_ context.Context, _ *http.Client, _ string, query string) (bool, error) {
		queries = append(queries, query)
		return true, nil
	})
	if err != nil {
		t.Fatalf("verify provider sources: %v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("source queries = %d, want one application and one endpoint-backed provider query: %v", len(queries), queries)
	}
	joined := strings.Join(queries, "\n")
	if !strings.Contains(joined, realSourceID) {
		t.Fatalf("queries %q do not cover provider source %q", joined, realSourceID)
	}
	if !strings.Contains(joined, "baseharbor_source=\"declared\"") || !strings.Contains(joined, "job=\"baseharbor-applications\"") {
		t.Fatalf("queries %q do not verify application source %q through the application job", joined, applicationSourceID)
	}
	if strings.Contains(joined, otherEnvironmentSourceID) || strings.Contains(joined, "baseharbor_environment=\"prod\"") {
		t.Fatalf("queries %q include application metrics from another environment %q", joined, otherEnvironmentSourceID)
	}
	if strings.Contains(joined, logicalID) {
		t.Fatalf("queries %q include snapshot-only workload source %q", joined, logicalID)
	}
}
