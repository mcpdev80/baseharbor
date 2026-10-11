package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func minimalRepositorySelection(t *testing.T) application.RepositoryEnvironmentSelection {
	t.Helper()
	root := t.TempDir()
	authored := "version: 1\napp:\n  name: plain\n  environment: dev\n"
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(authored), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selection, err := application.ResolveRepositoryEnvironment(root, "")
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func TestMinimalRepositoryIdentityInitializationAndReadSurfaces(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	authored, _ := os.ReadFile(selection.ManifestPath)
	for _, command := range []string{"plan", "status", "doctor", "stop", "destroy"} {
		_, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, command)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.CauseCode != "application_initialization_required" {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if _, err := os.Stat(filepath.Join(selection.RepositoryRoot, ".baseharbor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only surface initialized identity: %v", err)
	}
	initialized, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, "init")
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Manifest.ApplicationID == "" || initialized.Manifest.Services.SQL || !application.HasExplicitWorkload(initialized.Manifest) {
		t.Fatalf("incorrect minimal realization: %+v", initialized.Manifest)
	}
	for _, command := range []string{"init", "apply", "up", "plan", "status", "doctor", "stop", "destroy"} {
		next, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, command)
		if err != nil || next.Manifest.ApplicationID != initialized.Manifest.ApplicationID {
			t.Fatalf("%s identity drift: %v", command, err)
		}
	}
	after, _ := os.ReadFile(selection.ManifestPath)
	if string(after) != string(authored) {
		t.Fatal("authored intent rewritten")
	}
}

func TestMinimalRepositoryMissingSourceDoesNotInitialize(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	if err := os.Remove(filepath.Join(selection.RepositoryRoot, "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, "apply")
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.CauseCode != "workload_missing" {
		t.Fatalf("missing actionable workload error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(selection.RepositoryRoot, ".baseharbor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source allocated state: %v", err)
	}
}
