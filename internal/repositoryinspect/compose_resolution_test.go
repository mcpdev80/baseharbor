package repositoryinspect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestComposeExternalInterpolationRemainsExplicit(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"compose.yml": []byte("services:\n  api:\n    image: example/api:${TAG}\n    ports:\n      - \"${PORT}:8080\"\n"),
	}}
	candidates, err := InspectWorkloadSources(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v", candidates)
	}
	evidence, err := NormalizeWorkloadSource(snapshot, candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Source.Unresolved) != 2 {
		t.Fatalf("unresolved = %#v", evidence.Source.Unresolved)
	}
	if len(evidence.Components) != 1 || evidence.Components[0].Image != "example/api:${TAG}" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestComposeDeterministicDefaultsResolve(t *testing.T) {
	services, err := detectComposeServices([]byte(`services:
  api:
    image: example/api:${TAG:-release}
    build: ${BUILD_CONTEXT:-.}
    ports:
      - "${PORT:-8080}:8080"
  gateway:
    image: example/gateway:${GATEWAY_TAG-latest}
    ports:
      - "${PUBLIC_PORT:-${INNER_PORT:-8000}}:8000/tcp"
`))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]composeService{}
	for _, service := range services {
		byName[service.Name] = service
	}
	if got := byName["api"]; got.Image != "example/api:release" || got.Build != "." || len(got.Ports) != 1 || got.Ports[0] != "8080:8080" {
		t.Fatalf("api = %#v", got)
	}
	if got := byName["gateway"]; got.Image != "example/gateway:latest" || len(got.Ports) != 1 || got.Ports[0] != "8000:8000/tcp" {
		t.Fatalf("gateway = %#v", got)
	}
}

func TestInspectIgnoresBrokenUnselectedComposeCandidate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docker", "docker-compose.yml"), []byte("services:\n  api:\n    image: example/api:1\n  db:\n    image: postgres:17\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "compose.yml"), []byte("include:\n  - missing.yml\nservices:\n  test:\n    image: example/test:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("unselected broken Compose candidate poisoned inspection: %v", err)
	}
	if result.SelectedWorkloadSource == nil || result.SelectedWorkloadSource.Path != "docker/docker-compose.yml" {
		t.Fatalf("selected source = %#v", result.SelectedWorkloadSource)
	}
	foundSQL := false
	for _, finding := range result.Findings {
		if finding.Capability == "database.sql" {
			foundSQL = true
			for _, evidence := range finding.Evidence {
				if evidence.Path == "tests/compose.yml" {
					t.Fatalf("unselected test Compose leaked into capability evidence: %#v", finding)
				}
			}
		}
	}
	if !foundSQL {
		t.Fatalf("selected Compose database capability missing: %#v", result.Findings)
	}
}

func TestInspectSelectedUnsupportedComposeFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yml"), []byte("include:\n  - external.yml\nservices:\n  api:\n    image: example/api:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(context.Background(), root); err == nil {
		t.Fatal("selected Compose with unsupported include must fail")
	}
}

func TestSnapshotForSelectedWorkloadSourceRemovesForeignCompose(t *testing.T) {
	snapshot := Snapshot{Files: map[string][]byte{
		"docker/compose.yml": []byte("services: {}\n"),
		"tests/compose.yml":  []byte("services: {}\n"),
		"main.go":            []byte("package main\n"),
	}}
	selected := &WorkloadSourceCandidate{Kind: WorkloadSourceCompose, Path: "docker/compose.yml"}
	filtered := snapshotForSelectedWorkloadSource(snapshot, selected)
	if _, ok := filtered.Files["docker/compose.yml"]; !ok {
		t.Fatal("selected Compose removed")
	}
	if _, ok := filtered.Files["tests/compose.yml"]; ok {
		t.Fatal("unselected Compose remained in detector snapshot")
	}
	if _, ok := filtered.Files["main.go"]; !ok {
		t.Fatal("normal source file removed")
	}
}
