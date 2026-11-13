package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppDestroyAlreadyAbsentUninitializedRepository(t *testing.T) {
	configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	root := selection.RepositoryRoot
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("destroy of an uninitialized, unregistered repository should be idempotent: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Application already absent") || !strings.Contains(out.String(), "source files were kept") {
		t.Fatalf("missing human-readable no-op result: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".baseharbor", "apps")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destroy created identity state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "compose.yaml")); err != nil {
		t.Fatalf("destroy removed authored source: %v", err)
	}
}

func TestTopLevelDestroyKeepsInstallationScopeInsideRepository(t *testing.T) {
	configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	t.Chdir(selection.RepositoryRoot)

	var out bytes.Buffer
	err := runWithIO(context.Background(), []string{"destroy", "--invalid-test-option"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown argument --invalid-test-option") {
		t.Fatalf("top-level destroy was routed to an application command: err=%v output=%s", err, out.String())
	}
}
