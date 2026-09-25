package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestFullDestroyDryRunDoesNotMutateState(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	targetRoot, err := deployment.TargetStateRoot("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(targetRoot, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}

	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{NonInteractive: true})
	var out bytes.Buffer
	if err := runtimeDestroyAll(ctx, []string{"--all"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(targetRoot); err != nil {
		t.Fatalf("dry-run mutated target state: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("No changes were made")) {
		t.Fatalf("expected dry-run guidance, got:\n%s", out.String())
	}
}

func TestFullDestroyRemovesOwnedLocalStateButPreservesSourceRepository(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	targetRoot, err := deployment.TargetStateRoot("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(targetRoot, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}

	sourceRoot := filepath.Join(t.TempDir(), "source-repository")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceMarker := filepath.Join(sourceRoot, "baseharbor.yaml")
	if err := os.WriteFile(sourceMarker, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runtimeDestroyAll(context.Background(), []string{"--all", "--yes"}, &out, &out); err != nil {
		t.Fatal(err)
	}

	dataRoot, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("BaseHarbor data root still exists or stat failed: %v", err)
	}
	configPath, err := deployment.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(configPath)); !os.IsNotExist(err) {
		t.Fatalf("BaseHarbor config root still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(sourceMarker); err != nil {
		t.Fatalf("source repository was modified by full destroy: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Cleanup report")) {
		t.Fatalf("expected cleanup report, got:\n%s", out.String())
	}
}
