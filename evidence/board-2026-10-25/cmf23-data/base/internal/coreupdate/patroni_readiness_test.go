package coreupdate

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWaitForPatroniQuorumTimeoutAndUnexpectedLeader(t *testing.T) {
	good := []PatroniMemberState{{Name: "pg1", Primary: true, Healthy: true}, {Name: "pg2", Replica: true, Healthy: true}, {Name: "pg3", Replica: true, Healthy: true}}
	gate := &fakePatroniRoll{members: good}
	if err := WaitForPatroniQuorum(context.Background(), gate, "pg1", 0, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := WaitForPatroniQuorum(context.Background(), gate, "pg2", 0, 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "unexpected leader") {
		t.Fatalf("unplanned leader accepted: %v", err)
	}
	gate.members[2].Healthy = false
	if err := WaitForPatroniQuorum(context.Background(), gate, "pg1", 0, 10*time.Millisecond); err == nil || !strings.Contains(err.Error(), "readiness not restored") {
		t.Fatalf("degraded quorum accepted: %v", err)
	}
}
