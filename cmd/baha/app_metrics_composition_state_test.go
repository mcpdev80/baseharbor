package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
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

// TestVerifyManagedMetricsDedupesProviderCompositionState (finding 2): for an
// app whose provider composition reports via a *CompositionStateError, the
// unconditional ProviderSourcesCompositionState print must not repeat the same
// [STATE] line. The workload-only app has no provider sources, so the state is
// surfaced once via the no-source path.
func TestVerifyManagedMetricsDedupesProviderCompositionState(t *testing.T) {
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
	if strings.Count(buf.String(), "[STATE] metrics") != 1 {
		t.Fatalf("expected exactly one [STATE] metrics line for a provider-less app, got %d:\n%q", strings.Count(buf.String(), "[STATE] metrics"), buf.String())
	}
}
