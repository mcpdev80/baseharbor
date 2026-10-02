package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
)

func TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile(t *testing.T) {
	if os.Getenv("BASEHARBOR_RUNTIME_RESTART_ACCEPTANCE") != "true" {
		t.Skip("real control-plane restart acceptance requires BASEHARBOR_RUNTIME_RESTART_ACCEPTANCE=true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	runtimeCommand := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if runtimeCommand == "" {
		runtimeCommand = "docker"
	}
	if runtimeCommand == "podman" {
		var targetOut bytes.Buffer
		if err := runWithIO(ctx, []string{"target", "create", "podman-restart-ci", "--provider", "podman", "--access", "podman-restart-ci", "--reference", "local", "--default"}, &targetOut, &targetOut); err != nil {
			t.Fatalf("create Podman restart target: %v\n%s", err, targetOut.String())
		}
	}
	if output, err := exec.CommandContext(
		ctx,
		runtimeCommand, "ps", "-a",
		"--filter", "label=com.docker.compose.project=baseharbor",
		"--format", "{{.ID}}",
	).Output(); err == nil && strings.TrimSpace(string(output)) != "" {
		t.Skip("existing global BaseHarbor Compose project detected; restart acceptance requires an isolated host")
	}

	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	postgresPort, err := selectControlPlanePort(&bytes.Buffer{}, "PostgreSQL", "--postgres-port", 0, bhruntime.DefaultPostgresPort, 15432)
	if err != nil {
		t.Fatal(err)
	}
	openBaoPort, err := selectControlPlanePort(&bytes.Buffer{}, "OpenBao", "--openbao-port", 0, bhruntime.DefaultOpenBaoPort, 18200)
	if err != nil {
		t.Fatal(err)
	}
	if postgresPort == openBaoPort {
		t.Fatal("test selected identical control-plane ports")
	}

	var out bytes.Buffer
	if err := runtimeUpWithPorts(ctx, &out, bhruntime.Ports{Postgres: postgresPort, OpenBao: openBaoPort}); err != nil {
		t.Fatalf("initial control-plane start: %v\n%s", err, out.String())
	}
	runtimeFiles, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"postgres", "openbao"} {
		if err := containersecurity.VerifyComposeService(ctx, runtimeFiles.Project, service, containersecurity.Requirements{
			ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true,
		}); err != nil {
			t.Fatalf("%s runtime security: %v", service, err)
		}
	}

	compose, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	files := runtimeFiles
	recovery := filepath.Join(t.TempDir(), "openbao-recovery.json")
	if err := platformopenbao.Bootstrap(ctx, compose, files, recovery); err != nil {
		t.Fatalf("bootstrap OpenBao: %v", err)
	}

	defer func() {
		_ = runtimeDestroy(context.Background(), []string{"--yes"}, &bytes.Buffer{})
		_ = os.Remove(recovery)
	}()

	out.Reset()
	if err := runtimeDown(ctx, &out); err != nil {
		t.Fatalf("control-plane down: %v\n%s", err, out.String())
	}

	if err := persistTargetRecoveryFileReference(ctx, recovery); err != nil {
		t.Fatalf("persist target recovery reference: %v", err)
	}

	out.Reset()
	if err := runtimeUpExisting(ctx, &out, ""); err != nil {
		t.Fatalf("restart with persisted target recovery reference: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "OpenBao is sealed; unsealing from the operator recovery file") {
		t.Fatalf("restart did not report automatic shared OpenBao unseal: %q", out.String())
	}
	if !strings.Contains(out.String(), "control-plane runtime started and ready") {
		t.Fatalf("restart output did not report verified readiness: %q", out.String())
	}

	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Initialized || state.Sealed {
		t.Fatalf("OpenBao state after verified restart=%+v", state)
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
		t.Fatalf("manager auth after verified restart: %v", err)
	}

	environment := mustRuntimeEnvForHATest(t, files.Env)
	workdir := filepath.Dir(files.Compose)

	openBaoLeader := mustOpenBaoLeaderService(t, ctx, compose, files)
	if err := compose.StopProjectFilesSelected(ctx, files.Project, workdir, environment, []string{openBaoLeader}, files.Compose); err != nil {
		t.Fatalf("stop OpenBao leader %s: %v", openBaoLeader, err)
	}
	waitForOpenBaoHAAfterFailure(t, ctx, compose, files, "OpenBao leader failure")
	if err := compose.UpProjectFilesSelected(ctx, files.Project, workdir, environment, []string{openBaoLeader}, files.Compose); err != nil {
		t.Fatalf("restart OpenBao leader %s: %v", openBaoLeader, err)
	}
	waitForOpenBaoHAAfterFailure(t, ctx, compose, files, "OpenBao member recovery")

	postgresPrimary := mustPostgresPrimaryService(t, ctx, compose, files)
	if err := compose.StopProjectFilesSelected(ctx, files.Project, workdir, environment, []string{postgresPrimary}, files.Compose); err != nil {
		t.Fatalf("stop PostgreSQL primary %s: %v", postgresPrimary, err)
	}
	waitForPostgresHAAfterFailure(t, ctx, compose, files)
	waitForOpenBaoHAAfterFailure(t, ctx, compose, files, "PostgreSQL primary failure")
	if err := compose.UpProjectFilesSelected(ctx, files.Project, workdir, environment, []string{postgresPrimary}, files.Compose); err != nil {
		t.Fatalf("restart PostgreSQL member %s: %v", postgresPrimary, err)
	}
	waitForPostgresHAAfterFailure(t, ctx, compose, files)
	waitForOpenBaoHAAfterFailure(t, ctx, compose, files, "PostgreSQL member recovery")
}

func mustRuntimeEnvForHATest(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("invalid runtime env line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func mustOpenBaoLeaderService(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files) string {
	t.Helper()
	for ordinal := 1; ordinal <= 3; ordinal++ {
		service := fmt.Sprintf("openbao-member-%d", ordinal)
		out, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, service,
			"sh", "-ec",
			"BAO_ADDR=https://127.0.0.1:8200 BAO_CACERT=/run/baseharbor/openbao/ca.pem bao read -field=is_self sys/leader",
		)
		if err == nil && strings.EqualFold(strings.TrimSpace(out), "true") {
			return service
		}
	}
	t.Fatal("no active OpenBao leader found")
	return ""
}

func mustPostgresPrimaryService(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files) string {
	t.Helper()
	const probe = "import urllib.request,sys;\ntry:\n r=urllib.request.urlopen('http://127.0.0.1:8008/primary', timeout=2); sys.exit(0 if r.status == 200 else 1)\nexcept Exception:\n sys.exit(1)"
	for ordinal := 1; ordinal <= 3; ordinal++ {
		service := fmt.Sprintf("postgres-member-%d", ordinal)
		if _, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, service, "python3", "-c", probe); err == nil {
			return service
		}
	}
	t.Fatal("no Patroni PostgreSQL primary found")
	return ""
}

func waitForOpenBaoHAAfterFailure(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, phase string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		state, err := platformopenbao.Inspect(ctx, runtime, files)
		if err == nil && state.Initialized && !state.Sealed {
			if err = platformopenbao.CheckManager(ctx, runtime, files); err == nil {
				return
			}
		}
		last = err
		time.Sleep(time.Second)
	}
	t.Fatalf("%s did not preserve OpenBao API/manager continuity: %v", phase, last)
}

func waitForPostgresHAAfterFailure(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		_, last = runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, "postgres-admin",
			"sh", "-ec",
			"pg_isready -h postgres -p 5432 -U \"$BASEHARBOR_POSTGRES_USER\" -d \"$BASEHARBOR_POSTGRES_DB\"",
		)
		if last == nil {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("stable PostgreSQL endpoint did not recover after primary failure: %v", last)
}
