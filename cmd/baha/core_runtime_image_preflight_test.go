package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestRuntimeUpEnsuresRuntimeImageBeforeStartingCore(t *testing.T) {
	target := configureTestTarget(t)
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "oras"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	called := false
	want := errors.New("runtime image preflight test stop")
	previous := ensureCoreRuntimeImageForTarget
	ensureCoreRuntimeImageForTarget = func(_ context.Context, got deployment.ResolvedTarget, _ io.Writer) error {
		called = true
		if got.Name != target.Name {
			t.Fatalf("preflight target = %q, want %q", got.Name, target.Name)
		}
		return want
	}
	t.Cleanup(func() { ensureCoreRuntimeImageForTarget = previous })

	err := runtimeUpGuided(context.Background(), strings.NewReader(""), &bytes.Buffer{}, runtimeUpOptions{Yes: true, ControlPlaneOnly: true})
	if !errors.Is(err, want) || !called {
		t.Fatalf("baha up did not stop at runtime image preflight: called=%t err=%v", called, err)
	}
}

func TestCoreUpPreflightPullsResolvedRuntimeImage(t *testing.T) {
	const image = "ghcr.io/mcpdev80/baseharbor-runtime:0.4.27"
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", image)
	compose := &registryPullRecordingCompose{}
	exec := &resolveRegistrySourceExec{}
	previous := registrySourceExec
	registrySourceExec = exec.fn
	t.Cleanup(func() { registrySourceExec = previous })

	var output bytes.Buffer
	if err := pullCoreRuntimeImage(context.Background(), compose, &output); err != nil {
		t.Fatalf("preflight pull failed: %v", err)
	}
	want := "ghcr.io/mcpdev80/baseharbor-runtime@sha256:" + strings.Repeat("a", 64)
	if len(compose.pulled) != 1 || compose.pulled[0] != want {
		t.Fatalf("preflight pulls = %v, want one immutable pull of %q", compose.pulled, want)
	}
	if len(exec.calls) != 1 || !strings.Contains(exec.calls[0], image) {
		t.Fatalf("preflight did not resolve configured version %q: %v", image, exec.calls)
	}
	if !strings.Contains(output.String(), "available") {
		t.Fatalf("preflight did not report image readiness: %s", output.String())
	}
}

func TestCoreUpPreflightFailsClosedWhenRuntimeImageCannotBeResolved(t *testing.T) {
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "registry.example.invalid/baseharbor/runtime:0.4.27")
	installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
		return "unverifiable", nil
	})
	compose := &registryPullRecordingCompose{}
	if err := pullCoreRuntimeImage(context.Background(), compose, &bytes.Buffer{}); err == nil {
		t.Fatal("preflight accepted an unverified runtime image")
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("preflight pulled an unverified image: %v", compose.pulled)
	}
}

var _ bhruntime.RuntimeProvider = (*registryPullRecordingCompose)(nil)
