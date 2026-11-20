package metrics

import (
	"context"
	"net/http"
	"os"
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

// 0428-203 regression: a same-ID application-class source that switches from a
// verifying Verification mode to VerificationNone must no longer carry its
// previously emitted scrape target in the exported surface. Asserted through
// the real exported TargetsDir, not an internal helper. The verifier wait
// semantics (0428-143) are unchanged: this asserts only the exported target set.
func TestVerifyApplicationSourceVerificationNoneRetiresPreviouslyEmitted(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPrometheus), "application")

	m := application.WithWorkloadComponents(application.New("alpha", "dev", false, false, false), "api")
	issuer := serviceissuer.New(t)

	const srcID = "application:metrics:alpha:dev:declared"
	register := func(capability.ObservabilityVerificationMode) {
		if err := observability.Update(observability.MetricsSource{
			ID:               srcID,
			Provider:         capability.ProviderPrometheus,
			Class:            observability.SourceApplication,
			Scope:            capability.ScopeApplication,
			OwnerApplication: m.Name,
			Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
			Target:           application.MetricsTargetAliasFor(m.Name, m.Environment, "api") + ":8080",
			Path:             "/metrics",
			Verification:     capability.ObservabilityVerifyBackend,
		}); err != nil {
			t.Fatalf("register source: %v", err)
		}
	}

	// 1. Emit the target under a verifying mode through the real exporter.
	register(capability.ObservabilityVerifyBackend)
	files, err := EnsureProviderFiles(context.Background(), issuer, m)
	if err != nil {
		t.Fatalf("EnsureProviderFiles (verifying): %v", err)
	}
	emittedName := appTargetFileName(observability.MetricsSource{ID: srcID})
	if _, err := os.Stat(filepath.Join(files.TargetsDir, emittedName)); err != nil {
		t.Fatalf("expected emitted target present in exported surface: %v", err)
	}

	// 2. Switch the SAME id to Verification=none.
	if err := observability.Update(observability.MetricsSource{
		ID:               srcID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           application.MetricsTargetAliasFor(m.Name, m.Environment, "api") + ":8080",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyNone,
	}); err != nil {
		t.Fatalf("switch source to none: %v", err)
	}
	after, err := EnsureProviderFiles(context.Background(), issuer, m)
	if err != nil {
		t.Fatalf("EnsureProviderFiles (none): %v", err)
	}

	// 3. Assert the previously emitted target is gone from the exported surface.
	entries, err := os.ReadDir(after.TargetsDir)
	if err != nil {
		t.Fatalf("read exported surface: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() == emittedName {
			t.Fatalf("stale emitted target %q still present after switch to Verification=none", emittedName)
		}
	}
	// The exporter must not write a new file for the none source either.
	if _, err := os.Stat(filepath.Join(after.TargetsDir, emittedName)); !os.IsNotExist(err) {
		t.Fatalf("none source produced a target file: %v", err)
	}
}

// TestVerifyApplicationSourceVerificationNoneIsSkipped is the regression for
// 0428-143: an application-class source with Verification=none in the manifest's
// own environment must not be awaited as a required "up" sample, matching the
// provider-class skip. The application loop now carries the same
// ObservabilityVerifyNone guard as the provider loop, so the verifier issues no
// query for the none source while still verifying a load-bearing sibling.
func TestVerifyApplicationSourceVerificationNoneIsSkipped(t *testing.T) {
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
	_ = runtimeFiles

	// Load-bearing source in the manifest's own environment (dev): the verifier
	// must still query it, proving the skip is scoped to Verification=none.
	loadBearingID := observability.ApplicationMetricsSourceID(m.Name, m.Environment, "api")
	if err := observability.Update(observability.MetricsSource{
		ID:               loadBearingID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           application.MetricsTargetAliasFor(m.Name, m.Environment, "api") + ":8080",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatalf("register load-bearing application metrics source: %v", err)
	}
	// Verification=none source in the same environment: the verifier must skip
	// it entirely and issue no query.
	noneSourceID := observability.ApplicationMetricsSourceID(m.Name, m.Environment, "exported")
	if err := observability.Update(observability.MetricsSource{
		ID:               noneSourceID,
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourceApplication,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		Network:          application.MetricsProviderNetworkNameForNamespace(m, ""),
		Target:           application.MetricsTargetAliasFor(m.Name, m.Environment, "exported") + ":8080",
		Path:             "/metrics",
		// Verification=none: the verifier must skip this source entirely.
		Verification: capability.ObservabilityVerifyNone,
	}); err != nil {
		t.Fatalf("register verification:none application metrics source: %v", err)
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
	joined := strings.Join(queries, "\n")
	if strings.Contains(joined, `baseharbor_source="exported"`) {
		t.Fatalf("queries %q include verification:none application source 'exported'; the guard must skip it", joined)
	}
	if !strings.Contains(joined, `baseharbor_source="api"`) || !strings.Contains(joined, `job="baseharbor-applications"`) {
		t.Fatalf("queries %q do not verify the load-bearing application source through the applications job", joined)
	}
}
