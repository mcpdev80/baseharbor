package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestManagedCoreRegistrationPinsSourceOnLaterApply(t *testing.T) {
	root := t.TempDir()
	manifest := application.New("pinned-managed-app", "dev", false, false, false)
	path := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(path, []byte(manifest.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion,
		Application:   manifest.Name,
		Sources: []development.SourceDefinition{{
			ID: "app", Type: development.SourceRepository,
			Repository: "https://git.example/pinned-managed-app.git",
			Ref:        strings.Repeat("a", 40),
		}},
		Components: []development.ComponentSource{{Component: "app", Source: "app"}},
	}
	if _, err := development.WriteSourceModel(path, model); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Manifest: manifest, ManifestPath: path,
		DeploymentRecord: &deployment.DeploymentRecord{
			Source: deployment.DeploymentSource{Kind: "managed", Repository: root, Manifest: path},
		},
	}
	bound, err := bindRegisteredCoreSource(resolved, nil)
	if err != nil || bound == nil || bound.Sources[0].Ref != model.Sources[0].Ref {
		t.Fatalf("registered immutable Git selection was not loaded: %+v, %v", bound, err)
	}
	if _, err := bindRegisteredCoreSource(resolved, &model); err != nil {
		t.Fatalf("matching client source was refused: %v", err)
	}
	foreign := model
	foreign.Sources = append([]development.SourceDefinition(nil), model.Sources...)
	foreign.Sources[0].Ref = strings.Repeat("b", 40)
	if _, err := bindRegisteredCoreSource(resolved, &foreign); err == nil {
		t.Fatal("client replaced authoritative Git commit without a source-change operation")
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(development.SourceModelRelativePath))); err != nil {
		t.Fatal(err)
	}
	if bound, err := bindRegisteredCoreSource(resolved, nil); err != nil || bound != nil {
		t.Fatalf("legacy managed app with no SourceModel was changed: %+v, %v", bound, err)
	}
	if _, err := bindRegisteredCoreSource(resolved, &model); err == nil {
		t.Fatal("unregistered client-supplied source bypassed Core authority")
	}
}

func TestRegisteredCoreSourceDoesNotOverrideRepositoryOwnedApplication(t *testing.T) {
	manifest := application.New("existing-repository-app", "dev", false, false, false)
	source := &development.SourceModel{SchemaVersion: development.SourceModelVersion, Application: manifest.Name}
	resolved := resolvedApplication{
		Manifest: manifest,
		DeploymentRecord: &deployment.DeploymentRecord{
			Source: deployment.DeploymentSource{Kind: "repository", Repository: "/registered/repo"},
		},
	}
	bound, err := bindRegisteredCoreSource(resolved, source)
	if err != nil || bound != source {
		t.Fatalf("legacy repository-owned application was unexpectedly modified: %+v, %v", bound, err)
	}
}
