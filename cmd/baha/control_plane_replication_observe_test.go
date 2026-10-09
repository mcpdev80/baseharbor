package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCoreReplicationObservationRequiresOwnedNativeHealthRoles(t *testing.T) {
	roles := map[string]string{"postgres-member-1": "primary", "postgres-member-2": "replica", "postgres-member-3": "replica"}
	rt := patroniInspectionRuntime{role: roles}
	if !observePatroniReplication(context.Background(), rt, "owned", "compose", "env", "postgres-member") {
		t.Fatal("native leader and streaming replicas not observed")
	}
	if observePatroniReplication(context.Background(), rt, "owned", "compose", "env", "keycloak-db-member") {
		t.Fatal("foreign SQL cluster supplied identity replication proof")
	}
	roles["postgres-member-3"] = "primary"
	if observePatroniReplication(context.Background(), rt, "owned", "compose", "env", "postgres-member") {
		t.Fatal("local role disagreement supplied replication proof")
	}
	r := controlPlaneAvailability{HA: true, ActualHA: true, PostgresMembers: 3, IdentitySQLMembers: 3}
	if detail := r.Detail(); !strings.Contains(detail, "ha-active=unknown") || !strings.Contains(detail, "replicas=postgresql:unknown") {
		t.Fatal("counts supplied unobserved replication proof")
	}
	r.PostgresReplicationVerified, r.IdentityReplicationVerified = true, true
	if detail := r.Detail(); !strings.Contains(detail, "ha-active=true") || !strings.Contains(detail, "replicas=postgresql:2") || !strings.Contains(detail, "identity-sql-replicas=2") {
		t.Fatal("native replication proof not projected")
	}
}

// Native cluster inventory can name a synchronous or quorum standby while its
// per-member REST health endpoint still identifies it as a replica.
type replicationInventoryRuntime struct {
	patroniInspectionRuntime
	followerRole string
}

func (r replicationInventoryRuntime) ExecProject(ctx context.Context, project, compose, env, service string, argv ...string) (string, error) {
	if len(argv) >= 3 && strings.Contains(argv[2], "/cluster") {
		return fmt.Sprintf(`{"members":[{"name":"postgres-member-1","role":"leader","state":"running"},{"name":"postgres-member-2","role":%q,"state":"streaming","lag":0},{"name":"postgres-member-3","role":%q,"state":"streaming","lag":0}]}`, r.followerRole, r.followerRole), nil
	}
	return r.patroniInspectionRuntime.ExecProject(ctx, project, compose, env, service, argv...)
}
func TestCoreReplicationObservationRecognizesOnlyHealthVerifiedStandbys(t *testing.T) {
	for _, role := range []string{"replica", "sync_standby", "quorum_standby", "unknown", "leader"} {
		rt := replicationInventoryRuntime{patroniInspectionRuntime: patroniInspectionRuntime{role: map[string]string{"postgres-member-1": "primary", "postgres-member-2": "replica", "postgres-member-3": "replica"}}, followerRole: role}
		want := role == "replica" || role == "sync_standby" || role == "quorum_standby"
		if got := observePatroniReplication(context.Background(), rt, "owned", "compose", "env", "postgres-member"); got != want {
			t.Fatalf("role %s proof=%v want=%v", role, got, want)
		}
		if want {
			rt.role["postgres-member-3"] = "primary"
			if observePatroniReplication(context.Background(), rt, "owned", "compose", "env", "postgres-member") {
				t.Fatal("unhealthy standby supplied proof")
			}
		}
	}
}
