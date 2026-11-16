package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestCreateTargetManagedApplicationRegistersResolvableDeployment(t *testing.T) {
	target := configureTestTarget(t)
	manifest := application.New("managed-demo", "dev", true, false, false)

	path, err := createTargetManagedApplication(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}

	record, found, err := deployment.FindDeployment(target.Name, manifest.ApplicationID, manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("managed application deployment record not found")
	}
	id := record.Identity
	root, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "source", application.RepositoryManifestName)
	if path != wantPath {
		t.Fatalf("managed source path = %q, want %q", path, wantPath)
	}

	record, err = deployment.LoadDeploymentRecord(id)
	if err != nil {
		t.Fatal(err)
	}
	if record.Source.Kind != "managed" {
		t.Fatalf("source kind = %q, want managed", record.Source.Kind)
	}
	if record.Source.Repository != filepath.Dir(wantPath) {
		t.Fatalf("source repository = %q, want %q", record.Source.Repository, filepath.Dir(wantPath))
	}
	if record.Applied.RuntimeProvider != target.RuntimeProvider {
		t.Fatalf("runtime provider = %q, want %q", record.Applied.RuntimeProvider, target.RuntimeProvider)
	}
	// AuthoredYAML omits system-owned IDs. The Core must retain its identity
	// in the canonical source Store before a later read/apply can use it.
	owner := application.Store{Root: filepath.Join(record.Source.Repository, ".baseharbor", "apps")}
	stored, err := owner.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ApplicationID != manifest.ApplicationID ||
		stored[0].Name != manifest.Name || stored[0].Environment != manifest.Environment {
		t.Fatalf("Core-assigned application identity not retained in source Store: %+v", stored)
	}

	resolved, err := resolveApplication(context.Background(), application.Store{}, []string{manifest.Name}, "show")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.DeploymentIdentity != id {
		t.Fatalf("resolved identity = %#v, want %#v", resolved.DeploymentIdentity, id)
	}
	if !resolved.SourceAvailable {
		t.Fatal("managed application source should be available")
	}
	if resolved.Manifest.Name != manifest.Name || resolved.Manifest.Environment != manifest.Environment {
		t.Fatalf("resolved manifest = %s/%s, want %s/%s", resolved.Manifest.Name, resolved.Manifest.Environment, manifest.Name, manifest.Environment)
	}

	if _, err := createTargetManagedApplication(context.Background(), manifest); !errors.Is(err, application.ErrExists) {
		t.Fatalf("second create error = %v, want ErrExists", err)
	}
}

func TestCreateTargetManagedApplicationPersistsImmutableGitSourceSelection(t *testing.T) {
	target := configureTestTarget(t)
	manifest := application.New("remote-git-source", "dev", true, false, false)
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion, Application: manifest.Name,
		Sources: []development.SourceDefinition{{
			ID: "app", Type: development.SourceRepository,
			Repository: "https://git.example/remote-git-source.git", Ref: strings.Repeat("a", 40),
		}},
		Components: []development.ComponentSource{{Component: "app", Source: "app"}},
	}
	path, err := createTargetManagedApplication(context.Background(), manifest, &model)
	if err != nil {
		t.Fatalf("persist immutable remote Git selection: %v", err)
	}
	record, found, err := deployment.FindDeployment(target.Name, manifest.ApplicationID, "dev")
	if err != nil || !found || record.Source.Kind != "managed" {
		t.Fatalf("Core-owned application registration failed: record=%+v found=%v error=%v", record, found, err)
	}
	restored, _, err := development.LoadSourceModel(path)
	if err != nil {
		t.Fatalf("Core did not retain source selection: %v", err)
	}
	if restored.Sources[0].Ref != model.Sources[0].Ref ||
		restored.Sources[0].Repository != model.Sources[0].Repository {
		t.Fatalf("Core mutated immutable source selection: %+v", restored)
	}
	// Persisting the source selection does not create a Git checkout, run
	// builds or claim the source has been admitted for a deployment.
	if record.Observed.State == "ready" || record.Observed.Ready {
		t.Fatalf("source registration falsely claimed ready: %+v", record.Observed)
	}
}

func TestCreateTargetManagedApplicationRejectsMutableOrForeignSources(t *testing.T) {
	configureTestTarget(t)
	manifest := application.New("remote-source-failclosed", "dev", false, false, false)
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion, Application: manifest.Name,
		Sources: []development.SourceDefinition{{
			ID: "app", Type: development.SourceRepository,
			Repository: "https://git.example/remote-source.git", Ref: "main",
		}},
		Components: []development.ComponentSource{{Component: "app", Source: "app"}},
	}
	if _, err := createTargetManagedApplication(context.Background(), manifest, &model); err == nil {
		t.Fatal("Core accepted mutable Git branch during first registration")
	}
	model.Sources[0].Ref = strings.Repeat("a", 40)
	model.Application = "foreign"
	if _, err := createTargetManagedApplication(context.Background(), manifest, &model); err == nil {
		t.Fatal("Core accepted a source declaration for another application")
	}
}
