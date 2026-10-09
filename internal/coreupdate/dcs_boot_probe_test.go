package coreupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	"github.com/mcpdev80/baseharbor/internal/dcspki"
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

func TestDCSBootProbeNativeStatusWithoutIsLeader(t *testing.T) {
	root := t.TempDir()
	pki, err := dcspki.Ensure(filepath.Join(root, "pki"), []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		contradictory bool
	}{
		{"native-status", false}, {"contradictory-helper-flag", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var script strings.Builder
			script.WriteString("#!/bin/sh\ncase \"$1\" in\n")
			endpoints := []string{"https://one:2379", "https://two:2379", "https://three:2379"}
			for i, endpoint := range endpoints {
				status := map[string]any{"header": map[string]any{"cluster_id": 456, "member_id": i + 1, "revision": 100}, "leader": 1, "version": "3.7.2"}
				if tc.contradictory {
					status["isLeader"] = false
				}
				data, err := json.Marshal([]map[string]any{{"Endpoint": endpoint, "Status": status}})
				if err != nil {
					t.Fatal(err)
				}
				fmt.Fprintf(&script, "--endpoints=%s) printf '%%s\\n' '%s' ;;\n", endpoint, data)
			}
			script.WriteString("*) exit 1 ;;\nesac\n")
			binary := filepath.Join(t.TempDir(), "etcdctl")
			if err := os.WriteFile(binary, []byte(script.String()), 0700); err != nil {
				t.Fatal(err)
			}
			proof := 0
			probe := EtcdBootProbe{Binary: binary, Endpoints: endpoints, TLS: etcdbackup.TLSFiles{CA: pki.CA, Cert: pki.ClientCert, Key: pki.ClientKey}, VerifyPatroniDCS: func(context.Context) error { proof++; return nil }}
			err := probe.Verify(context.Background(), etcdbackup.Identity{Core: "core", Target: "target", Cluster: "123"}, etcdbackup.SnapshotInfo{ClusterID: "123", Revision: 100, Version: "3.7.2"})
			if tc.contradictory {
				if err == nil || proof != 0 {
					t.Fatalf("contradictory status accepted: %v proof=%d", err, proof)
				}
			} else if err != nil || proof != 1 {
				t.Fatalf("native etcd status rejected: %v proof=%d", err, proof)
			}
		})
	}
}

func TestRecoveredEtcdClusterUsesFreshQuorumIdentity(t *testing.T) {
	var observed string
	const source = "9223372036854775808"
	const recovered = "18446744073709551615"
	if err := validateRecoveredEtcdClusterID(source, recovered, &observed); err != nil {
		t.Fatal(err)
	}
	if observed != recovered {
		t.Fatalf("wrong recovered cluster ID %s", observed)
	}
	if err := validateRecoveredEtcdClusterID(source, recovered, &observed); err != nil {
		t.Fatal(err)
	}
	for _, foreign := range []string{source, "0", "invalid", "18446744073709551616", "18446744073709551614"} {
		if err := validateRecoveredEtcdClusterID(source, foreign, &observed); err == nil {
			t.Fatalf("invalid recovered cluster %s accepted", foreign)
		}
	}
}
func TestRecoveredEtcdMemberIDsAreUniqueUint64(t *testing.T) {
	seen := map[string]bool{}
	if err := validateRecoveredEtcdMemberID("18446744073709551615", seen); err != nil {
		t.Fatal(err)
	}
	if err := validateRecoveredEtcdMemberID("9223372036854775809", seen); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"18446744073709551615", "0", "", "-1", "18446744073709551616"} {
		if err := validateRecoveredEtcdMemberID(bad, seen); err == nil {
			t.Fatalf("invalid etcd member %s accepted", bad)
		}
	}
}
