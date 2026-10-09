package main

import (
	"context"
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
