package coreupdate

import (
	"context"
	"testing"
)

func TestVerifyPatroniQuorum(t *testing.T) {
	base := []PatroniMemberState{
		{Name: "postgres-member-1", Primary: true, Healthy: true},
		{Name: "postgres-member-2", Replica: true, Healthy: true},
		{Name: "postgres-member-3", Replica: true, Healthy: true},
	}
	leader, replicas, err := VerifyPatroniQuorum(context.Background(), base, 0)
	if err != nil || leader != "postgres-member-1" || len(replicas) != 2 {
		t.Fatalf("healthy quorum rejected: leader=%s replicas=%v err=%v", leader, replicas, err)
	}
	for _, tc := range []struct{name string; mutate func([]PatroniMemberState)}{
		{"missing", func(m []PatroniMemberState) { m[2].Healthy = false }},
		{"split_brain", func(m []PatroniMemberState) { m[2].Primary = true; m[2].Replica = false }},
		{"no_leader", func(m []PatroniMemberState) { m[0].Primary = false; m[0].Replica = true }},
		{"duplicate", func(m []PatroniMemberState) { m[2].Name = m[1].Name }},
		{"lag", func(m []PatroniMemberState) { m[1].ReplayLag = 2 }},
		{"unknown_role", func(m []PatroniMemberState) { m[1].Replica = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := append([]PatroniMemberState(nil), base...)
			tc.mutate(m)
			if _, _, err := VerifyPatroniQuorum(context.Background(), m, 1); err == nil {
				t.Fatal("unsafe Patroni state accepted")
			}
		})
	}
}
