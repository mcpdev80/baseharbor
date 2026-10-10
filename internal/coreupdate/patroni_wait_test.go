package coreupdate

import (
	"context"
	"errors"
	"testing"
	"time"
)

func waitingPatroniMembers(lag int64) []PatroniMemberState {
	return []PatroniMemberState{
		{Name: "a", Primary: true, Healthy: true},
		{Name: "b", Replica: true, Healthy: true, ReplayLag: lag},
		{Name: "c", Replica: true, Healthy: true},
	}
}

func TestAwaitPatroniQuorumWaitsForReplayWithoutRelaxingZeroLag(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	calls := 0
	leader, err := AwaitPatroniQuorum(ctx, "a", 0, func(context.Context) ([]PatroniMemberState, error) {
		calls++
		lag := int64(4096)
		if calls == 3 {
			lag = 0
		}
		return waitingPatroniMembers(lag), nil
	})
	if err != nil || leader != "a" || calls != 3 {
		t.Fatalf("replay convergence: leader=%q calls=%d err=%v", leader, calls, err)
	}
}

func TestAwaitPatroniQuorumUnsafeStatesFailImmediately(t *testing.T) {
	for _, kind := range []string{"unhealthy", "split-brain", "unknown-lag", "missing", "changed-leader", "inspection-failed"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			_, err := AwaitPatroniQuorum(t.Context(), "a", 0, func(context.Context) ([]PatroniMemberState, error) {
				calls++
				members := waitingPatroniMembers(4096)
				switch kind {
				case "unhealthy":
					members[1].Healthy = false
				case "split-brain":
					members[1].Primary, members[1].Replica = true, false
				case "unknown-lag":
					members[1].ReplayLag = -1
				case "missing":
					members = members[:2]
				case "changed-leader":
					members[0].Primary, members[0].Replica = false, true
					members[1].Primary, members[1].Replica = true, false
				case "inspection-failed":
					return nil, errors.New("unavailable")
				}
				return members, nil
			})
			if err == nil || calls != 1 {
				t.Fatalf("unsafe state admitted/retried: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestAwaitPatroniQuorumPersistentLagHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	_, err := AwaitPatroniQuorum(ctx, "a", time.Hour, func(context.Context) ([]PatroniMemberState, error) {
		calls++
		time.AfterFunc(time.Millisecond, cancel)
		return waitingPatroniMembers(4096), nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("persistent lag escaped bounded admission: calls=%d err=%v", calls, err)
	}
}

func TestAwaitPatroniQuorumPinsFirstObservedLeader(t *testing.T) {
	calls := 0
	_, err := AwaitPatroniQuorum(t.Context(), "", 0, func(context.Context) ([]PatroniMemberState, error) {
		calls++
		members := waitingPatroniMembers(4096)
		if calls > 1 {
			members[0].Primary, members[0].Replica = false, true
			members[1].Primary, members[1].Replica, members[1].ReplayLag = true, false, 0
		}
		return members, nil
	})
	if err == nil || calls != 2 {
		t.Fatalf("leader change admitted during replay convergence: calls=%d err=%v", calls, err)
	}
}
