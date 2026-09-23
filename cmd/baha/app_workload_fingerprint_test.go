package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRepositoryWorkloadBuildFingerprintChangesWithSource(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(name, value string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("Dockerfile", "FROM scratch\nCOPY app.txt /app.txt\n")
	mustWrite("app.txt", "one\n")
	workload := application.WorkloadFiles{RepositoryRoot: root, Compose: filepath.Join(root, "compose.yaml")}
	raw := json.RawMessage(`{"context":".","dockerfile":"Dockerfile"}`)
	build := repositoryWorkloadBuildConfig{Context: ".", Dockerfile: "Dockerfile"}

	first, err := fingerprintRepositoryWorkloadBuild(workload, "api", raw, build)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite("app.txt", "two\n")
	second, err := fingerprintRepositoryWorkloadBuild(workload, "api", raw, build)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("source change did not change build fingerprint")
	}
}

func TestRepositoryWorkloadBuildFingerprintHonorsDockerIgnore(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("Dockerfile", "FROM scratch\n")
	mustWrite(".dockerignore", "ignored.txt\n")
	mustWrite("app.txt", "stable\n")
	mustWrite("ignored.txt", "one\n")
	workload := application.WorkloadFiles{RepositoryRoot: root, Compose: filepath.Join(root, "compose.yaml")}
	raw := json.RawMessage(`{"context":"."}`)
	build := repositoryWorkloadBuildConfig{Context: "."}

	first, err := fingerprintRepositoryWorkloadBuild(workload, "api", raw, build)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite("ignored.txt", "two\n")
	second, err := fingerprintRepositoryWorkloadBuild(workload, "api", raw, build)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("ignored source changed build fingerprint")
	}
}

func TestRepositoryWorkloadBuildFingerprintIgnoresBaseHarborState(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".baseharbor"), 0o700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, ".baseharbor", "state")
	if err := os.WriteFile(statePath, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	workload := application.WorkloadFiles{RepositoryRoot: root, Compose: filepath.Join(root, "compose.yaml")}
	raw := json.RawMessage(`{"context":"."}`)
	build := repositoryWorkloadBuildConfig{Context: "."}

	first, err := fingerprintRepositoryWorkloadBuild(workload, "api", raw, build)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := fingerprintRepositoryWorkloadBuild(workload, "api", raw, build)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("BaseHarbor state changed application build fingerprint")
	}
}

func TestChangedRepositoryWorkloadBuildServicesIsSelective(t *testing.T) {
	current := map[string]string{"api": "new", "worker": "same"}
	previous := map[string]string{"api": "old", "worker": "same"}
	changed := changedRepositoryWorkloadBuildServices(current, previous)
	if len(changed) != 1 || changed[0] != "api" {
		t.Fatalf("changed=%v", changed)
	}
}
