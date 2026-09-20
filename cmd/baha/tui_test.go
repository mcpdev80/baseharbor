package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

func TestTUIRejectsPlainMode(t *testing.T) {
	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{Plain: true})
	var out bytes.Buffer
	err := tuiCommand(application.Store{Root: t.TempDir()}).Run(ctx, nil, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "unavailable in --plain mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTUIRejectsNoInputMode(t *testing.T) {
	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{NonInteractive: true})
	var out bytes.Buffer
	err := tuiCommand(application.Store{Root: t.TempDir()}).Run(ctx, nil, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "unavailable in --no-input mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTUISummaryAndDoctorRemainReadableWithoutColor(t *testing.T) {
	success := lipgloss.NewStyle()
	failure := lipgloss.NewStyle()
	status := application.StatusResult{
		Application: "mailflow",
		Environment: "dev",
		Project:     "baseharbor-mailflow-dev",
		Ready:       false,
		Checks: []application.StatusCheck{
			{Name: "postgres", OK: true, Detail: "authenticated SELECT 1"},
			{Name: "workload/api", OK: false, Detail: "health check timed out"},
		},
	}
	summary := renderTUISummary(status, 60, success, failure)
	for _, wanted := range []string{"DEGRADED", "mailflow", "1 passed", "1 failed", "baha doctor"} {
		if !strings.Contains(summary, wanted) {
			t.Fatalf("summary missing %q:\n%s", wanted, summary)
		}
	}

	doctor := renderTUIDoctor(tuiDoctorResult{
		Healthy: false,
		Checks:  []preflight.Result{{Name: "workload", OK: false, Detail: "health check timed out"}},
	}, 60, success, failure)
	for _, wanted := range []string{"DEGRADED", "FAILED", "workload", "baha doctor --verbose"} {
		if !strings.Contains(doctor, wanted) {
			t.Fatalf("doctor view missing %q:\n%s", wanted, doctor)
		}
	}
}
