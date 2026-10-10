package coreupdate

import (
	"context"
	"errors"
	"fmt"
	"time"
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
func RollPatroniCluster(ctx context.Context, gate PatroniSwitchoverGate, dcs DCSRecoveryAdapter, evidence DCSRecoveryEvidence, installation, target, cluster, release string, maxLag int64) error {
	if gate == nil {
		return errors.New("native Patroni rolling provider is unavailable")
	}
	if err := VerifyDCSEvidence(ctx, dcs, evidence, installation, target, cluster, release); err != nil {
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
	// An already-upgraded leader can be the verified candidate from an
	// interrupted switchover. Reconcile only the single old primary that
	// is now a replica; NEVER issue another switchover in that case.
	leaderState, err := gate.StepState(ctx, leader)
	if err != nil {
		return err
	}
	if leaderState != "" {
		return resumePatroniAfterSwitchover(ctx, gate, leader, replicas, leaderState, maxLag)
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
		case "":
			if err := VerifyDCSEvidence(ctx, dcs, evidence, installation, target, cluster, release); err != nil {
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
		default:
			return fmt.Errorf("unsupported Patroni journal state %s", state)
		}
		if err := WaitForPatroniQuorum(ctx, gate, leader, maxLag, 30*time.Second); err != nil {
			return err
		}
		// Image identity alone does not prove that PostgreSQL has rejoined.
		// Persist completion only after the member and its replay lag are ready.
		if state != "verified" {
			if err := gate.Record(ctx, replica, "verified"); err != nil {
				return err
			}
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
	if err := VerifyDCSEvidence(ctx, dcs, evidence, installation, target, cluster, release); err != nil {
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
	if err := WaitForPatroniQuorum(ctx, gate, candidate, maxLag, 30*time.Second); err != nil {
		return fmt.Errorf("Patroni switchover not proven; refusing old primary mutation: %w", err)
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
	if err := WaitForPatroniQuorum(ctx, gate, candidate, maxLag, 30*time.Second); err != nil {
		return fmt.Errorf("Patroni leader drift after old primary reconciliation: %w", err)
	}
	return gate.Record(ctx, leader, "verified")
}

// resumePatroniAfterSwitchover requires a unique old-primary journal marker,
// two already-verified images and a healthy promoted leader. Recreating the
// former leader is safe only after it is observed as a replica; no second
// switchover is permitted on an unknown timeline.
func resumePatroniAfterSwitchover(ctx context.Context, gate PatroniSwitchoverGate, leader string, replicas []string, leaderState string, maxLag int64) error {
	if leaderState != "verified" || len(replicas) != 2 {
		return errors.New("UNSUPPORTED: current leader already has upgrade journal state; reconcile original switchover before resume")
	}
	if err := gate.VerifyMemberImage(ctx, leader); err != nil {
		return err
	}
	former := ""
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
			if former != "" {
				return errors.New("UNSUPPORTED: multiple interrupted Patroni replicas; manual reconciliation required")
			}
			former = replica
		default:
			return errors.New("UNSUPPORTED: interrupted Patroni leader timeline is ambiguous")
		}
	}
	if former == "" {
		// The last member receipt may have reached disk before the caller's
		// overall success receipt. All three images and roles were rechecked;
		// completing this replay must not trigger another switchover.
		return WaitForPatroniQuorum(ctx, gate, leader, maxLag, 30*time.Second)
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return err
	}
	if err := WaitForPatroniQuorum(ctx, gate, leader, maxLag, 30*time.Second); err != nil {
		return fmt.Errorf("promoted Patroni leader is unverified: %w", err)
	}
	if err := gate.VerifyMemberImage(ctx, former); err != nil {
		if err := gate.Recreate(ctx, former); err != nil {
			return fmt.Errorf("recreate verified old-primary replica %s: %w", former, err)
		}
	}
	if err := gate.VerifyMemberImage(ctx, former); err != nil {
		return err
	}
	if err := WaitForPatroniQuorum(ctx, gate, leader, maxLag, 30*time.Second); err != nil {
		return fmt.Errorf("leader drift on interrupted Patroni recovery: %w", err)
	}
	return gate.Record(ctx, former, "verified")
}

func verifiedPatroniSnapshot(ctx context.Context, gate PatroniRollingGate, maxLag int64) (string, []string, error) {
	snapshot, err := gate.Inspect(ctx)
	if err != nil {
		return "", nil, err
	}
	return VerifyPatroniQuorum(ctx, snapshot, maxLag)
}

// PrepareAndRollPatroniCluster is the central handoff for Session 1B. It
// obtains a fresh DCS snapshot through the injected adapter, verifies its
// identity and restore viability, and only then enters the native mutation
// state machine. A nil/unavailable adapter stops before any member action.
func PrepareAndRollPatroniCluster(ctx context.Context, gate PatroniSwitchoverGate, dcs DCSRecoveryAdapter, installation, target, cluster, release string, maxLag int64) error {
	if gate == nil {
		return errors.New("native Patroni rolling provider is unavailable")
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return fmt.Errorf("PostgreSQL physical recovery proof: %w", err)
	}
	evidence, err := CaptureAndVerifyDCS(ctx, dcs, installation, target, cluster, release)
	if err != nil {
		return err
	}
	return RollPatroniCluster(ctx, gate, dcs, evidence, installation, target, cluster, release, maxLag)
}

// StageAndRollPatroniCluster is the durable high-availability coordinator.
// A streaming PostgreSQL backup is verified before obtaining DCS evidence;
// both recovery artifacts precede atomic, idempotent Compose staging, and only
// then may individual native Patroni members be reconciled.
func StageAndRollPatroniCluster(ctx context.Context, gate PatroniSwitchoverGate, dcs DCSRecoveryAdapter, dcsCheckpoint DCSCheckpoint, images HAPostgresComposeCheckpoint, installation, target, cluster, release string, maxLag int64) error {
	if gate == nil {
		return errors.New("native Patroni rolling provider is unavailable")
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return fmt.Errorf("PostgreSQL physical backup verification failed: %w", err)
	}
	evidence, err := dcsCheckpoint.Acquire(ctx, dcs, installation, target, cluster, release)
	if err != nil {
		return err
	}
	if err := gate.VerifyRecovery(ctx); err != nil {
		return err
	}
	if err := VerifyDCSEvidence(ctx, dcs, evidence, installation, target, cluster, release); err != nil {
		return err
	}
	// A failed live Patroni quorum check must not even rewrite Compose.
	// Stage changes durable on-disk desired state; inspect the untouched
	// cluster before crossing that boundary.
	members, err := gate.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect Patroni before immutable image staging: %w", err)
	}
	if _, _, err := VerifyPatroniQuorum(ctx, members, maxLag); err != nil {
		return fmt.Errorf("Patroni pre-stage quorum: %w", err)
	}
	if err := images.Stage(); err != nil {
		return fmt.Errorf("stage owned immutable Spilo images: %w", err)
	}
	return RollPatroniCluster(ctx, gate, dcs, evidence, installation, target, cluster, release, maxLag)
}
