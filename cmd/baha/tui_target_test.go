package main

import (
	"context"
	"strings"
	"testing"
)

func TestTargetSelectionOrigin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	if got := targetSelectionOrigin(context.Background()); got != "implicit-local" {
		t.Fatalf("unexpected origin %q", got)
	}
	if err := writePersistedTarget("docker-test"); err != nil {
		t.Fatal(err)
	}
	if got := targetSelectionOrigin(context.Background()); got != "persisted" {
		t.Fatalf("unexpected origin %q", got)
	}
	t.Setenv("BASEHARBOR_TARGET", "podman-test")
	if got := targetSelectionOrigin(context.Background()); got != "environment" {
		t.Fatalf("unexpected origin %q", got)
	}
	if got := targetSelectionOrigin(withTargetOverride(context.Background(), "explicit-test")); got != "explicit" {
		t.Fatalf("unexpected origin %q", got)
	}
}

func TestRenderCoreTUIStatus(t *testing.T) {
	report := controlPlaneReport{Target: "docker-test", State: "running", Ready: true, Running: []string{"postgres", "openbao"}, Checks: []publicControlPlaneCheck{{Name: "Core Identity", Ready: true}}, AvailabilitySatisfied: true}
	view := renderCoreTUIStatus("docker-test", "docker", "local", "persisted", report)
	for _, want := range []string{"docker-test (persisted)", "Readiness    READY", "Core Identity", "postgres", "openbao"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in Core TUI", want)
		}
	}
}
