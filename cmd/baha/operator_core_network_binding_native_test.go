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
	native := func(commandCtx context.Context, args ...string) ([]byte, error) {
		command := exec.CommandContext(commandCtx, kind, args...)
		if kind == "podman" {
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "XDG_CONFIG_HOME=") && !strings.HasPrefix(entry, "XDG_DATA_HOME=") {
					command.Env = append(command.Env, entry)
				}
			}
		}
		return command.CombinedOutput()
	}
	// BaseHarbor's own Podman/Quadlet renderer writes both compatibility
	// project labels. Both remain forgeable and neither is protected owner
	// inventory, so matching both must still be refused.
	labels := []string{
		"--label", "com.docker.compose.project=" + files.Project,
		"--label", "io.podman.compose.project=" + files.Project,
	}
	args := append([]string{"network", "create"}, labels...)
	args = append(args, network)
	if output, err := native(ctx, args...); err != nil {
		t.Fatalf("create same-name native network: %v\n%s", err, output)
	}
	inspectTemplate := `{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}`
	inspected, err := native(ctx, "network", "inspect", "--format", inspectTemplate, network)
	if err != nil {
		t.Fatalf("inspect same-name native network labels: %v\n%s", err, inspected)
	}
	parts := strings.Split(strings.TrimSpace(string(inspected)), "|")
	if len(parts) != 3 || strings.TrimPrefix(strings.TrimSpace(parts[0]), "/") != network ||
		(strings.TrimSpace(parts[1]) != files.Project && strings.TrimSpace(parts[2]) != files.Project) {
		t.Fatalf("native runtime did not retain the forgeable owner labels needed by this refusal test: network=%q labels=%q output=%q", network, labels, strings.TrimSpace(string(inspected)))
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if output, err := native(cleanup, "network", "rm", network); err != nil {
			t.Errorf("remove isolated foreign network: %v\n%s", err, output)
		}
	})
	bound, err := prepareCoreDefaultNetworkBinding(ctx, target, files)
	if err == nil || !strings.Contains(err.Error(), "existing native network lacks a protected Core or shared backend owner") {
		provider, providerErr := detectRuntimeForTarget(ctx, target)
		var owned []bhruntime.ProjectResource
		var inspectErr error
		networks, networkListErr := native(ctx, "network", "ls", "--format", "{{.Name}}")
		if providerErr == nil {
			owned, inspectErr = provider.InspectProjectResources(ctx, files.Project, []bhruntime.ProjectResource{{Kind: "network", Name: network}})
		}
		t.Fatalf("unowned native network must be refused with protected-owner diagnostic; got files=%+v err=%v native-labels=%q network-list=%q network-list-error=%v inventory=%+v inventory-error=%v runtime-error=%v", bound, err, strings.TrimSpace(string(inspected)), strings.TrimSpace(string(networks)), networkListErr, owned, inspectErr, providerErr)
	}
	if bound.DefaultNetworkExternal {
		t.Fatal("unowned native network must not be adopted as external")
	}
}
