package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

func TestHARecoveryProjectionRetainsOriginalDataAndPinsReplacementMembers(t *testing.T) {
	data, err := os.ReadFile("../../internal/runtime/assets/compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := coreHARecoverySource{PostgresSHA: strings.Repeat("b", 64), PostgresImage: "spilo@sha256:" + strings.Repeat("a", 64), EtcdImage: "etcd@sha256:" + strings.Repeat("c", 64), Evidence: coreupdate.DCSRecoveryEvidence{SHA256: strings.Repeat("d", 64)}}
	projected, volumes, err := projectHARecoveryCompose(data, source, "owned-core", "/private/recovery path", etcdRecoveryIdentity{User: "1001:1001", UserNS: "keep-id"})
	if err != nil {
		t.Fatal(err)
	}
	var original, result map[string]any
	if err := yaml.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(projected, &result); err != nil {
		t.Fatal(err)
	}
	oldServices := original["services"].(map[string]any)
	newServices := result["services"].(map[string]any)
	if !reflect.DeepEqual(oldServices["openbao-member-1"], newServices["openbao-member-1"]) {
		t.Fatal("recovery modified unrelated OpenBao service")
	}
	for name, value := range original["volumes"].(map[string]any) {
		if !reflect.DeepEqual(value, result["volumes"].(map[string]any)[name]) {
			t.Fatal("old data volume declaration was changed")
		}
	}
	if len(volumes) != 3 {
		t.Fatal("recovery replacement volume count mismatch")
	}
	for member, volume := range volumes {
		svc := newServices[member].(map[string]any)
		if svc["image"] != source.PostgresImage || !strings.HasPrefix(volume, "owned-core_postgres-recovered-") {
			t.Fatal("replacement image or volume identity mismatch")
		}
	}
	dcs := newServices["postgres-etcd-1"].(map[string]any)
	if dcs["image"] != source.EtcdImage || dcs["userns_mode"] != "keep-id" || dcs["volumes"].([]any)[0] != "/private/recovery path/postgres-etcd-1:/etcd-data" {
		t.Fatal("restored DCS mount/namespace lost")
	}
	oldServices["postgres-member-4"] = oldServices["postgres-member-1"]
	foreign, _ := yaml.Marshal(original)
	if _, _, err := projectHARecoveryCompose(foreign, source, "owned-core", "/private/recovery", etcdRecoveryIdentity{}); err == nil {
		t.Fatal("unfenced additional HA member admitted")
	}
}

type haFenceTestRuntime struct {
	bhruntime.RuntimeProvider
	containers []bhruntime.RuntimeContainer
	stops      [][]string
	mounts     string
}

func (r *haFenceTestRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return r.containers, nil
}
func (r *haFenceTestRuntime) StopProjectFilesSelected(_ context.Context, project, _ string, _ map[string]string, services []string, _ ...string) error {
	r.stops = append(r.stops, append([]string(nil), services...))
	for i := range r.containers {
		for _, s := range services {
			if r.containers[i].Project == project && r.containers[i].Service == s {
				r.containers[i].Running = false
			}
		}
	}
	return nil
}
func (r *haFenceTestRuntime) DirectOutput(context.Context, ...string) (string, error) {
	return r.mounts, nil
}

func TestNativeHAFenceStopsSQLWritersBeforeDCSAndRejectsForeignLiveData(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(root, "runtime.env")
	if err := os.WriteFile(env, []byte("BASEHARBOR_RUNTIME_HA=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := &haFenceTestRuntime{containers: []bhruntime.RuntimeContainer{
		{ID: "old-pg", Project: "owned", Service: "postgres-member-1", Running: true},
		{ID: "old-dcs", Project: "owned", Service: "postgres-etcd-1", Running: true},
		{ID: "unrelated", Project: "other", Service: "postgres-member-1", Running: true},
	}}
	ops := &nativeHACutover{runtime: rt, files: bhruntime.Files{Project: "owned", Compose: filepath.Join(root, "compose.yaml"), Env: env, HA: true}, journal: root, recoveryDir: "/private/new-dcs", volumes: map[string]string{"postgres-member-1": "owned_recovered-pg"}, tools: runtimeEtcdTools{Members: []string{"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3"}}}
	if err := ops.FenceOldDCS(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.stops) != 2 || rt.stops[0][0] != "postgres-member-1" || rt.stops[1][0] != "postgres-etcd-1" || !rt.containers[2].Running {
		t.Fatal("fencing order or project isolation violated")
	}
	rt.containers[0].Running = true
	if err := ops.VerifyFenced(context.Background()); err == nil {
		t.Fatal("running original member admitted")
	}
	rt.containers[0].ID = "new-pg"
	rt.mounts = `[{"Type":"volume","Name":"owned_old-data","Destination":"/home/postgres/pgdata/pgroot"}]`
	if err := ops.VerifyFenced(context.Background()); err == nil {
		t.Fatal("changed container ID with old data admitted")
	}
	rt.mounts = `[{"Type":"volume","Name":"owned_recovered-pg","Destination":"/home/postgres/pgdata/pgroot"}]`
	if err := ops.VerifyFenced(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHARecoveryRejectsWrongTargetBeforeRuntimeMutation(t *testing.T) {
	root := t.TempDir()
	source := coreHARecoverySource{Evidence: coreupdate.DCSRecoveryEvidence{Installation: "core", Target: "different-target", Release: "0.4.24"}}
	data, _ := json.Marshal(source)
	if err := os.WriteFile(filepath.Join(root, "ha-recovery-source.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := recoverOwnedCoreHA(context.Background(), &haFenceTestRuntime{}, bhruntime.Files{HA: true}, root, "core", "target", "0.4.24"); err == nil || !strings.Contains(err.Error(), "different Core, Target or release") {
		t.Fatalf("unbound recovery source admitted: %v", err)
	}
}

func TestHARecoveryCLIRequiresExplicitBackupPointAndConfirmation(t *testing.T) {
	for _, args := range [][]string{{"--recover"}, {"--recover", "--yes"}, {"--recover", "--version", "0.4.24"}, {"--recover", "--check", "--version", "0.4.24"}, {"--recover", "--yes", "--channel", "stable"}} {
		if _, err := parseSelfUpdateOptions(args); err == nil {
			t.Fatalf("ambiguous recovery admitted: %v", args)
		}
	}
	opts, err := parseSelfUpdateOptions([]string{"--recover", "--version", "0.4.24", "--yes"})
	if err != nil || !opts.Recover {
		t.Fatalf("explicit recovery rejected: %v", err)
	}
}

type haPinnedImageTestRuntime struct {
	bhruntime.RuntimeProvider
	image bhruntime.ImageIdentity
}

func (r haPinnedImageTestRuntime) ProjectServiceImageIdentity(context.Context, string, string) (bhruntime.ImageIdentity, error) {
	return r.image, nil
}

func TestPatroniImageVerificationAcceptsRealDigestPinnedContainerReference(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	rt := haPinnedImageTestRuntime{image: bhruntime.ImageIdentity{Reference: "spilo:4.1-p2@" + digest, Digest: "spilo@" + digest}}
	ops := patroniCoreRollingOps{runtime: rt, files: bhruntime.Files{HA: true}, journal: coreupdate.PatroniMemberJournal{Desired: coreupdate.Desired{Image: "spilo:4.1-p2", Digest: digest}}}
	if err := ops.VerifyMemberImage(context.Background(), "postgres-member-1"); err != nil {
		t.Fatal(err)
	}
	rt.image.Reference = "foreign:4.1-p2@" + digest
	ops.runtime = rt
	if err := ops.VerifyMemberImage(context.Background(), "postgres-member-1"); err == nil {
		t.Fatal("foreign image reference accepted despite matching digest")
	}
}

func TestRecoveredUpdateRetryNeverRenamesActiveDCSDirectory(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "core-updates", "0.4.24")
	if err := os.MkdirAll(filepath.Join(base, "dcs-isolated-restore"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(base, "dcs-isolated-restore", "active-data")
	if err := os.WriteFile(marker, []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "ha-cutover-committed"), []byte("committed"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := coreUpdateJournal(root, "0.4.24", true)
	if err != nil || next != filepath.Join(base, "update-after-recovery") {
		t.Fatalf("new transaction not separated: %s %v", next, err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "active" {
		t.Fatal("active DCS bind directory moved or overwritten")
	}
	replay, err := coreUpdateJournal(root, "0.4.24", false)
	if err != nil || replay != base {
		t.Fatal("committed replay selected an empty subsequent transaction")
	}
	if err := os.WriteFile(filepath.Join(next, "ha-recovery-source.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	replay, err = coreUpdateJournal(root, "0.4.24", false)
	if err != nil || replay != next {
		t.Fatal("latest captured recovery point not selected")
	}
}
