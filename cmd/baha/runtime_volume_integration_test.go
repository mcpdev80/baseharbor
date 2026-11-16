package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
)

func TestOwnedVolumeDestroyPreservesExternalAndSharedData(t *testing.T) {
	if os.Getenv("BASEHARBOR_OWNED_VOLUMES_ACCEPTANCE") != "true" {
		t.Skip("requires isolated Docker/Podman owned-volume acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	engine := os.Getenv("BASEHARBOR_TEST_RUNTIME")
	if engine == "" {
		engine = "docker"
	}
	runtime, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target := deployment.ResolvedTarget{Name: "volume-acceptance"}
	project := bhruntime.SharedProjectName(target.Name)
	unique := fmt.Sprintf("%s-%d", project, time.Now().UnixNano())
	exclusive, shared, foreign := unique+"-exclusive", unique+"-shared", unique+"-foreign"
	external, unrelated := unique+"-external", unique+"-unrelated"
	command := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, engine, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", engine, strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		for _, name := range []string{foreign, exclusive, shared} {
			_ = exec.CommandContext(cleanupCtx, engine, "container", "rm", "-f", "-v", name).Run()
		}
		for _, name := range []string{external, unrelated} {
			_ = exec.CommandContext(cleanupCtx, engine, "volume", "rm", name).Run()
		}
	}()
	command("volume", "create", external)
	command("volume", "create", unrelated)
	command("run", "-d", "--name", exclusive, "--label", "com.docker.compose.project="+project, "--mount", "type=volume,source="+external+",target=/external", "--entrypoint", "sleep", "postgres:18-alpine", "600")
	command("run", "-d", "--name", shared, "--label", "com.docker.compose.project="+project, "--entrypoint", "sleep", "postgres:18-alpine", "600")
	sharedVolume := command("container", "inspect", "--format", `{{range .Mounts}}{{if eq .Type "volume"}}{{if eq .Destination "/var/lib/postgresql"}}{{.Name}}{{end}}{{end}}{{end}}`, shared)
	if sharedVolume == "" {
		t.Fatal("image-declared PostgreSQL volume was not created")
	}
	command("run", "-d", "--name", foreign, "--label", "com.docker.compose.project=foreign-project", "--mount", "type=volume,source="+sharedVolume+",target=/shared", "--entrypoint", "sleep", "alpine:3.23", "600")
	plan, err := collectTargetDestroyInventoryWithRuntime(ctx, target, runtime, true)
	if err != nil {
		t.Fatal(err)
	}
	var exclusiveVolume string
	for _, volume := range plan.Volumes[project] {
		if volume.Removable {
			exclusiveVolume = volume.Name
		}
		if volume.Name == sharedVolume && volume.Removable {
			t.Fatal("shared volume authorized for deletion")
		}
	}
	if exclusiveVolume == "" {
		t.Fatalf("missing concrete owned anonymous volume: %#v", plan.Volumes)
	}
	var results []fullDestroyResult
	plan.cleanup(ctx, &results)
	for _, result := range results {
		if result.Status == "FAILED" {
			t.Fatalf("cleanup: %#v", results)
		}
	}
	inventory := runtime.(destroyVolumeInventory)
	for _, name := range []string{external, unrelated, sharedVolume} {
		exists, err := inventory.ContainerVolumeExists(ctx, name)
		if err != nil || !exists {
			t.Fatalf("protected volume %s deleted: %v", name, err)
		}
	}
	if exists, err := inventory.ContainerVolumeExists(ctx, exclusiveVolume); err != nil || exists {
		t.Fatalf("owned anonymous volume remains: %s %v", exclusiveVolume, err)
	}
	if command("container", "inspect", "--format", "{{.State.Running}}", foreign) != "true" {
		t.Fatal("foreign container changed")
	}
	command("container", "rm", "-f", "-v", foreign)
	if exists, err := inventory.ContainerVolumeExists(ctx, sharedVolume); err != nil {
		t.Fatal(err)
	} else if exists {
		command("volume", "rm", sharedVolume)
	}
}
