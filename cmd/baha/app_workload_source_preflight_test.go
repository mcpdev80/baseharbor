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

func TestRepositoryWorkloadSecuritySkipsWhenNoWorkload(t *testing.T) {
	root := t.TempDir()
	m := application.New("demo", "dev", true, true, false)
	resolved := resolvedApplication{
		Manifest:       m,
		ManifestPath:   filepath.Join(root, "baseharbor.yaml"),
		RepositoryRoot: root,
		FromRepository: true,
	}

	if err := preflightRepositoryWorkload(resolved); err != nil {
		t.Fatalf("workload preflight must skip without workload: %v", err)
	}
	if _, err := preflightRepositoryWorkloadSecuritySource(resolved); err != nil {
		t.Fatalf("source security preflight must skip without workload: %v", err)
	}
	if _, err := preflightRepositoryWorkloadSecurity(context.Background(), nil, resolved); err != nil {
		t.Fatalf("rendered security preflight must skip without workload: %v", err)
	}
}

func TestRepositoryWorkloadSourceRealizationComposePositive(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := application.New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m = application.WithWorkloadComponents(m, "api")
	if err := preflightRepositoryWorkloadSourceRealization(root, m); err != nil {
		t.Fatalf("compose realization should be supported: %v", err)
	}
}

func TestRepositoryWorkloadSourceRealizationKubernetesNegative(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "deployment.yaml"), []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: api
          image: example/api:1
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := application.New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m = application.WithWorkloadComponents(m, "api")
	err := preflightRepositoryWorkloadSourceRealization(root, m)
	var typed *machine.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed machine error, got %T %v", err, err)
	}
	if typed.Code != machine.ErrorUnsupported || typed.CauseCode != "workload_source_runtime_unsupported" {
		t.Fatalf("unexpected typed error: %#v", typed)
	}
}

func TestRepositoryWorkloadSourceRealizationAmbiguousNegative(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deployment.yaml"), []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\nspec:\n  template:\n    spec:\n      containers:\n        - name: api\n          image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := application.New("demo", "dev", false, false, false)
	m.Services.SQL = false
	m = application.WithWorkloadComponents(m, "api")
	err := preflightRepositoryWorkloadSourceRealization(root, m)
	var typed *machine.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed machine error, got %T %v", err, err)
	}
	if typed.CauseCode != "workload_source_selection_required" {
		t.Fatalf("unexpected cause: %#v", typed)
	}
}
