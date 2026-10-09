package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
)

func TestCorePlanOnlyAllowsStableApplicationProviders(t *testing.T) {
	base := []coreupdate.Delta{
		{Installed: coreupdate.Realization{Kind: coreupdate.SQL, Scope: "shared", Instance: "postgres"}, Classification: coreupdate.NoChange},
		{Installed: coreupdate.Realization{Kind: coreupdate.Secrets, Scope: "shared", Instance: "openbao"}, Classification: coreupdate.BackupRequired},
		{Installed: coreupdate.Realization{Kind: coreupdate.Identity, Scope: "shared", Instance: "keycloak-1"}, Classification: coreupdate.BackupRequired},
		{Installed: coreupdate.Realization{Kind: coreupdate.SQL, Scope: "shared", Instance: "keycloak-db"}, Classification: coreupdate.NoChange},
		{Installed: coreupdate.Realization{Kind: coreupdate.Secrets, Scope: "application", Instance: "app-openbao"}, Classification: coreupdate.NoChange},
	}
	plan, err := corePlanOnly(coreupdate.Plan{Release: "v0.4.24", Deltas: base})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Deltas) != 4 {
		t.Fatalf("core plan contains %d deltas, want 4", len(plan.Deltas))
	}
	base[4].Classification = coreupdate.BackupRequired
	if _, err := corePlanOnly(coreupdate.Plan{Release: "v0.4.24", Deltas: base}); err == nil {
		t.Fatal("changed application-isolated provider accepted without its own migration")
	}
}

func TestValidateHAProviderPlanBlocksBackingMutation(t *testing.T) {
	allowed := coreupdate.Plan{Deltas: []coreupdate.Delta{
		{Installed: coreupdate.Realization{Kind: coreupdate.Secrets, Instance: "openbao-1"}, Classification: coreupdate.BackupRequired},
		{Installed: coreupdate.Realization{Kind: coreupdate.Identity, Instance: "keycloak-1"}, Classification: coreupdate.MigrationRequired},
		{Installed: coreupdate.Realization{Kind: coreupdate.SQL, Instance: "postgres"}, Classification: coreupdate.NoChange},
	}}
	if err := validateHAProviderPlan(allowed); err != nil {
		t.Fatalf("safe HA provider-only plan rejected: %v", err)
	}
	allowed.Deltas[2].Classification = coreupdate.BackupRequired
	if err := validateHAProviderPlan(allowed); err == nil {
		t.Fatal("HA backing PostgreSQL mutation accepted without explicit rolling migration")
	}
}

func TestRuntimeEtcdStatusParsesIdentityAndLeader(t *testing.T) {
	status, err := parseRuntimeEtcdStatus([]byte(`[{"Endpoint":"https://postgres-etcd-1:2379","Status":{"header":{"cluster_id":1234,"member_id":11,"revision":99},"leader":11,"version":"3.7.2","isLeader":true}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if status.ClusterID != "1234" || status.MemberID != "11" || status.LeaderID != "11" || status.Revision != 99 || !status.IsLeader {
		t.Fatalf("unexpected parsed status: %+v", status)
	}
	if _, err := parseRuntimeEtcdStatus([]byte(`[{"Endpoint":"http://x:2379","Status":{"header":{"cluster_id":1,"member_id":1,"revision":0},"leader":0,"version":""}}]`)); err == nil {
		t.Fatal("incomplete etcd status accepted")
	}
}

func TestRecoveryEtcdComposeHasNoLiveVolumeAndRequiresMTLS(t *testing.T) {
	data, err := recoveryEtcdCompose(
		"gcr.io/etcd-development/etcd@sha256:"+strings.Repeat("a", 64),
		"/tmp/baseharbor-recovery", "/tmp/baseharbor-pki",
		[]string{"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3"},
	)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"--client-cert-auth=true",
		"--peer-client-cert-auth=true",
		"--trusted-ca-file=/run/baseharbor/etcd/ca.pem",
		"postgres-etcd-recovery:",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("isolated recovery compose missing %q", required)
		}
	}
	if strings.Contains(text, "postgres-etcd-data-") || strings.Contains(text, "http://") {
		t.Fatal("isolated recovery compose references live data volume or plaintext transport")
	}
}

func TestRuntimeEtcdSnapshotRequiresSingleConsistentQuorumLeader(t *testing.T) {
	healthy := []runtimeEtcdStatus{
		{Endpoint: "https://postgres-etcd-1:2379", ClusterID: "100", MemberID: "11", LeaderID: "11", Revision: 10, IsLeader: true},
		{Endpoint: "https://postgres-etcd-2:2379", ClusterID: "100", MemberID: "12", LeaderID: "11", Revision: 10},
		{Endpoint: "https://postgres-etcd-3:2379", ClusterID: "100", MemberID: "13", LeaderID: "11", Revision: 10},
	}
	if err := verifyRuntimeEtcdQuorum(healthy); err != nil {
		t.Fatalf("valid three-member etcd quorum rejected: %v", err)
	}
	for name, mutate := range map[string]func([]runtimeEtcdStatus){
		"duplicate-member": func(s []runtimeEtcdStatus) { s[2].MemberID = "12" },
		"split-leader":     func(s []runtimeEtcdStatus) { s[2].LeaderID = "12" },
		"foreign-cluster":  func(s []runtimeEtcdStatus) { s[2].ClusterID = "200" },
		"two-leaders":      func(s []runtimeEtcdStatus) { s[1].IsLeader = true },
		"missing-leader":   func(s []runtimeEtcdStatus) { s[0].IsLeader = false },
		"unknown-leader": func(s []runtimeEtcdStatus) {
			for i := range s {
				s[i].LeaderID = "99"
			}
			s[0].IsLeader = false
		},
		"zero-member": func(s []runtimeEtcdStatus) { s[1].MemberID = "0" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := append([]runtimeEtcdStatus(nil), healthy...)
			mutate(invalid)
			if err := verifyRuntimeEtcdQuorum(invalid); err == nil {
				t.Fatalf("%s etcd quorum accepted", name)
			}
		})
	}
	if err := verifyRuntimeEtcdQuorum(healthy[:2]); err == nil {
		t.Fatal("two etcd members accepted as complete recovery inventory")
	}
}

func TestRuntimeEtcdStatusRejectsInvalidUnsignedIdentities(t *testing.T) {
	cases := []string{
		`{"header":{"cluster_id":"invalid","member_id":2,"revision":9},"leader":2,"version":"3.7.2"}`,
		`{"header":{"cluster_id":0,"member_id":2,"revision":9},"leader":2,"version":"3.7.2"}`,
		`{"header":{"cluster_id":1,"member_id":-2,"revision":9},"leader":2,"version":"3.7.2"}`,
		`{"header":{"cluster_id":1,"member_id":2,"revision":9},"leader":18446744073709551616,"version":"3.7.2"}`,
	}
	for _, status := range cases {
		if _, err := parseRuntimeEtcdStatus([]byte(`[{"Endpoint":"https://postgres-etcd-1:2379","Status":` + status + `}]`)); err == nil {
			t.Fatalf("invalid etcd numeric identity accepted: %s", status)
		}
	}
	if _, err := parseRuntimeEtcdStatus([]byte(`[{"Endpoint":"https://postgres-etcd-1:2379","Status":{"header":{"cluster_id":18446744073709551615,"member_id":18446744073709551614,"revision":9},"leader":18446744073709551614,"version":"3.7.2"}}]`)); err != nil {
		t.Fatalf("valid large unsigned etcd identity rejected: %v", err)
	}
}
