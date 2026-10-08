package coreupdate

import (
	"context"
	"errors"
	"fmt"
)

// PatroniSwitchoverGate supplies the one missing native transition after all
// replicas have been individually upgraded and their image digests verified.
// Native switchover must be compare-and-set against the observed leader.
type PatroniSwitchoverGate interface {
	PatroniRollingResume
	Switchover(context.Context, string, string) error
}

// RollPatroniCluster requires both data and DCS recovery evidence before ANY
// mutation. It rolls replicas, explicitly switches leadership, then reconciles
// the old primary as a replica. An ambiguous switchover is never retried blindly.
func RollPatroniCluster(ctx context.Context, gate PatroniSwitchoverGate, dcs DCSRecoveryAdapter, evidence DCSRecoveryEvidence, installation, cluster, release string, maxLag int64) error {
	if gate == nil {
		return errors.New("native Patroni rolling provider is unavailable")
	}
	if err := VerifyDCSEvidence(ctx, dcs, evidence, installation, cluster, release); err != nil {
		return err
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return fmt.Errorf("PostgreSQL physical recovery proof: %w", err)
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
		state, err := gate.StepState(ctx, replica)
		if err != nil {
			return err
		}
		switch state {
		case "verified":
			if err := gate.VerifyMemberImage(ctx, replica); err != nil {
				return err
			}
		case "applying", "apply_failed", "verify_failed":
			if err := gate.RecoverInterrupted(ctx, replica, state); err != nil {
				return err
			}
			if err := gate.VerifyMemberImage(ctx, replica); err != nil {
				return err
			}
			if err := gate.Record(ctx, replica, "verified"); err != nil {
				return err
			}
		case "":
			if err := VerifyDCSEvidence(ctx, dcs, evidence, installation, cluster, release); err != nil {
				return err
			}
			if err := gate.VerifyRecovery(ctx); err != nil {
				return err
			}
			current, _, err := verifiedPatroniSnapshot(ctx, gate, maxLag)
			if err != nil {
				return err
			}
			if current != leader {
				return errors.New("Patroni leader changed before replica mutation")
			}
			if err := gate.Record(ctx, replica, "applying"); err != nil {
				return err
			}
			if err := gate.Recreate(ctx, replica); err != nil {
				return fmt.Errorf("rolling replica %s: %w", replica, err)
			}
			if err := gate.VerifyMemberImage(ctx, replica); err != nil {
				return err
			}
			if err := gate.Record(ctx, replica, "verified"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported Patroni journal state %s", state)
		}
		current, _, err := verifiedPatroniSnapshot(ctx, gate, maxLag)
		if err != nil {
			return err
		}
		if current != leader {
			return errors.New("Patroni leadership changed during replica rolling")
		}
	}
	candidate := replicas[0]
	state, err := gate.StepState(ctx, leader)
	if err != nil {
		return err
	}
	if state == "verified" {
		return errors.New("old-primary journal contradicts current leader identity; manual reconciliation required")
	}
	if err := VerifyDCSEvidence(ctx, dcs, evidence, installation, cluster, release); err != nil {
		return err
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return err
	}
	if state == "" {
		if err := gate.Record(ctx, leader, "applying"); err != nil {
			return err
		}
		if err := gate.Switchover(ctx, leader, candidate); err != nil {
			return fmt.Errorf("Patroni switchover outcome must be inspected before resume: %w", err)
		}
	} else if state != "applying" && state != "apply_failed" && state != "verify_failed" {
		return fmt.Errorf("unsupported old-primary journal state %q", state)
	}
	current, _, err := verifiedPatroniSnapshot(ctx, gate, maxLag)
	if err != nil {
		return err
	}
	if current != candidate {
		return errors.New("Patroni switchover not proven; refusing old primary mutation")
	}
	if state != "" {
		if err := gate.RecoverInterrupted(ctx, leader, state); err != nil {
			return err
		}
	} else {
		if err := gate.Recreate(ctx, leader); err != nil {
			return err
		}
	}
	if err := gate.VerifyMemberImage(ctx, leader); err != nil {
		return err
	}
	current, _, err = verifiedPatroniSnapshot(ctx, gate, maxLag)
	if err != nil {
		return err
	}
	if current != candidate {
		return errors.New("Patroni leader drift after old primary reconciliation")
	}
	return gate.Record(ctx, leader, "verified")
}

func verifiedPatroniSnapshot(ctx context.Context, gate PatroniRollingGate, maxLag int64) (string, []string, error) {
	snapshot, err := gate.Inspect(ctx)
	if err != nil {
		return "", nil, err
	}
	return VerifyPatroniQuorum(ctx, snapshot, maxLag)
}
