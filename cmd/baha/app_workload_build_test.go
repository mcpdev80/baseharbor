package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFingerprintRenderedWorkloadServiceTracksInterpolatedEnvironment(t *testing.T) {
	first, err := fingerprintRenderedWorkloadService([]byte(`{"image":"example/api","environment":{"APP_SECRET":"old-value"},"ports":["8080:8080"]}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := fingerprintRenderedWorkloadService([]byte(`{"ports":["8080:8080"],"environment":{"APP_SECRET":"new-value"},"image":"example/api"}`))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("effective environment change did not change workload configuration fingerprint")
	}

	reordered, err := fingerprintRenderedWorkloadService([]byte(`{"environment":{"APP_SECRET":"old-value"},"ports":["8080:8080"],"image":"example/api"}`))
	if err != nil {
		t.Fatal(err)
	}
	if first != reordered {
		t.Fatal("JSON object key order changed workload configuration fingerprint")
	}
}

func TestChangedRepositoryWorkloadConfigServicesIsSelective(t *testing.T) {
	previous := repositoryWorkloadConfigState{
		Version:  1,
		Services: map[string]string{"api": "same", "worker": "old"},
	}
	got := changedRepositoryWorkloadConfigServices(map[string]string{
		"api":    "same",
		"worker": "new",
	}, previous)
	if len(got) != 1 || got[0] != "worker" {
		t.Fatalf("changed services = %v, want [worker]", got)
	}
}

func TestPersistRepositoryWorkloadConfigStateContainsOnlyDigests(t *testing.T) {
	files := application.RuntimeFiles{Dir: t.TempDir()}
	secret := "dont-persist-this-secret"
	digest, err := fingerprintRenderedWorkloadService([]byte(`{"environment":{"APP_SECRET":"` + secret + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := persistRepositoryWorkloadConfigState(files, map[string]string{"api": digest}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(repositoryWorkloadConfigStatePath(files))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatal("workload configuration state persisted a secret value")
	}
	got, err := loadRepositoryWorkloadConfigState(files)
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["api"] != digest {
		t.Fatalf("loaded config digest = %q, want %q", got.Services["api"], digest)
	}
	info, err := os.Stat(repositoryWorkloadConfigStatePath(files))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config state mode = %o, want 600", info.Mode().Perm())
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

func TestFailedBuildCandidateInvalidatesVerifiedFingerprintState(t *testing.T) {
	files := application.RuntimeFiles{Dir: t.TempDir()}
	if err := persistRepositoryWorkloadBuildState(files, map[string]string{"api": "verified-good"}); err != nil {
		t.Fatal(err)
	}
	execution := repositoryWorkloadExecution{
		files:        files,
		buildChanged: map[string]struct{}{"api": {}},
	}
	if err := execution.invalidateFailedBuildCandidate(); err != nil {
		t.Fatal(err)
	}
	state, err := loadRepositoryWorkloadBuildState(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Services) != 0 {
		t.Fatalf("failed candidate left source fingerprint state: %#v", state.Services)
	}
}

func TestSanitizeWorkloadDiagnosticRedactsInjectedValues(t *testing.T) {
	const secret = "super-secret-value"
	got := sanitizeWorkloadDiagnostic("panic: token="+secret+"\n", map[string]string{"APP_SECRET": secret})
	if strings.Contains(got, secret) {
		t.Fatalf("diagnostic leaked injected environment value: %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Fatalf("diagnostic did not mark redaction: %q", got)
	}
}
