package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func TestManifestFromCreateArgsSourceNeutralWorkloadPositive(t *testing.T) {
	m, err := manifestFromCreateArgs([]string{"demo", "--workload-component", "api", "--workload-component", "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Services.SQL {
		t.Fatal("workload-only contract unexpectedly enabled SQL")
	}
	got := application.WorkloadComponentNames(m)
	if len(got) != 2 || got[0] != "api" || got[1] != "worker" {
		t.Fatalf("components = %#v", got)
	}
	yaml := m.YAML()
	if !strings.Contains(yaml, "components:") || strings.Contains(yaml, "compose:") || strings.Contains(yaml, "services:\n    - api") {
		t.Fatalf("portable workload leaked source-specific identity:\n%s", yaml)
	}
}

func TestParseWorkloadSourceArgPositive(t *testing.T) {
	got, ok, err := parseWorkloadSourceArg([]string{"demo", "--workload-source", "kubernetes:deploy/k8s"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Kind != repositoryinspect.WorkloadSourceKubernetes || got.Path != "deploy/k8s" {
		t.Fatalf("source = %#v ok=%v", got, ok)
	}
}

func TestParseWorkloadSourceArgRejectsUnknownKindNegative(t *testing.T) {
	if _, _, err := parseWorkloadSourceArg([]string{"--workload-source", "helm:deploy/chart"}); err == nil {
		t.Fatal("expected Helm source to be rejected in v0.4.20")
	}
}

func TestDeterministicInitRejectsAmbiguousSourcesWithoutSelectionNegative(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deployment.yaml"), []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\nspec:\n  template:\n    spec:\n      containers:\n        - name: api\n          image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := appInitCommand().Run(context.Background(), []string{"demo", "--workload-component", "api"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "multiple workload sources") {
		t.Fatalf("expected ambiguous workload source error, got %v", err)
	}
}

func TestDeterministicInitDoesNotPersistUnneededSourceSelection(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := appInitCommand().Run(context.Background(), []string{"demo", "--workload-source", "compose:compose.yaml", "--workload-component", "api"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.repository.yaml")); !os.IsNotExist(err) {
		t.Fatalf("unambiguous source selection should not require repository metadata, err=%v", err)
	}
}

func TestDeterministicInitPersistsAmbiguousSourceSelectionPositive(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deployment.yaml"), []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\nspec:\n  template:\n    spec:\n      containers:\n        - name: api\n          image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := appInitCommand().Run(context.Background(), []string{"demo", "--workload-source", "compose:compose.yaml", "--workload-component", "api"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "baseharbor.repository.yaml")); err != nil {
		t.Fatalf("ambiguous explicit source selection metadata missing: %v", err)
	}
}

func TestParseWorkloadSourceArgRejectsDuplicateNegative(t *testing.T) {
	if _, _, err := parseWorkloadSourceArg([]string{"--workload-source", "compose:compose.yaml", "--workload-source", "kubernetes:deploy/k8s"}); err == nil {
		t.Fatal("expected duplicate source selection to fail")
	}
}
