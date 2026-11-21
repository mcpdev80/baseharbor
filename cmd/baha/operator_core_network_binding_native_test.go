package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// A Compose owner label is user controlled. Core must refuse to adopt a
// same-name native network unless protected Core/shared-owner inventory proves
// ownership.
func TestCoreDefaultNetworkBindingRejectsUnownedNativeNetwork(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_FOREIGN_NETWORK_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native Docker or rootless Podman acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(t.Context()), 3*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	kind := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if kind != "docker" && kind != "podman" {
		t.Fatal("explicit native Docker or Podman runtime required")
	}
	targetName := fmt.Sprintf("foreign-network-%d", time.Now().UnixNano())
	// Configure an isolated Target directly. The assertion is about native
	// resource ownership and must not depend on a separate authenticated
	// target.create command.
	target := configureIsolatedEmptyRuntimeTarget(t, targetName, kind)
	files := bhruntime.Files{Project: targetRuntimeProjectName(target), ResourceProject: bhruntime.SharedResourceProjectName(target.Name)}
	network := bhruntime.ControlPlaneNetworkName(files.ResourceProject)
	labels := []string{"--label", "com.docker.compose.project=" + files.Project}
	if kind == "podman" {
		labels = []string{"--label", "io.podman.compose.project=" + files.Project}
	}
	args := append([]string{"network", "create"}, labels...)
	args = append(args, network)
	if output, err := exec.CommandContext(ctx, kind, args...).CombinedOutput(); err != nil {
		t.Fatalf("create same-name native network: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if output, err := exec.CommandContext(cleanup, kind, "network", "rm", network).CombinedOutput(); err != nil {
			t.Errorf("remove isolated foreign network: %v\n%s", err, output)
		}
	})
	bound, err := prepareCoreDefaultNetworkBinding(ctx, target, files)
	if err == nil || !strings.Contains(err.Error(), "existing native network lacks a protected Core or shared backend owner") {
		t.Fatalf("unowned native network must be refused with protected-owner diagnostic; got files=%+v err=%v", bound, err)
	}
	if bound.DefaultNetworkExternal {
		t.Fatal("unowned native network must not be adopted as external")
	}
}
