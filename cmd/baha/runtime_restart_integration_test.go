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

	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/containersecurity"
)

func TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile(t *testing.T) {
	if os.Getenv("BASEHARBOR_RUNTIME_RESTART_ACCEPTANCE") != "true" {
		t.Skip("real control-plane restart acceptance requires BASEHARBOR_RUNTIME_RESTART_ACCEPTANCE=true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runtimeCommand := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if runtimeCommand == "" {
		runtimeCommand = "docker"
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
	for _, service := range []string{"postgres", "openbao"} {
		if err := containersecurity.VerifyComposeService(ctx, "baseharbor", service, containersecurity.Requirements{
			ReadOnlyRootfs: true, DropAllCaps: true, NoNewPrivs: true,
		}); err != nil {
			t.Fatalf("%s runtime security: %v", service, err)
		}
	}

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatal(err)
	}
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		t.Fatal(err)
	}
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

	out.Reset()
	err = runtimeUpExisting(ctx, &out, "")
	if err == nil || !strings.Contains(err.Error(), "initialized but sealed") {
		t.Fatalf("restart without recovery error=%v output=%q", err, out.String())
	}

	out.Reset()
	if err := runtimeUpExisting(ctx, &out, recovery); err != nil {
		t.Fatalf("restart with recovery: %v\n%s", err, out.String())
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
}
