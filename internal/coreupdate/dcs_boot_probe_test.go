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

func TestRecoveredEtcdClusterUsesFreshQuorumIdentity(t *testing.T) {
	var observed string
	const source="9223372036854775808"
	const recovered="18446744073709551615"
	if err:=validateRecoveredEtcdClusterID(source,recovered,&observed);err!=nil{t.Fatal(err)}
	if observed!=recovered{t.Fatalf("wrong recovered cluster ID %s",observed)}
	if err:=validateRecoveredEtcdClusterID(source,recovered,&observed);err!=nil{t.Fatal(err)}
	for _,foreign:=range []string{source,"0","invalid","18446744073709551616","18446744073709551614"}{
		if err:=validateRecoveredEtcdClusterID(source,foreign,&observed);err==nil{t.Fatalf("invalid recovered cluster %s accepted",foreign)}
	}
}
func TestRecoveredEtcdMemberIDsAreUniqueUint64(t *testing.T) {
	seen:=map[string]bool{}
	if err:=validateRecoveredEtcdMemberID("18446744073709551615",seen);err!=nil{t.Fatal(err)}
	if err:=validateRecoveredEtcdMemberID("9223372036854775809",seen);err!=nil{t.Fatal(err)}
	for _,bad:=range []string{"18446744073709551615","0","","-1","18446744073709551616"}{
		if err:=validateRecoveredEtcdMemberID(bad,seen);err==nil{t.Fatalf("invalid etcd member %s accepted",bad)}
	}
}
