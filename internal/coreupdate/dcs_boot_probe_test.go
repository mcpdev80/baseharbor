package coreupdate

import (
	"context"
	"strings"
	"testing"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
)

func TestDCSBootProbeRequiresThreeEndpointsAndPatroniProof(t *testing.T) {
	identity := etcdbackup.Identity{Core: "core", Target: "target", Cluster: "123"}
	snapshot := etcdbackup.SnapshotInfo{ClusterID: "123", Version: "3.6.0", Revision: 100}
	probe := EtcdBootProbe{Binary: "/usr/bin/etcdctl", Endpoints: []string{"https://one:2379", "https://two:2379", "https://three:2379"}}
	err := probe.Verify(context.Background(), identity, snapshot)
	if err == nil || !strings.Contains(err.Error(), "UNSUPPORTED") {
		t.Fatalf("missing Patroni DCS proof accepted: %v", err)
	}
	probe.VerifyPatroniDCS = func(context.Context) error { return nil }
	probe.Endpoints = probe.Endpoints[:2]
	if err := probe.Verify(context.Background(), identity, snapshot); err == nil {
		t.Fatal("two-member isolated recovery accepted")
	}
	probe.Endpoints = append(probe.Endpoints, "https://three:2379")
	snapshot.ClusterID = "456"
	if err := probe.Verify(context.Background(), identity, snapshot); err == nil {
		t.Fatal("foreign snapshot cluster accepted")
	}
}
