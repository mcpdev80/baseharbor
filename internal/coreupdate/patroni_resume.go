package coreupdate

import (
	"context"
	"errors"
	"fmt"
)

// PatroniRollingResume extends the runtime gate with a durable per-member
// state lookup. A pending mutation cannot be replayed blindly: it must be
// reconciled and verified independently by the provider implementation.
type PatroniRollingResume interface {
	PatroniRollingGate
	StepState(context.Context, string) (string, error)
	RecoverInterrupted(context.Context, string, string) error
}

// RollPatroniMembersResumable runs only replica steps. Verified entries are
// always live-reverified. In-progress entries require explicit recovery first.
// The primary still requires a separate, proved switchover contract.
func RollPatroniMembersResumable(ctx context.Context, gate PatroniRollingResume, maxLag int64) error {
	if gate == nil {
		return errors.New("Patroni resume requires durable recovery and journal")
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return fmt.Errorf("Patroni recovery preflight: %w", err)
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
		state, err := gate.StepState(ctx, replica)
		if err != nil {
			return err
		}
		switch state {
		case "verified":
			if err := gate.VerifyMemberImage(ctx, replica); err != nil {
				return fmt.Errorf("previously verified replica %s changed: %w", replica, err)
			}
		case "applying", "apply_failed", "verify_failed":
			if err := gate.RecoverInterrupted(ctx, replica, state); err != nil {
				return fmt.Errorf("interrupted replica %s must be recovered: %w", replica, err)
			}
			if err := gate.VerifyMemberImage(ctx, replica); err != nil {
				return err
			}
			if err := gate.Record(ctx, replica, "verified"); err != nil {
				return err
			}
		case "":
			snapshot, err := gate.Inspect(ctx)
			if err != nil {
				return err
			}
			current, _, err := VerifyPatroniQuorum(ctx, snapshot, maxLag)
			if err != nil {
				return err
			}
			if current != leader {
				return errors.New("Patroni primary changed during resume")
			}
			if err := gate.VerifyRecovery(ctx); err != nil {
				return err
			}
			if err := gate.Record(ctx, replica, "applying"); err != nil {
				return err
			}
			if err := gate.Recreate(ctx, replica); err != nil {
				return fmt.Errorf("replica %s mutation interrupted: %w", replica, err)
			}
			if err := gate.VerifyMemberImage(ctx, replica); err != nil {
				return err
			}
			if err := gate.Record(ctx, replica, "verified"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported Patroni journal state for %s", replica)
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
			return errors.New("Patroni leader changed during replica resume")
		}
	}
	return fmt.Errorf("UNSUPPORTED: Patroni primary %s requires controlled switchover", leader)
}
