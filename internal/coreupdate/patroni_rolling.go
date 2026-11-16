package coreupdate

import (
	"context"
	"errors"
	"fmt"
)

// PatroniRollingGate deliberately separates a verified backup from permission
// to mutate cluster members. The implementation must be backed by durable
// physical PostgreSQL AND DCS recovery artifacts, proven before any action.
type PatroniRollingGate interface {
	VerifyRecovery(context.Context) error
	Inspect(context.Context) ([]PatroniMemberState, error)
	Recreate(context.Context, string) error
	VerifyMemberImage(context.Context, string) error
	Record(context.Context, string, string) error
}

// RollPatroniMembers performs a replica-first replacement. There is no
// automatic leadership switch: changing the current primary is unsupported
// unless a separate, verified switchover contract has been supplied.
// Every step is rechecked before mutation and journaled before/after.
func RollPatroniMembers(ctx context.Context, gate PatroniRollingGate, maxLag int64) error {
	if gate == nil {
		return errors.New("Patroni rolling update requires recovery and runtime gates")
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return fmt.Errorf("Patroni rolling recovery preflight: %w", err)
	}
	members, err := gate.Inspect(ctx)
	if err != nil {
		return err
	}
	leader, replicas, err := VerifyPatroniQuorum(ctx, members, maxLag)
	if err != nil {
		return err
	}
	for _, replica := range replicas {
		if err := ctx.Err(); err != nil {
			return err
		}
		snapshot, err := gate.Inspect(ctx)
		if err != nil {
			return err
		}
		current, _, err := VerifyPatroniQuorum(ctx, snapshot, maxLag)
		if err != nil {
			return err
		}
		if current != leader {
			return errors.New("Patroni primary changed during rolling update; stop and replan")
		}
		if err := gate.VerifyRecovery(ctx); err != nil {
			return err
		}
		if err := gate.Record(ctx, replica, "applying"); err != nil {
			return err
		}
		if err := gate.Recreate(ctx, replica); err != nil {
			return fmt.Errorf("recreate Patroni replica %s: %w", replica, err)
		}
		if err := gate.VerifyMemberImage(ctx, replica); err != nil {
			return err
		}
		snapshot, err = gate.Inspect(ctx)
		if err != nil {
			return err
		}
		current, _, err = VerifyPatroniQuorum(ctx, snapshot, maxLag)
		if err != nil {
			return err
		}
		if current != leader {
			return errors.New("Patroni leader changed during replica verification")
		}
		if err := gate.Record(ctx, replica, "verified"); err != nil {
			return err
		}
	}
	// The primary must undergo a separately verified switchover; recreating it
	// directly could interrupt writers or change timeline unexpectedly.
	return fmt.Errorf("UNSUPPORTED: Patroni primary %s requires an explicit controlled switchover and verified journal resume", leader)
}
