package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestFingerprintRepositoryBuildContextTracksRelevantSource(t *testing.T) {
	root := t.TempDir()
	mustWriteBuildFile(t, root, "Dockerfile", "FROM scratch\nCOPY app.txt /app.txt\n")
	mustWriteBuildFile(t, root, "app.txt", "one\n")
	mustWriteBuildFile(t, root, ".dockerignore", "ignored.txt\n.baseharbor/\n")
	mustWriteBuildFile(t, root, "ignored.txt", "ignored-one\n")
	mustWriteBuildFile(t, root, ".baseharbor/generated", "generated-one\n")

	build := []byte(`{"context":".","dockerfile":"Dockerfile"}`)
	first, err := fingerprintRepositoryBuildContext(root, "Dockerfile", build)
	if err != nil {
		t.Fatal(err)
	}

	mustWriteBuildFile(t, root, "ignored.txt", "ignored-two\n")
	mustWriteBuildFile(t, root, ".baseharbor/generated", "generated-two\n")
	second, err := fingerprintRepositoryBuildContext(root, "Dockerfile", build)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("ignored/generated files changed build fingerprint: %s -> %s", first, second)
	}

	mustWriteBuildFile(t, root, "app.txt", "two\n")
	third, err := fingerprintRepositoryBuildContext(root, "Dockerfile", build)
	if err != nil {
		t.Fatal(err)
	}
	if third == second {
		t.Fatal("source change did not change build fingerprint")
	}
}

func TestFingerprintRepositoryBuildContextTracksBuildDefinition(t *testing.T) {
	root := t.TempDir()
	mustWriteBuildFile(t, root, "Dockerfile", "FROM scratch\n")
	first, err := fingerprintRepositoryBuildContext(root, "Dockerfile", []byte(`{"context":".","args":{"MODE":"one"}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := fingerprintRepositoryBuildContext(root, "Dockerfile", []byte(`{"context":".","args":{"MODE":"two"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("build-definition change did not change fingerprint")
	}
}

func TestChangedRepositoryWorkloadBuildServicesIsSelective(t *testing.T) {
	previous := repositoryWorkloadBuildState{
		Version:  1,
		Services: map[string]string{"api": "same", "worker": "old"},
	}
	got := changedRepositoryWorkloadBuildServices(map[string]string{
		"api":    "same",
		"worker": "new",
	}, previous)
	if len(got) != 1 || got[0] != "worker" {
		t.Fatalf("changed services = %v, want [worker]", got)
	}
}

func TestPersistRepositoryWorkloadBuildStateOwnerOnly(t *testing.T) {
	files := application.RuntimeFiles{Dir: t.TempDir()}
	want := map[string]string{"api": "abc"}
	if err := persistRepositoryWorkloadBuildState(files, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadRepositoryWorkloadBuildState(files)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Services["api"] != "abc" {
		t.Fatalf("loaded build state = %#v", got)
	}
	info, err := os.Stat(repositoryWorkloadBuildStatePath(files))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("build state mode = %o, want 600", info.Mode().Perm())
	}
}

func mustWriteBuildFile(t *testing.T, root, rel, value string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
