package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestCreateTargetManagedApplicationRegistersResolvableDeployment(t *testing.T) {
	target := configureTestTarget(t)
	manifest := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "managed-demo",
		Environment: "dev",
		Services: application.Services{
			SQL: true,
		},
	}

	path, err := createTargetManagedApplication(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}

	id := deployment.DeploymentIdentity{
		Target:      target.Name,
		Application: manifest.Name,
		Environment: manifest.Environment,
	}
	root, err := deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "source", application.RepositoryManifestName)
	if path != wantPath {
		t.Fatalf("managed source path = %q, want %q", path, wantPath)
	}

	record, err := deployment.LoadDeploymentRecord(id)
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
