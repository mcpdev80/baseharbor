package main

import (
	"context"
	"errors"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"strings"
	"testing"
)

type credentialProfileRuntime struct {
	bhruntime.RuntimeProvider
	inputs []string
}

func (r *credentialProfileRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, _ ...string) (string, error) {
	r.inputs = append(r.inputs, string(input))
	return "1", nil
}

func TestControlPlaneCredentialRotationIncludesReplicationOnlyForHA(t *testing.T) {
	credentials := bhruntime.ControlPlaneCredentials{PostgresUser: "admin", PostgresPassword: "admin-secret", PostgresInternalUser: "internal", PostgresInternalPassword: "internal-secret", PostgresReplicationUser: "replication", PostgresReplicationPass: "replication-secret", OpenBaoDBUser: "bao", OpenBaoDBPassword: "bao-secret"}
	for _, ha := range []bool{false, true} {
		runtime := &credentialProfileRuntime{}
		if err := prepareControlPlaneDatabaseCredentialOverlap(context.Background(), runtime, bhruntime.Files{HA: ha}, credentials, credentials); err != nil {
			t.Fatal(err)
		}
		identities := controlPlaneDatabaseIdentities(credentials, ha)
		want := 3
		if ha {
			want = 4
		}
		if len(identities) != want || len(runtime.inputs) != want+1 {
			t.Fatalf("ha=%t credential identities=%d calls=%d", ha, len(identities), len(runtime.inputs))
		}
		if strings.Contains(runtime.inputs[0], "REPLICATION") != ha || strings.Contains(strings.Join(runtime.inputs, "\n"), "replication-secret") != ha {
			t.Fatal("replication credentials crossed availability boundary")
		}
		for _, role := range []string{"admin", "internal", "bao"} {
			if !strings.Contains(runtime.inputs[0], role) {
				t.Fatalf("required identity missing: %s", role)
			}
		}
	}
}

type isolatedCredentialRuntime struct {
	bhruntime.RuntimeProvider
	sql             string
	replicationUser string
}

func (r *isolatedCredentialRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	_, sql, hasSQL := strings.Cut(string(input), "\n")
	if hasSQL && strings.Contains(sql, "CREATE ROLE") {
		r.sql = sql
		return "", nil
	}
	if len(args) >= 2 && args[len(args)-2] == r.replicationUser && args[len(args)-1] == "postgres" {
		grant := "GRANT CONNECT ON DATABASE " + quoteControlPlaneIdent("postgres") + " TO " + quoteControlPlaneIdent(r.replicationUser) + ";"
		if !strings.Contains(r.sql, grant) {
			return "", errors.New("replacement replication identity lacks CONNECT on hardened system database")
		}
	}
	return "1", nil
}

func TestReplacementReplicationCredentialRespectsSharedDatabaseIsolation(t *testing.T) {
	current := bhruntime.ControlPlaneCredentials{PostgresUser: "admin", PostgresPassword: "admin-secret", PostgresInternalUser: "internal", PostgresInternalPassword: "internal-secret", PostgresReplicationUser: "old-replication", PostgresReplicationPass: "old-secret", OpenBaoDBUser: "bao", OpenBaoDBPassword: "bao-secret"}
	next := current
	next.PostgresReplicationUser = `replacement"replication`
	next.PostgresReplicationPass = "replacement-secret"
	for _, ha := range []bool{false, true} {
		rt := &isolatedCredentialRuntime{replicationUser: next.PostgresReplicationUser}
		if err := prepareControlPlaneDatabaseCredentialOverlap(context.Background(), rt, bhruntime.Files{HA: ha}, current, next); err != nil {
			t.Fatal(err)
		}
		grant := strings.Contains(rt.sql, "GRANT CONNECT ON DATABASE")
		if grant != ha {
			t.Fatal("replication CONNECT crossed the effective HA boundary")
		}
		if strings.Contains(rt.sql, "TO PUBLIC") || strings.Contains(rt.sql, "CONNECT ON DATABASE \"template1\"") {
			t.Fatal("rotation weakened Shared consumer isolation")
		}
	}
}
