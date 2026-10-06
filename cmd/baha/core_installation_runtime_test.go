package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
)

func TestCoreOnlyBootstrapRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_BOOTSTRAP_ACCEPTANCE") != "1" {
		t.Skip("set BASEHARBOR_CORE_BOOTSTRAP_ACCEPTANCE=1 on an isolated Docker/Podman runtime")
	}
	command := "docker"
	format := "{{json .SecurityOptions}}"
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		command, format = "podman", "{{.Host.Security.Rootless}}"
	}
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer probeCancel()
	rootless, err := exec.CommandContext(probeCtx, command, "info", "--format", format).Output()
	if err != nil || (command == "docker" && !strings.Contains(string(rootless), "rootless")) || (command == "podman" && strings.TrimSpace(string(rootless)) != "true") {
		t.Fatal("Core bootstrap acceptance requires a verified rootless runtime host")
	}
	for _, role := range []coreinstallation.MachineRole{coreinstallation.Development, coreinstallation.Deployment} {
		t.Run(string(role), func(t *testing.T) { runCoreOnlyBootstrapRuntime(t, role) })
	}
}

func runCoreOnlyBootstrapRuntime(t *testing.T, role coreinstallation.MachineRole) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	target := configureTestTarget(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
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
	t.Chdir(t.TempDir()) // Deliberately no repository or application contract.
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
			t.Fatal("isolated Core acceptance host required; existing project refused")
		}
	}
	var out bytes.Buffer
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		if err := runtimeDestroy(cleanupCtx, []string{"--yes"}, &bytes.Buffer{}); err != nil {
			t.Errorf("Core cleanup failed: %v", err)
		}
	}()
	sampler := startCoreMemorySampler(ctx, runtime, target.RuntimeProvider, targetRuntimeProjectName(target), bhruntime.SharedProjectName(target.Name), bhruntime.SharedProjectName(target.Name+"-core"))
	defer sampler.stop()
	opts := runtimeUpOptions{Yes: true, ControlPlaneOnly: true, MachineRole: role, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")}
	first, err := installCore(ctx, strings.NewReader(""), &out, opts)
	if err != nil {
		t.Fatalf("Core-only bootstrap failed: %v", err)
	}
	if !first.Ready || first.IdentityIssuer == "" || first.Spec.MachineRole != role {
		t.Fatalf("Core not ready: %+v", first)
	}
	report, err := inspectControlPlane(ctx)
	if err != nil || !report.Ready || report.Installation == nil || report.Installation.ID != first.ID {
		t.Fatalf("independent inspection failed: %+v %v", report, err)
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"postgres", "openbao"} {
		if err := containersecurity.VerifyComposeService(ctx, files.Project, service, containersecurity.Requirements{ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true}); err != nil {
			t.Fatal(err)
		}
	}
	second, err := installCore(ctx, strings.NewReader(""), &out, opts)
	if err != nil || !second.Ready || second.ID != first.ID {
		t.Fatalf("retry changed installation: %+v %v", second, err)
	}
	if err := requireApplicationCore(machineNoninteractiveContext(ctx), strings.NewReader(""), &out); err != nil {
		t.Fatalf("existing Core not reused: %v", err)
	}
	if _, err := os.Stat("baseharbor.yaml"); !os.IsNotExist(err) {
		t.Fatal("Core bootstrap created an Application")
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	// Secret-bearing provider environment files must not leak into human output.
	for _, env := range []string{files.Env, filepath.Join(dataDir, "providers", "keycloak", "shared", "core", "runtime.env")} {
		data, err := os.ReadFile(env)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok && len(value) >= 12 && (strings.Contains(key, "PASSWORD") || strings.Contains(key, "SECRET")) && strings.Contains(out.String(), value) {
				t.Fatal("bootstrap output contains a protected credential")
			}
		}
	}
	public, err := json.Marshal(map[string]any{"installation": first.ID, "role": role, "runtime": target.RuntimeProvider, "capabilities": first.Capabilities, "placement": first.Placement, "ready": true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Core-only verified result: %s", public)
	memory, err := sampler.evidence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Core resource evidence: %s", memory)
}
