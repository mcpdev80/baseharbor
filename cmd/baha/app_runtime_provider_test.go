package main

import (
	"os"
	"path/filepath"
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

func TestRuntimeProviderKindForRepositoryAcceptsKubernetes(t *testing.T) {
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
	got, err := runtimeProviderKindForApplication(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if got != bhruntime.ProviderKubernetes {
		t.Fatalf("provider = %q, want %q", got, bhruntime.ProviderKubernetes)
	}
}
