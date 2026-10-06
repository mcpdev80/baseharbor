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

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
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
		if !t.Run(string(role), func(t *testing.T) { runCoreOnlyBootstrapRuntime(t, role) }) {
			return
		}
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
	var preservedRecovery string
	if role == coreinstallation.Development {
		preservedRecovery, err = defaultTargetRecoveryFile(target.Name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(preservedRecovery), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(preservedRecovery, []byte("previous installation recovery material\n"), 0600); err != nil {
			t.Fatal(err)
		}
		opts.RecoveryFile = ""
	}
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
	for _, service := range []string{"postgres", "openbao", "postgres-member-1", "openbao-member-1"} {
		if err := containersecurity.VerifyComposeService(ctx, files.Project, service, containersecurity.Requirements{ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true}); err != nil {
			inventory, inventoryErr := runtime.ListRuntimeContainers(ctx)
			for _, container := range inventory {
				t.Logf("Runtime resource: project=%s service=%s running=%t", container.Project, container.Service, container.Running)
			}
			if inventoryErr != nil {
				t.Logf("Runtime inventory unavailable: %v", inventoryErr)
			}
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
	if preservedRecovery != "" {
		data, err := os.ReadFile(preservedRecovery)
		if err != nil || string(data) != "previous installation recovery material\n" {
			t.Fatal("fresh bootstrap changed previous recovery material")
		}
		current, source, err := resolveTargetRecoveryFile(ctx, "")
		if err != nil || current == preservedRecovery || source != "persisted target" {
			t.Fatalf("fresh recovery reference not persisted: %s %s %v", current, source, err)
		}
	}
	if role == coreinstallation.Development && os.Getenv("BASEHARBOR_BUG_HUNT_LIFECYCLE_ACCEPTANCE") == "1" {
		runManagedReadinessAndBackupRegression(t, ctx)
	}
}

func runManagedReadinessAndBackupRegression(t *testing.T, ctx context.Context) {
	t.Helper()
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeShared))
	manifest := application.New("backup-broker-regression", "dev", true, false, false)
	manifest = application.WithWorkloadComponents(manifest, "api")
	manifest = application.WithRuntimePermission(manifest, "object-storage.s3/v1", []string{"api"}, "runtime.create", "runtime.get", "runtime.delete")
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if manifest.Services.Secrets || !application.RequiresRuntimeBroker(manifest) {
		t.Fatal("fixture must require a broker without application secrets")
	}
	if err := os.WriteFile(application.RepositoryManifestName, []byte(manifest.YAML()), 0600); err != nil {
		t.Fatal(err)
	}
	compose := `services:
  api:
    image: docker.io/library/python:3.13-alpine
    command: ["python", "-m", "http.server", "8080"]
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/')"]
      interval: 1s
      timeout: 3s
      retries: 30
`
	if err := os.WriteFile("compose.yaml", []byte(compose), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "apply"}, &output, &output); err != nil {
		t.Fatalf("managed fixture apply: %v\n%s", err, output.String())
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := runWithIO(cleanup, []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	}()
	check := func() {
		status, err := collectApplicationStatus(ctx, application.DefaultStore(), nil)
		if err != nil || !status.Ready {
			t.Fatalf("canonical readiness: %+v %v", status, err)
		}
		resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "bug hunt")
		if err != nil {
			t.Fatal(err)
		}
		overview, err := inspectApplicationOverview(ctx, resolved)
		if err != nil || !overview.Ready {
			t.Fatalf("overview disagrees with READY: %+v %v", overview, err)
		}
		var doctor bytes.Buffer
		if err := runWithIO(ctx, []string{"app", "doctor"}, &doctor, &doctor); err != nil {
			t.Fatalf("doctor after backup: %v\n%s", err, doctor.String())
		}
	}
	check()
	password := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(password, []byte("native-regression-backup-password"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second"} {
		archive := filepath.Join(t.TempDir(), name+".bhbackup")
		output.Reset()
		if err := runWithIO(ctx, []string{"app", "backup", "--password-file", password, "--output", archive}, &output, &output); err != nil {
			t.Fatalf("backup: %v\n%s", err, output.String())
		}
		info, err := os.Stat(archive)
		if err != nil || info.Size() == 0 {
			t.Fatal("backup archive missing")
		}
		check()
	}
	t.Log("Shared provider app show/status/doctor agree; two encrypted backups restore the runtime-permission broker without application secrets")
}
