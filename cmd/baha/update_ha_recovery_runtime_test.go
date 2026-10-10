package main

import (
	"bytes"
	"context"
	"io"
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
	runCoreHARecoveryAcceptance(t, false)
}

func TestCoreHACutoverRollbackRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORE_HA_CUTOVER_ACCEPTANCE") != "1" {
		t.Skip("set BASEHARBOR_CORE_HA_CUTOVER_ACCEPTANCE=1 on an isolated rootless runtime")
	}
	runCoreHARecoveryAcceptance(t, true)
}

func runCoreHARecoveryAcceptance(t *testing.T, cutover bool) {
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
	if cutover {
		if result, err := haRecoveryFixtureSQL(ctx, rt, files, "CREATE TABLE public.baha_ha_recovery_probe (value int NOT NULL); INSERT INTO public.baha_ha_recovery_probe VALUES (1);"); err != nil {
			t.Fatalf("create SQL recovery marker: %v %s", err, result)
		}
	}
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
	if cutover {
		if err := captureCoreHARecoverySource(ctx, rt, files, journal, leader, ev); err != nil {
			t.Fatal(err)
		}
		source, _, err := loadCoreHARecoverySource(journal, "ha-recovery-fixture", target.Name, "v0.4.24")
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(source.PostgresImage, "@")
		pin := coreupdate.BackingPin{Role: "core-ha-postgresql", Version: "18-spilo-4.1-p2", Image: parts[0], Digest: parts[1]}
		rolling := &patroniCoreRollingOps{runtime: rt, files: files, backup: coreupdate.StreamRecoveryPoint{Directory: backupDir, Name: "core-spilo-basebackup"}, journal: coreupdate.PatroniMemberJournal{Path: filepath.Join(journal, "roll-members.json"), Release: "v0.4.24", Installation: "ha-recovery-fixture", Scope: "shared", Desired: coreupdate.Desired{Kind: coreupdate.SQL, Image: pin.Image, Digest: pin.Digest, Version: pin.Version}}}
		if err := coreupdate.StageAndRollPatroniCluster(ctx, rolling, bridge, coreupdate.DCSCheckpoint{Path: filepath.Join(journal, "dcs-recovery.json")}, coreupdate.HAPostgresComposeCheckpoint{Path: files.Compose, Directory: filepath.Join(journal, "roll-compose"), Previous: pin, Desired: pin}, "ha-recovery-fixture", target.Name, ev.Cluster, "v0.4.24", 0); err != nil {
			t.Fatalf("native replica-first roll and switchover: %v", err)
		}
		members, err := rolling.Inspect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		newLeader, replicas, err := coreupdate.VerifyPatroniQuorum(ctx, members, 0)
		if err != nil || newLeader == leader {
			t.Fatalf("real switchover unverified: %s %v", newLeader, err)
		}
		if _, err := haRecoveryFixtureSQL(ctx, rt, files, "UPDATE public.baha_ha_recovery_probe SET value=2;"); err != nil {
			t.Fatal(err)
		}
		var oldVolumes []string
		for _, name := range files.PostgresMembers() {
			volume, err := coreupdate.ResolveOwnedServiceVolume(files.Compose, name, files.Project)
			if err != nil {
				t.Fatal(err)
			}
			oldVolumes = append(oldVolumes, volume)
		}
		env, err := bhruntime.RuntimeEnvironment(files)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.StopProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), env, []string{replicas[0]}, files.Compose); err != nil {
			t.Fatal(err)
		}
		if err := recoverOwnedCoreHA(ctx, rt, files, journal, "ha-recovery-fixture", target.Name, "v0.4.24"); err != nil {
			t.Fatalf("coordinated physical SQL/DCS rollback after member failure: %v", err)
		}
		result, err := haRecoveryFixtureSQL(ctx, rt, files, "SELECT value FROM public.baha_ha_recovery_probe;")
		if err != nil || strings.TrimSpace(result) != "1" {
			t.Fatalf("physical SQL restore did not recover the selected point: %q %v", result, err)
		}
		for _, volume := range oldVolumes {
			owned, err := rt.InspectProjectResource(ctx, files.Project, bhruntime.ProjectResource{Kind: "volume", Name: volume})
			if err != nil || !owned {
				t.Fatalf("displaced original volume was lost: %s %v", volume, err)
			}
		}
		before, err := rt.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := recoverOwnedCoreHA(ctx, rt, files, journal, "ha-recovery-fixture", target.Name, "v0.4.24"); err != nil {
			t.Fatalf("committed recovery replay: %v", err)
		}
		after, err := rt.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]string{}
		for _, c := range before {
			if c.Project == files.Project && haRecoveryMember(c.Service) {
				ids[c.Service] = c.ID
			}
		}
		for _, c := range after {
			if c.Project == files.Project && haRecoveryMember(c.Service) && ids[c.Service] != c.ID {
				t.Fatalf("committed recovery recreated %s", c.Service)
			}
		}
		t.Logf("HA full recovery verified: runtime=%s replica-first roll/switchover=SAME_PIN_PASS failed-replica=INJECTED physical SQL/WAL restore=PASS live DCS cutover=PASS old volumes=PRESERVED committed replay=NO_RECREATE", engine)
		return
	}
	if err := coreupdate.WaitForPatroniQuorum(ctx, ops, leader, 0, 45*time.Second); err != nil {
		t.Fatal(err)
	}
	t.Logf("HA recovery transport verified: runtime=%s PostgreSQL=18 physical extraction=PASS DCS=%s snapshot=%s isolated mTLS/quorum=PASS live cutover=NOT_TESTED", engine, ev.Cluster, ev.SHA256)
}

func haRecoveryFixtureSQL(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, sql string) (string, error) {
	creds, err := bhruntime.LoadControlPlaneCredentials(files)
	if err != nil {
		return "", err
	}
	env, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		return "", err
	}
	const script = "IFS= read -r PGPASSWORD || exit 1\nexport PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem PGCONNECT_TIMEOUT=10\nexec psql -h postgres -U \"$1\" -d postgres -Atq -v ON_ERROR_STOP=1 -c \"$2\""
	var out strings.Builder
	err = rt.RunProjectFilesEnv(ctx, files.Project, filepath.Dir(files.Compose), env, strings.NewReader(creds.PostgresInternalPassword+"\n"), &out, io.Discard, []string{files.Compose}, "exec", "-T", "postgres-admin", "sh", "-ec", script, "--", creds.PostgresInternalUser, sql)
	return out.String(), err
}
