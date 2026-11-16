package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type partialRecoveryBootRuntime struct {
	bhruntime.RuntimeProvider
	started, stopped int
}

func (r *partialRecoveryBootRuntime) UpProject(context.Context, string, string, string) error {
	r.started++
	return errors.New("partial isolated etcd boot")
}
func (r *partialRecoveryBootRuntime) DownProject(context.Context, string, string, string) error {
	r.stopped++
	return nil
}

func TestIsolatedRecoveryPartialBootAlwaysTearsDown(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	files := bhruntime.Files{HA: true, Project: "core", Compose: filepath.Join(root, "core.yaml"), Env: filepath.Join(root, "core.env")}
	recovery := filepath.Join(root, "isolated")
	tools := runtimeEtcdTools{Members: []string{"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3"}}
	rt := &partialRecoveryBootRuntime{}
	err := verifyRuntimeRecoveredEtcdCluster(context.Background(), rt, files, tools, recovery,
		"gcr.io/etcd-development/etcd@sha256:"+strings.Repeat("a", 64),
		"baseharbor-control-postgres", etcdbackup.SnapshotInfo{ClusterID: "123", Version: "3.7.2", Revision: 20})
	if err == nil {
		t.Fatal("partial isolated restore boot accepted")
	}
	if rt.started != 1 || rt.stopped != 1 {
		t.Fatalf("partial isolated etcd members not cleaned up: started=%d stopped=%d", rt.started, rt.stopped)
	}
}
