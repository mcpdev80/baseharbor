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

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// This gate exercises the production transport and physical extraction, not a
// fake RuntimeProvider or a standalone etcd binary. It deliberately does not
// claim live cutover, PostgreSQL boot/replay or provider rollback acceptance.
func TestCoreHARecoveryRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_HA_RECOVERY_ACCEPTANCE") != "1" {
		t.Skip("set BASEHARBOR_CORE_HA_RECOVERY_ACCEPTANCE=1 on an isolated rootless runtime")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	target := configureTestTarget(t)
	engine, format := "docker", "{{json .SecurityOptions}}"
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		engine, format = "podman", "{{.Host.Security.Rootless}}"
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = engine
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	rootless, err := exec.CommandContext(ctx, engine, "info", "--format", format).Output()
	if err != nil || (engine == "docker" && !strings.Contains(string(rootless), "rootless")) || (engine == "podman" && strings.TrimSpace(string(rootless)) != "true") {
		t.Fatal("HA recovery gate requires verified rootless runtime")
	}
	t.Chdir(t.TempDir())
	rt, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	containers, err := rt.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range containers {
		if c.Project == targetRuntimeProjectName(target) {
			t.Fatal("refusing existing Core project")
		}
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		if t.Failed() {
			logCoreBootstrapFailure(t, rt, target.Name, engine)
		}
		if err := runtimeDestroy(cleanupCtx, []string{"--yes"}, &bytes.Buffer{}); err != nil {
			t.Errorf("HA fixture cleanup: %v", err)
		}
	}()
	pgPort, err := selectControlPlanePort(&bytes.Buffer{}, "PostgreSQL", "--postgres-port", 0, bhruntime.DefaultPostgresPort, 15432)
	if err != nil {
		t.Fatal(err)
	}
	baoPort, err := selectControlPlanePort(&bytes.Buffer{}, "OpenBao", "--openbao-port", 0, bhruntime.DefaultOpenBaoPort, 18200)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeUpWithPorts(ctx, &bytes.Buffer{}, bhruntime.Ports{Postgres: pgPort, OpenBao: baoPort}, true); err != nil {
		t.Fatalf("HA start: %v", err)
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyCoreHADCSSecurity(files); err != nil {
		t.Fatal(err)
	}
	ops := &patroniCoreRollingOps{runtime: rt, files: files}
	members, err := ops.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	leader, _, err := coreupdate.VerifyPatroniQuorum(ctx, members, 0)
	if err != nil {
		t.Fatal(err)
	}
	journal := t.TempDir()
	if err := os.Chmod(journal, 0700); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(journal, "patroni-recovery")
	if err := captureOwnedPatroniBackup(ctx, rt, files, backupDir); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := prepareOwnedPatroniPhysicalRestore(ctx, journal); err != nil {
			t.Fatal(err)
		}
	}
	bridge, err := buildCoreEtcdRecoveryBridge(ctx, rt, files, "ha-recovery-fixture", target.Name, "v0.4.24", journal)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := (coreupdate.DCSCheckpoint{Path: filepath.Join(journal, "dcs-recovery.json")}).Acquire(ctx, bridge, bridge.Store.Identity.Core, target.Name, bridge.Store.Identity.Cluster, "v0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if err := coreupdate.WaitForPatroniQuorum(ctx, ops, leader, 0, 45*time.Second); err != nil {
		t.Fatal(err)
	}
	t.Logf("HA recovery transport verified: runtime=%s PostgreSQL=18 physical extraction=PASS DCS=%s snapshot=%s isolated mTLS/quorum=PASS live cutover=NOT_TESTED", engine, ev.Cluster, ev.SHA256)
}
