package coreupdate

import (
	"context"
	"fmt"
	"math"
	"time"
)

// AwaitPatroniQuorum waits only for known healthy standbys to replay WAL.
// Unknown state, split brain, inspection failure or a changed expected leader
// is never treated as transient. Callers must supply a bounded context.
func AwaitPatroniQuorum(ctx context.Context, expectedLeader string, interval time.Duration, inspect func(context.Context) ([]PatroniMemberState, error)) (string, error) {
	if inspect == nil || interval < 0 {
		return "", fmt.Errorf("Patroni quorum requires an inspector and nonnegative poll interval")
	}
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		members, err := inspect(ctx)
		if err != nil {
			return "", err
		}
		leader, _, err := VerifyPatroniQuorum(ctx, members, math.MaxInt64)
		if err != nil {
			return "", err
		}
		if expectedLeader != "" && leader != expectedLeader {
			return "", fmt.Errorf("Patroni leader changed while establishing recovery evidence; replan required")
		}
		expectedLeader = leader
		if _, _, err := VerifyPatroniQuorum(ctx, members, 0); err == nil {
			return leader, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", fmt.Errorf("Patroni standbys did not reach verified zero replay lag: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
