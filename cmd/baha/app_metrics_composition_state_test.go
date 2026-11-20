package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

// TestVerifyManagedMetricsSurfacesNullTargetState exercises the operator-facing
// verifyManagedMetricsAfterWorkload seam for a workload-only app. The app has
// no provider metrics sources, so the operator must see the explicit
// no-source composition state (finding 1), never a false null-provider-target,
// and never silent convergence.
func TestVerifyManagedMetricsSurfacesNullTargetState(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	m := application.WithWorkloadComponents(application.New("workload-only", "dev", false, false, false), "api")

	prepared := &managedMetricsExecution{
		enabled:          true,
		manifest:         m,
		dataDir:          stateDir,
		namespace:        "",
		execution:        nil,
		placementChanged: false,
	}

	var buf bytes.Buffer
	if err := verifyManagedMetricsAfterWorkload(context.Background(), &buf, prepared); err != nil {
		t.Fatalf("verifyManagedMetricsAfterWorkload: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[STATE] metrics") {
		t.Fatalf("operator-facing verify must surface the composition state as a [STATE] line, got: %q", out)
	}
	// Finding 1: a provider-less workload-only app reports the no-source state,
	// not the Verification=none null-target message.
	if !strings.Contains(out, "no provider metrics sources") {
		t.Fatalf("composition state must name the no-source reason for a provider-less app, got: %q", out)
	}
	if strings.Contains(out, "Verification=none") {
		t.Fatalf("composition state for a provider-less app must not claim all sources are Verification=none, got: %q", out)
	}
	// Finding 2: exactly one [STATE] line is emitted, never the same state twice.
	if strings.Count(out, "[STATE] metrics") != 1 {
		t.Fatalf("expected exactly one [STATE] metrics line, got %d:\n%q", strings.Count(out, "[STATE] metrics"), out)
	}
}

// r4SharedProviderSeam lays out the shared provider state directory that the
// CLI seam's composition paths consult (ExistingProviderFilesAt via
// PlacementForAt) and registers one REAL shared platform-provider metrics
// source whose Verification mode is "none". This is the fixture the r3 verdict
// demands: the earlier workload-only test never fulfilled the branch
// condition, so counting [STATE] lines proved only that nothing was to be
// said. Here both reasons are live at once — the provider path raises a
// *metricsprovider.CompositionStateError and the display path
// (metricsprovider.ProviderSourcesCompositionState) would repeat the same
// state — and the dedupe gate is what keeps them to one line.
func r4SharedProviderSeam(t *testing.T, app, env string) string {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	t.Setenv(application.MetricsEnabledEnv, "true")

	dir := filepath.Join(filepath.Clean(stateDir), "providers", "prometheus", "shared")
	if err := os.MkdirAll(filepath.Join(dir, "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	// ExistingProviderFilesAt requires compose/env/config state files and the
	// targets directory to exist.
	for _, name := range []string{"compose.yaml", "runtime.env", "prometheus.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# r4 fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The shared registrations file drives the allowed-application set that
	// BOTH composition paths (the provider path and the [STATE] path) filter
	// by, so the display sees the same source set.
	registrations := `[{"application":"` + app + `","environment":"` + env + `"}]`
	if err := os.WriteFile(filepath.Join(dir, "registrations.json"), []byte(registrations), 0o600); err != nil {
		t.Fatal(err)
	}

	// The registry itself lives beside the runtime/ dir the CLI resolves from
	// BASEHARBOR_STATE_DIR (observability registryPath: DataDir/observability).
	if err := observability.Update(observability.MetricsSource{
		ID:               "prometheus:r4-cli-shared-platform",
		Provider:         capability.ProviderPrometheus,
		Class:            observability.SourcePlatformProvider,
		Scope:            capability.ScopeShared,
		OwnerApplication: app,
		Network:          "baseharbor-r4-cli-shared-metrics",
		Target:           "platform." + app + ".backend:9090",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyNone,
	}); err != nil {
		t.Fatalf("register shared verification:none source: %v", err)
	}
	return stateDir
}

// TestVerifyManagedMetricsDedupesProviderCompositionState (finding 2, r4): for
// an app whose provider composition reports through a *CompositionStateError,
// the unconditional ProviderSourcesCompositionState print must not repeat the
// same [STATE] line. The fixture registers one real shared provider metrics
// source with Verification=none so the provider path and the [STATE] path
// BOTH have the composition state as their reason; the assertion then counts
// exactly one [STATE] metrics line AND requires it to name the right reason.
func TestVerifyManagedMetricsDedupesProviderCompositionState(t *testing.T) {
	stateDir := r4SharedProviderSeam(t, "shared-app", "dev")

	m := application.WithWorkloadComponents(application.New("shared-app", "dev", false, false, false), "api")
	prepared := &managedMetricsExecution{
		enabled:          true,
		manifest:         m,
		dataDir:          stateDir,
		namespace:        "",
		execution:        nil,
		placementChanged: false,
	}

	var buf bytes.Buffer
	if err := verifyManagedMetricsAfterWorkload(context.Background(), &buf, prepared); err != nil {
		t.Fatalf("verifyManagedMetricsAfterWorkload: %v", err)
	}
	out := buf.String()
	if got := strings.Count(out, "[STATE] metrics"); got != 1 {
		t.Fatalf("expected exactly one [STATE] metrics line for an all-verification:none shared composition, got %d:\n%q", got, out)
	}
	// The one line must carry the null-target reason (from the provider path),
	// not the redundant display print and not another state.
	for _, want := range []string{"null effective scrape targets", "Verification=none"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the single [STATE] line must name the right reason (%q), got: %q", want, out)
		}
	}
}

// TestVerifyManagedMetricsProviderCompositionStateReasonIsUniqueLine pins the
// same seam from the line side: exactly one [STATE] metrics line, and that
// line is the composition state the provider path reported. This is the
// discriminating assertion for the dedupe gate in app_metrics.go
// (compositionErr == nil): commenting the gate out makes the display print the
// identical state a second time and this count fails (the r4 mutation proof).
func TestVerifyManagedMetricsProviderCompositionStateReasonIsUniqueLine(t *testing.T) {
	stateDir := r4SharedProviderSeam(t, "dedupe-app", "dev")

	m := application.WithWorkloadComponents(application.New("dedupe-app", "dev", false, false, false), "api")
	prepared := &managedMetricsExecution{
		enabled:          true,
		manifest:         m,
		dataDir:          stateDir,
		namespace:        "",
		execution:        nil,
		placementChanged: false,
	}

	var buf bytes.Buffer
	if err := verifyManagedMetricsAfterWorkload(context.Background(), &buf, prepared); err != nil {
		t.Fatalf("verifyManagedMetricsAfterWorkload: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "[STATE] metrics") {
			lines = append(lines, line)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("expected exactly one [STATE] metrics line, got %d:\n%q", len(lines), buf.String())
	}
	for _, want := range []string{"null effective scrape targets", "Verification=none"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("the [STATE] line must name the right reason (%q), got %q", want, lines[0])
		}
	}
}

// TestVerifyManagedMetricsCompositionErrorBranchIsReached proves the r3
// finding: the fixture actually drives the composition path into its
// CompositionStateError branch. The same state, consumed directly through
// metricsprovider.VerifyProviderSourcesAt (the seam the CLI calls), must fail
// with a *metricsprovider.CompositionStateError. Without this, a [STATE] line
// count of one could still be a test that never fulfilled the condition (the
// r3 blind spot the verdict named).
func TestVerifyManagedMetricsCompositionErrorBranchIsReached(t *testing.T) {
	stateDir := r4SharedProviderSeam(t, "branch-app", "dev")
	m := application.WithWorkloadComponents(application.New("branch-app", "dev", false, false, false), "api")

	err := metricsprovider.VerifyProviderSourcesAt(context.Background(), m, stateDir, "")
	var stateErr *metricsprovider.CompositionStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("the fixture must drive verifyProviderSourcesAt into its CompositionStateError branch, got err=%v", err)
	}
	for _, want := range []string{"null effective scrape targets", "Verification=none"} {
		if !strings.Contains(stateErr.State(), want) {
			t.Fatalf("the state error must carry the null-target reason (%q), got %q", want, stateErr.State())
		}
	}
}
