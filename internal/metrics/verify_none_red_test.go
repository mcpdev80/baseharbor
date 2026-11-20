package metrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// TestRedAllVerificationNoneHasNoCompositionCounter is the RED regression for
// hunt-918 befund 6: the composition with a single verification:none source
// currently yields zero effective scrape-targets with no explicit counter.
//
// The GREEN replacement (TestVerificationNoneCompositionReportsNullTargetState)
// asserts the documented counter: ProviderCompositionState names the null
// effective target count and its reason, and the composition carries it.
func TestRedAllVerificationNoneHasNoCompositionCounter(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	noneSource := observability.MetricsSource{
		ID:               "prometheus:platform-provider",
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourcePlatformProvider,
		Scope:            capability.ScopeShared,
		OwnerApplication: "platform-tools",
		Network:          "baseharbor-platform-dev-metrics",
		Target:           "platform.backend:9090",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyNone,
	}

	targetsDir := filepath.Join(stateDir, "targets")
	if err := os.MkdirAll(targetsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// syncProviderTargets skips the verification:none source -> no target file.
	if err := syncProviderTargets(targetsDir, []observability.MetricsSource{noneSource}); err != nil {
		t.Fatalf("syncProviderTargets: %v", err)
	}

	entries, err := os.ReadDir(targetsDir)
	if err != nil {
		t.Fatal(err)
	}
	var written int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		written++
	}
	if written != 0 {
		t.Fatalf("expected zero target files for verification:none, got %d", written)
	}

	networks := providerMetricNetworks([]observability.MetricsSource{noneSource})
	if len(networks) != 0 {
		t.Fatalf("expected zero networks for verification:none, got %v", networks)
	}
}

// TestVerificationNoneCompositionReportsNullTargetState is the GREEN assertion:
// the composition counter names the null effective target count and its reason,
// and the correct verification:none skips are preserved (no target, no network).
func TestVerificationNoneCompositionReportsNullTargetState(t *testing.T) {
	noneSource := observability.MetricsSource{
		ID:           "prometheus:platform-provider",
		Provider:     capability.ProviderPrometheus,
		Class:        observability.SourcePlatformProvider,
		Scope:        capability.ScopeShared,
		Network:      "baseharbor-platform-dev-metrics",
		Target:       "platform.backend:9090",
		Path:         "/metrics",
		Verification: capability.ObservabilityVerifyNone,
	}
	if got := EffectiveProviderTargetCount([]observability.MetricsSource{noneSource}); got != 0 {
		t.Fatalf("EffectiveProviderTargetCount = %d, want 0 for verification:none", got)
	}
	state := ProviderCompositionState([]observability.MetricsSource{noneSource})
	for _, want := range []string{"null effective scrape targets", "after composition"} {
		if !strings.Contains(state, want) {
			t.Fatalf("composition state %q does not contain %q", state, want)
		}
	}

	// A backend-backed source still counts as an effective target.
	backendSource := observability.MetricsSource{
		ID:           "prometheus:backend",
		Provider:     capability.ProviderPrometheus,
		Class:        observability.SourcePlatformProvider,
		Scope:        capability.ScopeShared,
		Network:      "baseharbor-backend-dev-metrics",
		Target:       "backend:9090",
		Path:         "/metrics",
		Verification: capability.ObservabilityVerifyBackend,
	}
	if got := EffectiveProviderTargetCount([]observability.MetricsSource{backendSource}); got != 1 {
		t.Fatalf("EffectiveProviderTargetCount = %d, want 1 for backend-backed source", got)
	}
	if got := ProviderCompositionState([]observability.MetricsSource{backendSource}); !strings.Contains(got, "1 effective scrape target") {
		t.Fatalf("composition state %q does not name the one effective target", got)
	}
}

// TestEnsureProviderFilesCompositionStateCarriesState runs the real composition
// entry point with a workload-only app (no provider metrics sources, only
// runtime workload metrics) and asserts ProviderFiles.CompositionState carries
// the explicit "no provider metrics sources" state. This is finding 1: an app
// with no provider sources must NOT be reported as "null effective scrape
// targets after composition (all Verification=none)" — that would be a false
// null-provider-target status.
func TestEnsureProviderFilesCompositionStateCarriesState(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	m := application.WithWorkloadComponents(application.New("workload-only", "dev", false, false, false), "api")
	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), m)
	if err != nil {
		t.Fatalf("EnsureProviderFiles: %v", err)
	}
	if files.CompositionState == "" {
		t.Fatal("ProviderFiles.CompositionState is empty; composition must report its state")
	}
	// The workload-only app has no provider metrics sources: the composition
	// must report the no-source state, not the Verification=none null-target
	// message (finding 1).
	for _, ok := range []string{"no provider metrics sources", "nothing to scrape"} {
		if !strings.Contains(files.CompositionState, ok) {
			t.Fatalf("composition state %q for a provider-less workload-only app must report the no-source state, got %q", ok, files.CompositionState)
		}
	}
}

// TestProviderCompositionStateEmptySourceListIsNoSource (finding 1): the empty
// provider source list must report the no-source state, NOT the "all
// Verification=none null effective scrape targets" message. A zero effective
// count with no sources is "no sources", not "all verification:none".
func TestProviderCompositionStateEmptySourceListIsNoSource(t *testing.T) {
	state := ProviderCompositionState(nil)
	for _, ok := range []string{"no provider metrics sources", "nothing to scrape"} {
		if !strings.Contains(state, ok) {
			t.Fatalf("composition state for empty source list %q must report the no-source state, got %q", ok, state)
		}
	}
	for _, bad := range []string{"Verification=none"} {
		if strings.Contains(state, bad) {
			t.Fatalf("composition state for empty source list %q must not claim all sources are Verification=none", state)
		}
	}
}

// TestProviderCompositionStateNonEmptyAllNoneReportsNullTarget (finding 1): a
// non-empty provider source list whose sources are all Verification=none must
// still report the null effective target count (the documented, expected-zero
// state). The empty-guard above must not swallow the genuine all-verification
// none case.
func TestProviderCompositionStateNonEmptyAllNoneReportsNullTarget(t *testing.T) {
	noneSource := observability.MetricsSource{
		ID:           "prometheus:platform-provider",
		Provider:     capability.ProviderPrometheus,
		Class:        observability.SourcePlatformProvider,
		Scope:        capability.ScopeShared,
		Target:       "platform.backend:9090",
		Path:         "/metrics",
		Verification: capability.ObservabilityVerifyNone,
	}
	state := ProviderCompositionState([]observability.MetricsSource{noneSource})
	for _, want := range []string{"null effective scrape targets", "after composition", "Verification=none"} {
		if !strings.Contains(state, want) {
			t.Fatalf("composition state %q for all-verification:none must report the null effective target count, got %q", want, state)
		}
	}
}
