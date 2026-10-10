package coreupdate

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WaitForPatroniQuorum tolerates bounded startup/readiness delays after a
// single member restart. Any *valid* unexpected leader is treated as drift
// rather than silently waited out. No additional mutation can proceed until
// all three members and replay lag satisfy the HA contract again.
func WaitForPatroniQuorum(ctx context.Context, gate PatroniRollingGate, expectedLeader string, maxLag int64, timeout time.Duration) error {
	if gate == nil || expectedLeader == "" || timeout <= 0 {
		return errors.New("invalid Patroni readiness contract")
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		members, err := gate.Inspect(bounded)
		if err == nil {
			leader, _, checkErr := VerifyPatroniQuorum(bounded, members, maxLag)
			if checkErr == nil {
				if leader != expectedLeader {
					return fmt.Errorf("Patroni unexpected leader %s (wanted %s)", leader, expectedLeader)
				}
				return nil
			}
			err = checkErr
		}
		lastErr = err
		select {
		case <-bounded.Done():
			return fmt.Errorf("Patroni quorum readiness not restored: %v: %w", lastErr, bounded.Err())
		case <-ticker.C:
		}
	}
}
