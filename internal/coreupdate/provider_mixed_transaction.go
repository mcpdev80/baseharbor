package coreupdate

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RunMixedProviderUpdates keeps one durable ExecuteJournaled transaction while
// delegating OpenBao/Keycloak lifecycle work to their bound provider adapters.
// SQL/backing deltas continue to use the native volume/Compose recovery path.
func RunMixedProviderUpdates(ctx context.Context, plan Plan, journalPath string, ops NativeProviderOps, assets map[string]NativeProviderAssets, bound Hooks, useBound func(Delta) bool) error {
	if ops == nil || useBound == nil {
		return errors.New("Core mixed provider update requires native operations and provider selector")
	}
	if journalPath == "" {
		return errors.New("durable Core update journal path required")
	}
	if bound.Preflight == nil || bound.RecoveryPoint == nil || bound.Apply == nil || bound.Verify == nil || bound.Recover == nil {
		return errors.New("bound provider lifecycle is incomplete")
	}
	for _, delta := range plan.Deltas {
		if delta.Classification == NoChange || useBound(delta) {
			continue
		}
		entry, ok := assets[JournalKey(delta)]
		if !ok || entry.Recovery.Runtime == nil || entry.Recovery.VerifyQuiesced == nil ||
			entry.Compose.Path == "" || entry.Compose.Directory == "" {
			return fmt.Errorf("Core %s missing native recovery and Compose assets", delta.Installed.Instance)
		}
	}

	boundPlan := Plan{Release: plan.Release}
	for _, delta := range plan.Deltas {
		if useBound(delta) {
			boundPlan.Deltas = append(boundPlan.Deltas, delta)
		}
	}
	hooks := Hooks{
		Preflight: func(ctx context.Context, p Plan) error {
			if err := ops.Preflight(ctx, p); err != nil {
				return err
			}
			if len(boundPlan.Deltas) > 0 {
				return bound.Preflight(ctx, boundPlan)
			}
			return nil
		},
		RecoveryPoint: func(ctx context.Context, d Delta) error {
			if useBound(d) {
				return bound.RecoveryPoint(ctx, d)
			}
			a := assets[JournalKey(d)]
			if err := ops.Quiesce(ctx, d); err != nil {
				return err
			}
			return a.Recovery.Capture(ctx, d)
		},
		Apply: func(ctx context.Context, d Delta) error {
			if useBound(d) {
				return bound.Apply(ctx, d)
			}
			a := assets[JournalKey(d)]
			if err := a.Compose.Stage(map[string]Delta{d.Installed.Instance: d}); err != nil {
				return err
			}
			return ops.ReconcilePinned(ctx, d)
		},
		Verify: func(ctx context.Context, d Delta) error {
			if useBound(d) {
				if err := bound.Verify(ctx, d); err != nil {
					return err
				}
			}
			return ops.VerifySemantics(ctx, d)
		},
		Recover: func(ctx context.Context, d Delta, state string) error {
			if useBound(d) {
				return bound.Recover(ctx, d, state)
			}
			a := assets[JournalKey(d)]
			if err := ops.Quiesce(ctx, d); err != nil {
				return err
			}
			if err := a.Recovery.Recover(ctx, d); err != nil {
				return err
			}
			if err := a.Compose.Restore(map[string]Delta{d.Installed.Instance: d}); err != nil {
				return err
			}
			if err := ops.ReconcileOriginal(ctx, d); err != nil {
				return err
			}
			return ops.VerifySemantics(ctx, d)
		},
		Record: ops.Record,
	}
	if err := ExecuteJournaled(ctx, plan, journalPath, hooks); err == nil {
		return nil
	} else {
		journal, readErr := LoadJournal(journalPath, plan.Release)
		if readErr != nil {
			return errors.Join(err, readErr)
		}
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for _, delta := range plan.Deltas {
			status := journal.Steps[JournalKey(delta)]
			if status != "applying" && status != "apply_failed" && status != "verify_failed" {
				continue
			}
			if recoverErr := hooks.Recover(rollbackCtx, delta, status); recoverErr != nil {
				return errors.Join(err, fmt.Errorf("automatic recovery failed for %s: %w", delta.Installed.Instance, recoverErr))
			}
			if recordErr := ops.Record(rollbackCtx, delta, "recovered"); recordErr != nil {
				return errors.Join(err, fmt.Errorf("provider recovery receipt failed: %w", recordErr))
			}
			if recordErr := journal.Record(journalPath, delta, "recovered"); recordErr != nil {
				return errors.Join(err, fmt.Errorf("persist provider recovered state: %w", recordErr))
			}
		}
		return err
	}
}
