package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
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

func TestStackCreateWizardBuildsReusableProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())

	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := development.LoadProfileCatalog(".", builtinDevelopmentProfiles(registry))
	if err != nil {
		t.Fatal(err)
	}

	input := strings.NewReader(strings.Join([]string{
		"team-stack",
		"1",
		"go",
		"app",
		"application",
		"1",
		"",
	}, "\n"))
	var out bytes.Buffer
	selection, err := guidedCreateStackProfile(bufio.NewReader(input), &out, catalog, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Created || selection.RawProfile == nil {
		t.Fatalf("wizard did not create reusable profile: %#v", selection)
	}
	if selection.SaveScope != development.ProfileScopeUser {
		t.Fatalf("scope = %q, want user", selection.SaveScope)
	}
	if selection.RawProfile.Metadata.Name != "team-stack" {
		t.Fatalf("name = %q", selection.RawProfile.Metadata.Name)
	}
	if len(selection.Effective.Components) != 1 || selection.Effective.Components[0].Adapter != "development/go" {
		t.Fatalf("unexpected effective profile: %#v", selection.Effective)
	}
}
