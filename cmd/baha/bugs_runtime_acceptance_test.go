package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	dockerprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/docker"
	podmanprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/podman"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Isolated fixtures only; no shared runner targets or user resource names.
func TestRepositoryVolumeDestroyRuntimeAcceptance(t *testing.T) {
	engine := os.Getenv("BASEHARBOR_BUGFIX_RUNTIME")
	if engine == "" {
		t.Skip("opt-in isolated Docker/Podman gate")
	}
	if engine != "docker" && engine != "podman" {
		t.Fatal("invalid engine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	path, err := exec.LookPath(engine)
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v: %s", engine, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Logf("engine: %s", run("version", "--format", "{{.Client.Version}}"))
	project := fmt.Sprintf("bh-bug854-%d", time.Now().UnixNano())
	other := project + "-other"
	names := []string{project + "_state", project + "_shared", project + "_recovery", project + "_orphan", project + "_foreign"}
	t.Cleanup(func() {
		for _, name := range []string{project + "-app", other + "-app"} {
			_ = exec.Command(path, "container", "rm", "-f", name).Run()
		}
		for _, name := range names {
			_ = exec.Command(path, "volume", "rm", name).Run()
		}
	})
	for i, name := range names {
		args := []string{"volume", "create"}
		owner := project
		if i == 4 {
			owner = other
		}
		args = append(args, "--label", "com.docker.compose.project="+owner, "--label", "io.podman.compose.project="+owner)
		if i == 2 {
			args = append(args, "--label", "io.baseharbor.recovery=true")
		}
		args = append(args, name)
		run(args...)
	}
	image := "docker.io/library/alpine:3.23"
	run("pull", image)
	args := []string{"run", "-d", "--name", project + "-app", "--label", "com.docker.compose.project=" + project, "--label", "io.podman.compose.project=" + project, "--label", "com.docker.compose.service=app"}
	for i := 0; i < 3; i++ {
		args = append(args, "-v", names[i]+fmt.Sprintf(":/data%d", i))
	}
	args = append(args, "-v", names[4]+":/external", image, "sleep", "300")
	run(args...)
	run("run", "-d", "--name", other+"-app", "--label", "com.docker.compose.project="+other, "--label", "io.podman.compose.project="+other, "--label", "com.docker.compose.service=other", "-v", names[1]+":/shared", image, "sleep", "300")
	run("exec", project+"-app", "sh", "-c", "echo retained-app-data > /data0/marker")
	backend := bhruntime.NewCLIBackend(path)
	var provider bhruntime.RuntimeProvider = &dockerprovider.Provider{Compose: backend}
	if engine == "podman" {
		provider = &podmanprovider.Provider{Compose: backend}
	}
	if err := provider.DestroyOwnedProjectResources(ctx, project, []bhruntime.ProjectResource{{Kind: "volume", Name: names[4]}}); err == nil {
		t.Fatal("foreign volume deletion was authorized")
	}
	volumes, err := backend.InventoryRepositoryVolumes(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(volumes) != 5 {
		t.Fatalf("incomplete live inventory: %+v", volumes)
	}
	var out bytes.Buffer
	m := application.Manifest{Name: "demo", Environment: "dev"}
	execution := applicationDestroyExecution{manifest: m, files: application.RuntimeFiles{Project: project}, runtimeErr: application.ErrRuntimeNotApplied, compose: provider, repositoryVolumes: volumes, term: cli.NewTerminal(ctx, &out, &out), out: &out, confirmed: true,
		resolved: resolvedApplication{Manifest: m, FromRepository: true, TargetStateRoot: t.TempDir(), Store: application.Store{Root: t.TempDir()}}}
	execution.resolved.Target.RuntimeProvider = engine
	if err := execution.renderDeletePlan(); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if !strings.Contains(out.String(), "PRESERVED volume "+name) {
			t.Fatalf("missing plan resource %s: %s", name, out.String())
		}
	}
	if err := execution.destroyRuntimeResources(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		exists, err := backend.ContainerVolumeExists(ctx, name)
		if err != nil || !exists {
			t.Fatalf("preserved volume lost: %s %v", name, err)
		}
	}
	if !strings.Contains(run("container", "ls", "-a", "--format", "{{.Names}}"), other+"-app") {
		t.Fatal("foreign consumer removed")
	}
	if strings.Contains(run("container", "ls", "-a", "--format", "{{.Names}}"), project+"-app") {
		t.Fatal("app container retained")
	}
	if marker := run("run", "--rm", "-v", names[0]+":/data:ro", image, "cat", "/data/marker"); marker != "retained-app-data" {
		t.Fatalf("data marker lost: %q", marker)
	}
	// Real second destroy must succeed and project-labelled data remains inventoried.
	if err := execution.destroyRuntimeResources(ctx); err != nil {
		t.Fatal(err)
	}
	previous, err := execution.previousRepositoryVolumeNames()
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := backend.InventoryRepositoryVolumes(ctx, project, previous...)
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated) != 5 {
		t.Fatalf("orphan volumes not reported on repeat: %+v", repeated)
	}
	t.Logf("PASS: owned workload removed twice; all five named volumes and foreign consumer preserved; all five retained volumes inventoried after repeated destroy")
	t.Log(out.String())
}

func TestTrustStatusPostDestroyRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_BUGFIX_RUNTIME") == "" {
		t.Skip("opt-in isolated gate")
	}
	// Control-Plane runtime definitions disappear on destroy. Status must remain
	// read-only, independent of an issuer/engine and valid in either working directory.
	TestTrustCoreStateBeforeAfterDestroyAndIncomplete(t)
	TestTrustStatusAbsentCoreInsideAndOutsideRepository(t)
	TestTrustStatusRetainsOwnedRecordsWithoutCore(t)
	t.Log("PASS: installed/incomplete/destroyed state, both repository contexts, human/plain/JSON and retained host ownership")
}
