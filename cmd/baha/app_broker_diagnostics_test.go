package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/runtime"
)

// brokerDiagnosticsFixture answers the readiness-failure diagnostic channels:
// the broker container log tail and one direct /readyz probe body.
type brokerDiagnosticsFixture struct {
	runtime.RuntimeProvider
	logs    string
	readyz  string
	execErr error
}

func (f *brokerDiagnosticsFixture) LogsProject(context.Context, string, string, string, ...string) (string, error) {
	return f.logs, nil
}

func (f *brokerDiagnosticsFixture) ExecProject(_ context.Context, _, _, _, _ string, args ...string) (string, error) {
	if f.execErr != nil {
		return "", f.execErr
	}
	if len(args) > 0 && strings.Contains(args[len(args)-1], "/readyz") {
		return f.readyz, nil
	}
	return "", errors.New("unexpected exec in diagnostics fixture")
}

func TestRuntimeBrokerReadinessFailureCarriesDependencyDiagnostics(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	// The readiness loop's last error was an unhealthy container (the measured
	// 0428-240 shape); the failure branch must fill diagnostics from BOTH
	// channels: the container log tail and the last /readyz body.
	f := &brokerDiagnosticsFixture{
		logs:   "runtime broker readiness failed: dependency=runtime-executor error=connection refused\n",
		readyz: `{"status":"not_ready","dependency":"runtime-executor"}`,
	}
	err := failRuntimeBrokerReadiness(ctx, f, application.New("broker", "dev", false, false, true), application.RuntimeFiles{Compose: "/tmp/app.compose.yaml"}, errors.New("application runtime broker container health is unhealthy"))
	if err == nil {
		t.Fatal("expected the bounded readiness attempt to fail")
	}
	if !strings.Contains(err.Error(), "dependency=runtime-executor") {
		t.Fatalf("readiness failure did not name the dependency in its diagnostics: %v", err)
	}
	if !strings.Contains(err.Error(), "broker container log tail:") || !strings.Contains(err.Error(), "broker /readyz (last):") {
		t.Fatalf("diagnostics channels missing from the failure text: %v", err)
	}
}

// TestRuntimeBrokerReadinessDiagnosticsNameTheDependency asserts the narrow
// rule: when /readyz reports a named dependency, the diagnostics text carries
// that dependency name. An unhealthy container alone used to leave the
// diagnostics block empty (0428-240).
func TestRuntimeBrokerReadinessDiagnosticsNameTheDependency(t *testing.T) {
	f := &brokerDiagnosticsFixture{
		logs:   "2026-11-22T07:00:00Z runtime broker readiness failed: dependency=openbao error=connection refused\n",
		readyz: `{"status":"not_ready","dependency":"openbao","version":"0.4.28"}`,
	}
	details := runtimeBrokerReadinessDiagnostics(context.Background(), f, "bh-demo-app-dev", "/tmp/compose.yaml", "/tmp/runtime.env")
	if !strings.Contains(details, "dependency=openbao") {
		t.Fatalf("diagnostics omitted the log dependency name: %q", details)
	}
	if !strings.Contains(details, "status=not_ready dependency=openbao") {
		t.Fatalf("diagnostics omitted the /readyz dependency name: %q", details)
	}
	if !strings.Contains(details, "broker container log tail:") || !strings.Contains(details, "broker /readyz (last):") {
		t.Fatalf("diagnostics missing one of its two channels: %q", details)
	}
}

func TestRuntimeBrokerReadinessDiagnosticsProbeFailureIsNoted(t *testing.T) {
	// A failing probe must surface as an explicit note, never as an empty block.
	f := &brokerDiagnosticsFixture{logs: "starting up", execErr: errors.New("probe exec failed")}
	details := runtimeBrokerReadinessDiagnostics(context.Background(), f, "bh-demo-app-dev", "/tmp/compose.yaml", "/tmp/runtime.env")
	if !strings.Contains(details, "probe exec failed") {
		t.Fatalf("probe failure was not noted in diagnostics: %q", details)
	}
}

func TestTruncateDiagnosticTailKeepsTailOnLineBoundary(t *testing.T) {
	body := strings.Repeat("filler line\n", 500) + "dependency=runtime-executor final line\n"
	tail := truncateDiagnosticTail(body, 64)
	if !strings.Contains(tail, "dependency=runtime-executor final line") {
		t.Fatalf("truncation dropped the trailing line: %q", tail)
	}
	if len(tail) > len(body) || tail == body {
		t.Fatalf("truncation did not bound the text: len=%d", len(tail))
	}
	if !strings.HasPrefix(tail, "[…truncated…]") {
		t.Fatalf("truncation marker missing: %q", tail)
	}
}
