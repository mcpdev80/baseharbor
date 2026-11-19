package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestVerifyManagedMetricsSurfacesNullTargetState exercises the operator-facing
// verifyManagedMetricsAfterWorkload seam for a workload-only app whose only
// provider source is Verification=none. The operator must see the explicit
// null-target composition state, never silent convergence.
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
	if !strings.Contains(out, "null effective scrape targets") {
		t.Fatalf("composition state must name the null effective target reason, got: %q", out)
	}
}
