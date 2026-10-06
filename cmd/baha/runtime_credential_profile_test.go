package main

import (
	"context"
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
