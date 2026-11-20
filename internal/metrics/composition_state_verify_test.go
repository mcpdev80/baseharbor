package metrics

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// TestVerifyNoneCompositionSurfacesNullTargetState is the RED regression for
// hunt-918 befund 6 over the REAL operator-facing verify seam
// (verifyProviderSourcesAt). Every provider source is Verification=none, so the
// composition has zero effective scrape targets. The verify path must surface
// that null-target reason instead of returning silent nil.
//
// This test is RED at the base: verifyProviderSourcesAt returns nil (no state),
// so the assertion below fails. It is GREEN once the state is wired into the
// operator-facing composition path.
func TestVerifyNoneCompositionSurfacesNullTargetState(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	// A shared platform-provider verification:none source is registered so that
	// observability.ListMetrics returns it at verify time.
	m := application.New("platform-tools", "dev", false, false, false)
	iss := serviceissuer.New(t)
	if err := observability.Update(observability.MetricsSource{
		ID:               "prometheus:platform-provider",
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourcePlatformProvider,
		Scope:            capability.ScopeShared,
		OwnerApplication: m.Name,
		Network:          "baseharbor-platform-dev-metrics",
		Target:           "platform.backend:9090",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyNone,
	}); err != nil {
		t.Fatalf("register verification:none platform-provider source: %v", err)
	}
	if _, err := EnsureProviderFiles(context.Background(), iss, m); err != nil {
		t.Fatalf("EnsureProviderFiles: %v", err)
	}

	var queries []string
	err := verifyProviderSourcesAt(context.Background(), m, stateDir, "", func(_ context.Context, _ *http.Client, _ string, query string) (bool, error) {
		queries = append(queries, query)
		return true, nil
	})

	// (a) RED at base: the null-target reason must be surfaced as a state error,
	// never silent nil.
	if err == nil {
		t.Fatalf("verifyProviderSourcesAt returned nil for an all-verification:none composition; the null-target reason must be surfaced as a composition state")
	}
	var stateErr *CompositionStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("verifyProviderSourcesAt error %v is not a *CompositionStateError carrying the composition state", err)
	}
	for _, want := range []string{"null effective scrape targets", "after composition"} {
		if !strings.Contains(stateErr.State(), want) {
			t.Fatalf("composition state %q does not contain %q", stateErr.State(), want)
		}
	}
	// The verification:none source must NOT have produced a scrape query: the
	// correct verification:none skip is preserved.
	for _, q := range queries {
		if strings.Contains(q, "prometheus:platform-provider") {
			t.Fatalf("verification:none source must not be scraped; got query %q", q)
		}
	}
}

// TestVerifyPositiveTargetCompositionReportsOneTarget is the GREEN positive
// case: a single backend-backed provider source still has one effective scrape
// target and verifies with no state error.
func TestVerifyPositiveTargetCompositionReportsOneTarget(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	m := application.New("platform-tools", "dev", false, false, false)
	iss := serviceissuer.New(t)
	if err := observability.Update(observability.MetricsSource{
		ID:               "prometheus:platform-provider",
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourcePlatformProvider,
		Scope:            capability.ScopeShared,
		OwnerApplication: m.Name,
		Network:          "baseharbor-platform-dev-metrics",
		Target:           "platform.backend:9090",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatalf("register backend-backed platform-provider source: %v", err)
	}
	if _, err := EnsureProviderFiles(context.Background(), iss, m); err != nil {
		t.Fatalf("EnsureProviderFiles: %v", err)
	}

	var queries []string
	err := verifyProviderSourcesAt(context.Background(), m, stateDir, "", func(_ context.Context, _ *http.Client, _ string, query string) (bool, error) {
		queries = append(queries, query)
		return true, nil
	})
	if err != nil {
		t.Fatalf("verifyProviderSourcesAt: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("expected exactly one backend-backed provider query, got %v", queries)
	}
}

// TestVerifyUnchangedNoCompositionDoesNotSurfaceState is the GREEN unchanged
// case: an app with no provider and no application sources still verifies with
// silent nil (nothing to report). This must not regress.
func TestVerifyUnchangedNoCompositionDoesNotSurfaceState(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	m := application.New("no-metrics", "dev", false, false, false)

	err := verifyProviderSourcesAt(context.Background(), m, stateDir, "", func(_ context.Context, _ *http.Client, _ string, query string) (bool, error) {
		t.Fatal("no query must be issued when there is nothing to verify")
		return false, nil
	})
	if err != nil {
		t.Fatalf("verifyProviderSourcesAt with no composition must return nil, got %v", err)
	}
}
