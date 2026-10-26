package etcd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEtcdutlRequiresOwnedBinaryAndExplicitTopology(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "etcdutl")
	if err := os.WriteFile(path, []byte("not executed"), 0700); err != nil {
		t.Fatal(err)
	}
	config := IsolatedEtcdutl{Binary: path, MemberName: "node1", ExpectedMemberNames: []string{"node1", "node2", "node3"}, InitialCluster: "node1=https://n1:2380,node2=https://n2:2380,node3=https://n3:2380", InitialAdvertisePeerURLs: "https://n1:2380"}
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	config.InitialCluster = "node1=http://n1:2380,node2=https://n2:2380,node3=https://n3:2380"
	if err := config.validate(); err == nil {
		t.Fatal("unencrypted peer permitted")
	}
	config.InitialCluster = "node1=https://n1:2380,node2=https://n2:2380,node3=https://n3:2380"
	config.MemberName = "foreign"
	if err := config.validate(); err == nil {
		t.Fatal("foreign member permitted")
	}
	config.MemberName = "node1"
	if err := os.Chmod(path, 0777); err != nil {
		t.Fatal(err)
	}
	if err := config.validate(); err == nil {
		t.Fatal("world-writable etcdutl permitted")
	}
}
func TestEtcdutlRejectsExistingRestoreTargetWithoutExecution(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "etcdutl")
	if err := os.WriteFile(binary, []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	e := IsolatedEtcdutl{Binary: binary, MemberName: "node1", ExpectedMemberNames: []string{"node1"}, InitialCluster: "node1=https://n1:2380", InitialAdvertisePeerURLs: "https://n1:2380"}
	archive := filepath.Join(dir, "snapshot")
	if err := os.WriteFile(archive, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "existing")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	id := Identity{Core: "core1", Target: "target1", Cluster: "cluster1"}
	info := SnapshotInfo{ClusterID: id.Cluster, Version: "3.6.0", Revision: 1}
	if err := e.RestoreIsolated(context.Background(), archive, dest, id, info); err == nil {
		t.Fatal("existing destination accepted")
	}
}
