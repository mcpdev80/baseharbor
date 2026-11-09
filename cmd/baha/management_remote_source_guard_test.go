package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestRemoteSourceGuardPinsConfiguredGitOrOCI(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	manifest := application.New("demo", "dev", false, false, false)
	manifestPath := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(manifestPath, []byte(manifest.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion, Application: "demo",
		Sources: []development.SourceDefinition{{
			ID: "app", Type: development.SourceRepository,
			Repository: "https://git.example/demo.git", Ref: "main",
		}},
		Components: []development.ComponentSource{{Component: "app", Source: "app"}},
	}
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteSelectedSources("demo", "dev"); err == nil || !strings.Contains(err.Error(), "not immutable") {
		t.Fatalf("mutable Git ref was accepted: %v", err)
	}
	model.Sources[0].Ref = strings.Repeat("a", 40)
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteSelectedSources("demo", "dev"); err != nil {
		t.Fatalf("pinned Git source rejected: %v", err)
	}
	model.Sources[0] = development.SourceDefinition{
		ID: "app", Type: development.SourceOCI, Image: "ghcr.io/example/demo:latest",
	}
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteSelectedSources("demo", "dev"); err == nil {
		t.Fatal("mutable OCI tag was accepted")
	}
	model.Sources[0].Image = "ghcr.io/example/demo@sha256:" + strings.Repeat("b", 64)
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteSelectedSources("demo", "dev"); err != nil {
		t.Fatalf("pinned OCI source rejected: %v", err)
	}
	// A locally checked-out project cannot provide the authority for an
	// independently selected remote application.
	if err := validateRemoteSelectedSources("other-app", "dev"); err != nil {
		t.Fatalf("unrelated local repository overrode explicit Core application: %v", err)
	}
}
