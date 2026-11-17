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

func TestVerifyProviderSourcesSkipsOnlySnapshotMetadata(t *testing.T) {
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
	if len(queries) != 1 {
		t.Fatalf("provider queries = %d, want exactly the real endpoint-backed source query: %v", len(queries), queries)
	}
	if !strings.Contains(queries[0], realSourceID) {
		t.Fatalf("query %q does not cover real source %q", queries[0], realSourceID)
	}
	if strings.Contains(queries[0], logicalID) {
		t.Fatalf("query %q includes snapshot-only workload source %q", queries[0], logicalID)
	}
}
