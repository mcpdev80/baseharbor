package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppNewWizardCancelLeavesNoProject(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	oldInput := appNewInput
	defer func() { appNewInput = oldInput }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"cancel-app",
		parent,
		"1",
		"1",
		"n",
		"",
	}, "\n"))

	var out bytes.Buffer
	if err := runAppNewWizard(context.Background(), &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, "cancel-app")); !os.IsNotExist(err) {
		t.Fatalf("cancelled wizard mutated project root: %v", err)
	}
	if !strings.Contains(out.String(), "No changes were made.") {
		t.Fatalf("missing cancellation confirmation:\n%s", out.String())
	}
}

func TestAppNewWizardCreatesValidatedProject(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	oldInput := appNewInput
	defer func() { appNewInput = oldInput }()
	appNewInput = strings.NewReader(strings.Join([]string{
		"guided-app",
		parent,
		"1",
		"1,2",
		"",
		"",
	}, "\n"))

	var out bytes.Buffer
	if err := runAppNewWizard(context.Background(), &out, &out); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "guided-app")
	for _, path := range []string{
		"baseharbor.yaml",
		"compose.yaml",
		".baseharbor/stack-profile.yaml",
		".baseharbor/development-plan.json",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected %s: %v\n%s", path, err, out.String())
		}
	}
	if !strings.Contains(out.String(), "Validation: SATISFIED") {
		t.Fatalf("wizard did not report validated result:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Project root:") {
		t.Fatalf("wizard did not preview project root:\n%s", out.String())
	}
}
