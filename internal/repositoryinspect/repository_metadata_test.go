package repositoryinspect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryMetadataSelectsAmbiguousSourcePositive(t *testing.T) {
	candidates := []WorkloadSourceCandidate{
		{Kind: WorkloadSourceCompose, Path: "compose.yaml"},
		{Kind: WorkloadSourceKubernetes, Path: "deploy/k8s"},
	}
	metadata, err := ParseRepositoryMetadata([]byte("version: 1\nworkload-source:\n  kind: kubernetes\n  path: deploy/k8s\n"))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectWorkloadSourceWithMetadata(candidates, &metadata)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.Kind != WorkloadSourceKubernetes || selected.Path != "deploy/k8s" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestRepositoryMetadataRejectsMissingCandidateNegative(t *testing.T) {
	candidates := []WorkloadSourceCandidate{{Kind: WorkloadSourceCompose, Path: "compose.yaml"}}
	metadata, err := ParseRepositoryMetadata([]byte("version: 1\nworkload-source:\n  kind: kubernetes\n  path: deploy/k8s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selectWorkloadSourceWithMetadata(candidates, &metadata); err == nil {
		t.Fatal("expected missing selected source to fail")
	}
}

func TestRepositoryMetadataRejectsEscapingPathNegative(t *testing.T) {
	if _, err := ParseRepositoryMetadata([]byte("version: 1\nworkload-source:\n  kind: compose\n  path: ../compose.yaml\n")); err == nil {
		t.Fatal("expected escaping path to fail")
	}
}

func TestInspectReportsInvalidRepositoryMetadataAsStructuredOutcome(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, RepositoryMetadataName), []byte("version: 1\nworkload-source:\n  kind: kubernetes\n  path: deploy/k8s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("inspect returned unstructured error: %v", err)
	}
	if result.WorkloadSourceResolution.State != WorkloadSourceResolutionInvalid ||
		result.WorkloadSourceResolution.Reason != WorkloadSourceReasonInvalidRepositoryMetadata ||
		result.WorkloadSourceResolution.Message == "" ||
		result.SelectedWorkloadSource != nil {
		t.Fatalf("resolution = %#v", result.WorkloadSourceResolution)
	}
}

func TestWriteRepositoryMetadataPositive(t *testing.T) {
	root := t.TempDir()
	path, err := WriteRepositoryMetadata(root, WorkloadSourceCandidate{Kind: WorkloadSourceQuadlet, Path: "deploy/quadlet"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != RepositoryMetadataName {
		t.Fatalf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := ParseRepositoryMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.WorkloadSource.Kind != WorkloadSourceQuadlet || metadata.WorkloadSource.Path != "deploy/quadlet" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
