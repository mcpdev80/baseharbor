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
