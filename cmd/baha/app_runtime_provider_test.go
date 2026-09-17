package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRuntimeProviderKindForNamedApplicationDefaultsCompose(t *testing.T) {
	got, err := runtimeProviderKindForApplication(resolvedApplication{})
	if err != nil {
		t.Fatal(err)
	}
	if got != bhruntime.ProviderCompose {
		t.Fatalf("provider = %q, want %q", got, bhruntime.ProviderCompose)
	}
}

func TestRuntimeProviderKindForRepositoryLegacyStateDefaultsCompose(t *testing.T) {
	root := t.TempDir()
	resolved := resolvedApplication{
		ManifestPath:   filepath.Join(root, "baseharbor.yaml"),
		FromRepository: true,
	}
	got, err := runtimeProviderKindForApplication(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if got != bhruntime.ProviderCompose {
		t.Fatalf("provider = %q, want %q", got, bhruntime.ProviderCompose)
	}
}

func TestRuntimeProviderKindForRepositoryRejectsUnavailableProvider(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".baseharbor")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, repositoryInitEnvName), []byte("BASEHARBOR_RUNTIME_PROVIDER=kubernetes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		ManifestPath:   filepath.Join(root, "baseharbor.yaml"),
		FromRepository: true,
	}
	_, err := runtimeProviderKindForApplication(resolved)
	if err == nil {
		t.Fatal("unavailable runtime provider unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "kubernetes") {
		t.Fatalf("error does not identify unavailable provider: %v", err)
	}
}
