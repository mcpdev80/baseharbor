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

func TestTUIDoctorUsesConciseHumanDetails(t *testing.T) {
	success := lipgloss.NewStyle()
	failure := lipgloss.NewStyle()
	doctor := renderTUIDoctor(tuiDoctorResult{
		Healthy: false,
		Checks: []preflight.Result{
			{Name: "workload security", OK: false, Detail: "render repository Compose for security preflight: compose config --format json: services.api.environment.SECRET_KEY: required variable SECRET_KEY is missing a value"},
			{Name: "OpenBao application scope", OK: false, Detail: "inspect OpenBao status: compose exec -T openbao sh -c bao status"},
			{Name: "application runtime broker", OK: false, Detail: "compose exec -T broker curl --fail https://baseharbor-runtime:8443/readyz: 503"},
		},
	}, 80, success, failure)

	for _, forbidden := range []string{"compose config --format json", "compose exec", "curl --fail", "bao status"} {
		if strings.Contains(doctor, forbidden) {
			t.Fatalf("TUI doctor leaked low-level diagnostic %q:\n%s", forbidden, doctor)
		}
	}
	for _, wanted := range []string{
		"BaseHarbor/OpenBao managed secret",
		"OpenBao application scope unavailable",
		"runtime broker is not ready",
		"baha app doctor --fix",
	} {
		if !strings.Contains(doctor, wanted) {
			t.Fatalf("TUI doctor missing %q:\n%s", wanted, doctor)
		}
	}
}

func TestTUIStatusUsesConciseHumanDetails(t *testing.T) {
	success := lipgloss.NewStyle()
	failure := lipgloss.NewStyle()
	status := application.StatusResult{
		Application: "mailflow",
		Environment: "production",
		Ready:       false,
		Checks: []application.StatusCheck{
			{Name: "runtime-broker", OK: false, Detail: "compose exec -T broker curl --fail https://baseharbor-runtime:8443/readyz: 503"},
			{Name: "workload", OK: false, Detail: "resolve required workload secret SECRET_KEY: inspect OpenBao status: compose exec -T openbao sh -c bao status"},
		},
	}
	view := renderTUIOverview(status, 80, success, failure)
	for _, forbidden := range []string{"compose exec", "curl --fail", "bao status"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("TUI status leaked low-level diagnostic %q:\n%s", forbidden, view)
		}
	}
	for _, wanted := range []string{"runtime broker is not ready", "required secrets unavailable because OpenBao is not running"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("TUI status missing %q:\n%s", wanted, view)
		}
	}
}
