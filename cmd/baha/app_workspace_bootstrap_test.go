package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestWorkspaceMapBootstrapsFirstRepositorySource(t *testing.T) {
	root := t.TempDir()
	appRoot := filepath.Join(root, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceBootstrapManifest(t, appRoot, []string{"app"})
	initWorkspaceGitRepo(t, appRoot, "https://git.example/acme/app.git")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	var out bytes.Buffer
	cmd := appWorkspaceMapCommand()
	if err := cmd.Run(
		context.Background(),
		[]string{"repo", appRoot, "--manifest", filepath.Join(appRoot, "baseharbor.yaml")},
		&out,
		&out,
	); err != nil {
		t.Fatalf("workspace map bootstrap failed: %v\n%s", err, out.String())
	}

	model, _, err := development.LoadSourceModel(filepath.Join(appRoot, "baseharbor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Sources) != 1 || model.Sources[0].ID != "repo" {
		t.Fatalf("unexpected source model: %#v", model)
	}
	if model.Sources[0].Repository != "https://git.example/acme/app.git" {
		t.Fatalf("repository identity = %q", model.Sources[0].Repository)
	}
	if model.Sources[0].Ref != "main" {
		t.Fatalf("repository ref = %q", model.Sources[0].Ref)
	}
	if len(model.Components) != 1 || model.Components[0].Component != "app" || model.Components[0].Source != "repo" {
		t.Fatalf("unexpected component mapping: %#v", model.Components)
	}
	if strings.Contains(model.Sources[0].Repository, appRoot) {
		t.Fatalf("portable source identity leaked local path: %#v", model.Sources[0])
	}

	mapping, _, err := development.LoadWorkspaceMapping(filepath.Join(appRoot, "baseharbor.yaml"), "demo")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, _ := filepath.Abs(appRoot)
	if mapping.Sources["repo"] != wantRoot {
		t.Fatalf("workspace mapping repo = %q, want %q", mapping.Sources["repo"], wantRoot)
	}
	if !strings.Contains(out.String(), "source model:") || !strings.Contains(out.String(), "mapped repo ->") {
		t.Fatalf("bootstrap output missing result: %s", out.String())
	}
}

func TestWorkspaceMapBootstrapRequiresStableGitOrigin(t *testing.T) {
	root := t.TempDir()
	appRoot := filepath.Join(root, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceBootstrapManifest(t, appRoot, []string{"app"})
	initWorkspaceGitRepo(t, appRoot, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	var out bytes.Buffer
	err := appWorkspaceMapCommand().Run(
		context.Background(),
		[]string{"repo", appRoot, "--manifest", filepath.Join(appRoot, "baseharbor.yaml")},
		&out,
		&out,
	)
	if err == nil || !strings.Contains(err.Error(), "stable Git origin") {
		t.Fatalf("expected stable-origin error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(appRoot, development.SourceModelRelativePath)); !os.IsNotExist(statErr) {
		t.Fatalf("failed bootstrap wrote source model: %v", statErr)
	}
}

func TestWorkspaceMapBootstrapDoesNotGuessMultiComponentMapping(t *testing.T) {
	root := t.TempDir()
	appRoot := filepath.Join(root, "app")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceBootstrapManifest(t, appRoot, []string{"api", "worker"})
	initWorkspaceGitRepo(t, appRoot, "https://git.example/acme/app.git")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	var out bytes.Buffer
	err := appWorkspaceMapCommand().Run(
		context.Background(),
		[]string{"repo", appRoot, "--manifest", filepath.Join(appRoot, "baseharbor.yaml")},
		&out,
		&out,
	)
	if err == nil || !strings.Contains(err.Error(), "cannot infer the first workspace component") {
		t.Fatalf("expected multi-component bootstrap error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(appRoot, development.SourceModelRelativePath)); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous bootstrap wrote source model: %v", statErr)
	}
}

func writeWorkspaceBootstrapManifest(t *testing.T, root string, components []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("version: 1\napp:\n  id: 33333333-3333-4333-8333-333333333333\n  name: demo\n  environment: dev\nworkload:\n  components:\n")
	for _, component := range components {
		b.WriteString("    - " + component + "\n")
	}
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initWorkspaceGitRepo(t *testing.T, root, origin string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "user.name", "BaseHarbor Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if origin != "" {
		cmd := exec.Command("git", "remote", "add", "origin", origin)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, output)
		}
	}
}
