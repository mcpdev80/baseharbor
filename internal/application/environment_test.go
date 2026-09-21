package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnvironmentTestManifest(t *testing.T, path, name, environment string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(name, environment, true, false, false)
	if err := os.WriteFile(path, []byte(m.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRepositoryEnvironmentPrefersCompleteEnvironmentManifest(t *testing.T) {
	repo := t.TempDir()
	writeEnvironmentTestManifest(t, filepath.Join(repo, RepositoryManifestName), "demo", "dev")
	writeEnvironmentTestManifest(t, filepath.Join(repo, "envs", "prod", RepositoryManifestName), "demo", "prod")

	selected, err := ResolveRepositoryEnvironment(repo, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if !selected.EnvironmentSpecific || selected.Environment != "prod" {
		t.Fatalf("selection=%+v", selected)
	}
	if selected.RepositoryRoot != repo {
		t.Fatalf("repository root=%q, want %q", selected.RepositoryRoot, repo)
	}
	wantState := filepath.Join(repo, ".baseharbor", "environments", "prod")
	if got := RepositoryEnvironmentStateRoot(selected); got != wantState {
		t.Fatalf("state root=%q, want %q", got, wantState)
	}
}

func TestResolveRepositoryEnvironmentKeepsLegacyRootState(t *testing.T) {
	repo := t.TempDir()
	writeEnvironmentTestManifest(t, filepath.Join(repo, RepositoryManifestName), "demo", "dev")

	selected, err := ResolveRepositoryEnvironment(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if !selected.LegacyState {
		t.Fatal("root manifest without override must keep legacy state")
	}
	if got := RepositoryEnvironmentStateRoot(selected); got != filepath.Join(repo, ".baseharbor") {
		t.Fatalf("state root=%q", got)
	}
}

func TestResolveRepositoryEnvironmentScopesRootOverride(t *testing.T) {
	repo := t.TempDir()
	writeEnvironmentTestManifest(t, filepath.Join(repo, RepositoryManifestName), "demo", "dev")

	selected, err := ResolveRepositoryEnvironment(repo, "test")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Manifest.Environment != "test" || selected.LegacyState {
		t.Fatalf("selection=%+v", selected)
	}
}

func TestResolveRepositoryEnvironmentRequiresSelectionForMultipleEnvironmentManifests(t *testing.T) {
	repo := t.TempDir()
	writeEnvironmentTestManifest(t, filepath.Join(repo, "envs", "dev", RepositoryManifestName), "demo", "dev")
	writeEnvironmentTestManifest(t, filepath.Join(repo, "envs", "prod", RepositoryManifestName), "demo", "prod")

	_, err := ResolveRepositoryEnvironment(repo, "")
	if err == nil || !strings.Contains(err.Error(), "-e/--environment") {
		t.Fatalf("expected deterministic selection error, got %v", err)
	}
}

func TestResolveRepositoryEnvironmentRejectsDirectoryEnvironmentMismatch(t *testing.T) {
	repo := t.TempDir()
	writeEnvironmentTestManifest(t, filepath.Join(repo, "envs", "prod", RepositoryManifestName), "demo", "test")

	_, err := ResolveRepositoryEnvironment(repo, "prod")
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("expected environment mismatch, got %v", err)
	}
}
