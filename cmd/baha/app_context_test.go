package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureRepositoryComposeEnvironmentUsesRepositoryDotEnv(t *testing.T) {
	t.Setenv("COMPOSE_ENV_FILES", "")
	repo := t.TempDir()
	envPath := filepath.Join(repo, ".env")
	if err := os.WriteFile(envPath, []byte("HTTP_PORT=8088\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := configureRepositoryComposeEnvironment(repo); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("COMPOSE_ENV_FILES"); got != envPath {
		t.Fatalf("COMPOSE_ENV_FILES=%q, want %q", got, envPath)
	}
}

func TestConfigureRepositoryComposeEnvironmentPreservesExplicitOverride(t *testing.T) {
	t.Setenv("COMPOSE_ENV_FILES", "/tmp/custom.env")
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("HTTP_PORT=8088\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := configureRepositoryComposeEnvironment(repo); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("COMPOSE_ENV_FILES"); got != "/tmp/custom.env" {
		t.Fatalf("COMPOSE_ENV_FILES=%q, want explicit override", got)
	}
}

func TestRepositoryManifestAllowsGroupWriteButRejectsWorldWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseharbor.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	if err := checkManifestPermissions(path, true); err != nil {
		t.Fatalf("0664 repository manifest should be accepted: %v", err)
	}

	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := checkManifestPermissions(path, true); err == nil {
		t.Fatal("0666 repository manifest should be rejected")
	}
}
