package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestDecideRepositoryUp(t *testing.T) {
	tests := []struct {
		state                   string
		runtimeDefinitionOK     bool
		fingerprintMatch        bool
		controlFingerprintMatch bool
		want                    repositoryUpDecision
	}{
		{"running", true, true, true, repositoryUpNoop},
		{"running", true, false, true, repositoryUpWorkloadApply},
		{"running", true, false, false, repositoryUpApply},
		{"running", false, true, true, repositoryUpApply},
		{"stopped", true, true, true, repositoryUpStart},
		{"stopped", true, false, true, repositoryUpApply},
		{"not_applied", true, true, true, repositoryUpApply},
	}
	for _, tt := range tests {
		if got := decideRepositoryUp(tt.state, tt.runtimeDefinitionOK, tt.fingerprintMatch, tt.controlFingerprintMatch); got != tt.want {
			t.Fatalf("decideRepositoryUp(%q, %v, %v, %v) = %q, want %q", tt.state, tt.runtimeDefinitionOK, tt.fingerprintMatch, tt.controlFingerprintMatch, got, tt.want)
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

// Source+image traversal completeness (r27 round 7, finding 3 recipe / w65
// Punkt 4): the desired-state fingerprint must detect a change to a component's
// OWNED image reference, not only to a tracked file. The image reference is part
// of the component source-identity that enters the fingerprint, so an operator
// who retargets the component image must change the candidate digest that binds
// the deployment — the ownership boundary of the component source is enforced.
func TestRepositoryDesiredStateFingerprintDetectsComponentImageRetarget(t *testing.T) {
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	manifest := `version: 1
app:
  name: demo
  environment: dev
components:
  - component: web
    source:
      type: repository
      path: ./web
    image: ghcr.io/example/web:v1
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	webDir := filepath.Join(repo, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<h1>1</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved := resolvedApplication{
		Manifest:       application.New("demo", "dev", false, false, false),
		ManifestPath:   manifestPath,
		FromRepository: true,
	}
	first, err := repositoryDesiredStateFingerprint(context.Background(), resolved)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	// Retarget the owned component image only — no tracked file changes.
	if err := os.WriteFile(manifestPath, []byte(strings.ReplaceAll(manifest, "ghcr.io/example/web:v1", "ghcr.io/example/web:v2")), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := repositoryDesiredStateFingerprint(context.Background(), resolved)
	if err != nil {
		t.Fatalf("fingerprint after retarget: %v", err)
	}
	if first == second {
		t.Fatal("component image retarget did not change desired-state fingerprint; component image ownership not traversed")
	}
}

func TestRepositoryControlStateFingerprintIgnoresSourceButTracksContractAndDeploymentState(t *testing.T) {
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\napp:\n  name: demo\n  environment: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(repo, "main.go")
	if err := os.WriteFile(sourcePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	resolved := resolvedApplication{
		Manifest:            application.New("demo", "dev", false, false, false),
		ManifestPath:        manifestPath,
		DeploymentStateRoot: stateRoot,
		FromRepository:      true,
	}

	first, err := repositoryControlStateFingerprint(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceOnly, err := repositoryControlStateFingerprint(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if first != sourceOnly {
		t.Fatal("application source change unexpectedly changed control-state fingerprint")
	}

	if err := os.WriteFile(manifestPath, []byte("version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  cache:\n    enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contractChanged, err := repositoryControlStateFingerprint(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if sourceOnly == contractChanged {
		t.Fatal("application contract change did not change control-state fingerprint")
	}

	if err := updateRepositoryInitValuesAtStateRoot(stateRoot, map[string]string{"HTTP_PORT": "8080"}); err != nil {
		t.Fatal(err)
	}
	deploymentChanged, err := repositoryControlStateFingerprint(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if contractChanged == deploymentChanged {
		t.Fatal("deployment control-state change did not change control-state fingerprint")
	}
}

func TestRepositoryDesiredStateFingerprintChangesWithDeploymentState(t *testing.T) {
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: 1\napp:\n  name: demo\n  environment: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	resolved := resolvedApplication{
		Manifest:            application.New("demo", "dev", false, false, false),
		ManifestPath:        manifestPath,
		DeploymentStateRoot: stateRoot,
		FromRepository:      true,
	}
	if err := updateRepositoryInitValuesAtStateRoot(stateRoot, map[string]string{"HTTP_PORT": "8080"}); err != nil {
		t.Fatal(err)
	}
	first, err := repositoryDesiredStateFingerprint(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := updateRepositoryInitValuesAtStateRoot(stateRoot, map[string]string{"HTTP_PORT": "8081"}); err != nil {
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
