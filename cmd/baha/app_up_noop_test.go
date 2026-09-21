package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestDecideRepositoryUp(t *testing.T) {
	tests := []struct {
		state               string
		runtimeDefinitionOK bool
		fingerprintMatch    bool
		want                repositoryUpDecision
	}{
		{"running", true, true, repositoryUpNoop},
		{"running", true, false, repositoryUpApply},
		{"running", false, true, repositoryUpApply},
		{"stopped", true, true, repositoryUpStart},
		{"stopped", true, false, repositoryUpApply},
		{"not_applied", true, true, repositoryUpApply},
	}
	for _, tt := range tests {
		if got := decideRepositoryUp(tt.state, tt.runtimeDefinitionOK, tt.fingerprintMatch); got != tt.want {
			t.Fatalf("decideRepositoryUp(%q, %v, %v) = %q, want %q", tt.state, tt.runtimeDefinitionOK, tt.fingerprintMatch, got, tt.want)
		}
	}
}

func TestRepositoryDesiredStateFingerprintChangesWithRepositorySource(t *testing.T) {
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\napp:\n  name: demo\n  environment: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(repo, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Manifest:       application.New("demo", "dev", false, false, false),
		ManifestPath:   manifestPath,
		FromRepository: true,
	}
	first, err := repositoryDesiredStateFingerprint(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := repositoryDesiredStateFingerprint(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("repository source change did not change desired-state fingerprint")
	}
}

func TestRepositoryDesiredStateFingerprintChangesWithDeploymentState(t *testing.T) {
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\napp:\n  name: demo\n  environment: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Manifest:       application.New("demo", "dev", false, false, false),
		ManifestPath:   manifestPath,
		FromRepository: true,
	}
	if err := updateRepositoryInitValues(repo, map[string]string{"HTTP_PORT": "8080"}); err != nil {
		t.Fatal(err)
	}
	first, err := repositoryDesiredStateFingerprint(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := updateRepositoryInitValues(repo, map[string]string{"HTTP_PORT": "8081"}); err != nil {
		t.Fatal(err)
	}
	second, err := repositoryDesiredStateFingerprint(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("deployment-state change did not change desired-state fingerprint")
	}
}

func TestRecordRepositoryAppliedFingerprintRoundTrip(t *testing.T) {
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\napp:\n  name: demo\n  environment: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Manifest:       application.New("demo", "dev", false, false, false),
		ManifestPath:   manifestPath,
		FromRepository: true,
	}
	files := application.RuntimeFiles{Dir: filepath.Join(repo, ".baseharbor", "runtime")}
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := recordRepositoryAppliedFingerprint(context.Background(), resolved, files); err != nil {
		t.Fatal(err)
	}
	match, err := repositoryFingerprintMatches(context.Background(), resolved, files)
	if err != nil {
		t.Fatal(err)
	}
	if !match {
		t.Fatal("recorded desired-state fingerprint did not match")
	}
}
