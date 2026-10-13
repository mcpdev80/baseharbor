package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// This bounded gate probes actual inventory before/after one owned member stop
// and restore. It does not repeat the full HA destroy/recovery/login matrix.
func TestProviderObservationNativePartialModule(t *testing.T) {
	if os.Getenv("BASEHARBOR_PROVIDER_OBSERVATION_ACCEPTANCE") != "1" {
		t.Skip("set BASEHARBOR_PROVIDER_OBSERVATION_ACCEPTANCE=1 on an isolated rootless runtime")
	}
	engine, format := "docker", "{{json .SecurityOptions}}"
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		engine, format = "podman", "{{.Host.Security.Rootless}}"
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Minute)
	defer cancel()
	rootless, err := exec.CommandContext(ctx, engine, "info", "--format", format).Output()
	if err != nil || engine == "docker" && !strings.Contains(string(rootless), "rootless") || engine == "podman" && strings.TrimSpace(string(rootless)) != "true" {
		t.Fatal("isolated rootless runtime proof required")
	}
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	if engine == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir())
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	containers, err := runtime.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, container := range containers {
		if container.Project == targetRuntimeProjectName(target) {
			t.Fatal("existing runtime project refused")
		}
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if err := approvedNativeFixtureCleanup(t, cleanup, &bytes.Buffer{}); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	}()
	state, err := installCore(ctx, strings.NewReader(""), &bytes.Buffer{}, runtimeUpOptions{HA: true, Yes: true, ControlPlaneOnly: true, MachineRole: coreinstallation.Development, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")})
	if err != nil || !state.Ready {
		t.Fatalf("fixture Core not ready: %v", err)
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{Target: target, TargetStateRoot: root, Manifest: application.New("observer-probe", "dev", false, false, false)}
	storageID := application.RuntimeStorageDependencyID(target.Name)
	facts := observeApplicationProviderFacts(ctx, resolved)
	if facts[storageID].State != application.ProviderRunning {
		t.Fatalf("initial SQL module not observed running: %+v", facts[storageID])
	}
	members := files.PostgresMembers()
	if len(members) < 3 {
		t.Fatal("fixture lacks three SQL members")
	}
	environment, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	selected := []string{members[len(members)-1]}
	if err := runtime.StopProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), environment, selected, files.Compose); err != nil {
		t.Fatal(err)
	}
	facts = observeApplicationProviderFacts(ctx, resolved)
	if facts[storageID].State != application.ProviderUnverifiable {
		t.Fatalf("partially running actual SQL module falsely fully available: %+v", facts[storageID])
	}
	t.Logf("Actual %s partial module classified unverifiable; declared members=%d; no HA-quorum or authenticated-continuity claim.", engine, len(members))
	if err := runtime.UpProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), environment, selected, files.Compose); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		facts = observeApplicationProviderFacts(ctx, resolved)
		if facts[storageID].State == application.ProviderRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual restored module did not regain complete process/health inventory")
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Log("Actual selected member restored; complete process/health inventory regained; existing authority retained.")
}
