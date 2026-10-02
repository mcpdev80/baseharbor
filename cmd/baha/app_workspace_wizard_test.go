package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestWorkspaceWizardCreatesPortableModelAndLocalMapping(t *testing.T) {
	root := t.TempDir()
	appRoot := filepath.Join(root, "application")
	frontend := filepath.Join(root, "frontend")
	api := filepath.Join(root, "api")
	for _, dir := range []string{appRoot, frontend, api} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manifest := "version: 1\napp:\n  id: 11111111-1111-4111-8111-111111111111\n  name: webshop\n  environment: dev\nworkload:\n  compose: compose.yaml\n  services:\n    - app\n"
	if err := os.WriteFile(filepath.Join(appRoot, "baseharbor.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appRoot, "compose.yaml"), []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	oldInput := appWorkspaceInput
	defer func() { appWorkspaceInput = oldInput }()
	appWorkspaceInput = strings.NewReader(strings.Join([]string{
		"frontend",
		"1",
		frontend,
		"frontend-source",
		"https://git.example/frontend.git",
		"main",
		".",
		"y",
		"api",
		"2",
		api,
		"api-source",
		"https://git.example/api.git",
		"main",
		".",
		"y",
		"external",
		"4",
		"external-source",
		"ghcr.io/example/external@sha256:deadbeef",
		"n",
		"y",
	}, "\n"))

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(appRoot); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runAppWorkspaceWizard(context.Background(), &out); err != nil {
		t.Fatalf("wizard failed: %v\n%s", err, out.String())
	}

	model, _, err := development.LoadSourceModel(filepath.Join(appRoot, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Components) != 3 || len(model.Sources) != 3 {
		t.Fatalf("unexpected source model: %#v", model)
	}
	data, err := os.ReadFile(filepath.Join(appRoot, development.SourceModelRelativePath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), frontend) || strings.Contains(string(data), api) {
		t.Fatalf("portable source model leaked local path:\n%s", data)
	}

	mapping, _, err := development.LoadWorkspaceMapping(filepath.Join(appRoot, "baseharbor.yaml"), "webshop")
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Sources["frontend-source"] != frontend || mapping.Sources["api-source"] != api {
		t.Fatalf("workspace mapping = %#v", mapping)
	}
	resolved, err := development.ResolveWorkspace(filepath.Join(appRoot, "baseharbor.yaml"), model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Components) != 3 {
		t.Fatalf("resolved components = %#v", resolved.Components)
	}
	if !strings.Contains(out.String(), "Canonical manifest:") || !strings.Contains(out.String(), "Resolution: SATISFIED") {
		t.Fatalf("wizard output missing ownership/resolution summary:\n%s", out.String())
	}
}

func TestWorkspaceWizardCancelDoesNotWriteState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "version: 1\napp:\n  id: 22222222-2222-4222-8222-222222222222\n  name: demo\n  environment: dev\nworkload:\n  compose: compose.yaml\n  services:\n    - app\n"
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  app:\n    image: alpine:3.20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	oldInput := appWorkspaceInput
	defer func() { appWorkspaceInput = oldInput }()
	appWorkspaceInput = strings.NewReader(strings.Join([]string{
		"app",
		"2",
		"app-image",
		"ghcr.io/example/app:latest",
		"n",
		"n",
	}, "\n"))
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runAppWorkspaceWizard(context.Background(), &out); err != nil {
		t.Fatalf("wizard failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, development.SourceModelRelativePath)); !os.IsNotExist(err) {
		t.Fatalf("cancelled wizard wrote portable state: %v", err)
	}
	if !strings.Contains(out.String(), "No changes were made.") {
		t.Fatalf("cancel output missing confirmation:\n%s", out.String())
	}
}
