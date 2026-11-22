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
	// 0428-240 shape); the failure branch must name the dependency in the
	// diagnostics through the safe channels.
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
	if !strings.Contains(err.Error(), "broker /readyz (last):") {
		t.Fatalf("readyz channel missing from the failure text: %v", err)
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
}

func TestRuntimeBrokerReadinessDiagnosticsProbeFailureIsNoted(t *testing.T) {
	// A failing probe must surface as an explicit FIXED note, never as an
	// empty block and never carrying the raw error detail.
	f := &brokerDiagnosticsFixture{logs: "starting up", execErr: errors.New("probe exec failed at /run/secrets/token=abc123")}
	details := runtimeBrokerReadinessDiagnostics(context.Background(), f, "bh-demo-app-dev", "/tmp/compose.yaml", "/tmp/runtime.env")
	if !strings.Contains(details, "readyz probe failed") {
		t.Fatalf("probe failure was not noted in diagnostics: %q", details)
	}
	if strings.Contains(details, "probe exec failed") || strings.Contains(details, "abc123") {
		t.Fatalf("probe error detail leaked into diagnostics: %q", details)
	}
}

// TestRuntimeBrokerReadinessDiagnosticsNeverEchoesValueMarker is the security
// rule receipt (0428-240 redaction round): a /readyz body or log stream that
// carries a fabricated value marker must never surface that marker (or any
// raw content) in the diagnostics text — only fixed phrases and validated
// names are allowed.
func TestRuntimeBrokerReadinessDiagnosticsNeverEchoesValueMarker(t *testing.T) {
	const marker = "token=abc123"
	cases := []struct {
		name   string
		logs   string
		readyz string
	}{
		{"non-json body with marker", "clean log line dependency=openbao", "health endpoint says " + marker},
		{"hostile dependency value", "plain log", `{"status":"not_ready","dependency":"` + marker + `"}`},
		{"json body with marker in unknown field", "logs " + marker + " mention nothing else", `{"status":"not_ready","dependency":"openbao","leaked":"` + marker + `"}`},
		{"hostile body: not JSON at all", "plain log", marker},
		{"marker only in log stream", "broker started " + marker + " dependency=runtime-executor refused", `{"status":"not_ready","dependency":"runtime-executor"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &brokerDiagnosticsFixture{logs: tc.logs, readyz: tc.readyz}
			details := runtimeBrokerReadinessDiagnostics(context.Background(), f, "bh-demo-app-dev", "/tmp/compose.yaml", "/tmp/runtime.env")
			if strings.Contains(details, marker) || strings.Contains(details, "abc123") {
				t.Fatalf("value marker leaked into diagnostics: %q", details)
			}
			// The dependency name must STILL survive through the safe set.
			if tc.name == "json body with marker in unknown field" {
				// This body parses as JSON and names openbao through the safe set.
				if !strings.Contains(details, "dependency=openbao") {
					t.Fatalf("safe dependency name lost: %q", details)
				}
			}
			if !strings.Contains(details, "dependency=") && !strings.Contains(details, "status=unknown") {
				t.Fatalf("redaction dropped every dependency/status signal from the channel: %q", details)
			}
			for _, fragment := range strings.Fields(details) {
				if strings.Contains(fragment, "=") && !strings.HasPrefix(fragment, "status=") &&
					!strings.HasPrefix(fragment, "dependency=") && !strings.HasPrefix(fragment, "version=") &&
					!strings.HasPrefix(fragment, "commit=") && !strings.HasPrefix(fragment, "body-length=") {
					t.Fatalf("unexpected key=value content in diagnostics: %q (%q)", fragment, details)
				}
			}
		})
	}
}
