package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestPodmanTargetDoesNotRenderDockerEngineDetails(t *testing.T) {
	target := deployment.ResolvedTarget{Name: "podman-only", RuntimeProvider: "podman", AccessProvider: "local"}
	engine, err := inspectTargetDockerEngine(context.Background(), target)
	if err != nil || engine != nil {
		t.Fatalf("Podman target returned Docker engine: %v, %v", engine, err)
	}
	var output bytes.Buffer
	renderDockerEngine(&output, engine)
	if strings.Contains(output.String(), "Docker") {
		t.Fatalf("Podman-only status renders a Docker block: %s", output.String())
	}
}

func TestAppNewDocumentsNonInteractiveExplicitArguments(t *testing.T) {
	cmd := appNewCommand()
	if !strings.Contains(cmd.Usage, "--directory PARENT") {
		t.Fatalf("deterministic non-interactive directory missing from help: %s", cmd.Usage)
	}
	if !strings.Contains(cmd.Long, "non-interactive") ||
		!strings.Contains(cmd.Long, "no-input") {
		t.Fatalf("help must distinguish guided TTY from explicit non-interactive creation: %s", cmd.Long)
	}
}

func TestExistingManifestRoutesInitToSingleRuntimeConfigurationPass(t *testing.T) {
	root := t.TempDir()
	present, err := repositoryInitHasExistingManifest(root)
	if err != nil || present {
		t.Fatalf("empty repository incorrectly contains a manifest: %v, %v", present, err)
	}
	manifest := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(manifest, []byte("schema: baseharbor/v1\\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	present, err = repositoryInitHasExistingManifest(root)
	if err != nil || !present {
		t.Fatalf("existing manifest must bypass contract re-generation: %v, %v", present, err)
	}
}
