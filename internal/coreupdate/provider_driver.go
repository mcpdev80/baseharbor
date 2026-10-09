package coreupdate

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// NativeProviderOps holds provider-specific lifecycle operations (PostgreSQL,
// OpenBao and Keycloak). It never guesses database quiescence or OIDC readiness.
type NativeProviderOps interface {
	Preflight(context.Context, Plan) error
	Quiesce(context.Context, Delta) error
	ReconcilePinned(context.Context, Delta) error
	ReconcileOriginal(context.Context, Delta) error
	VerifySemantics(context.Context, Delta) error
	Record(context.Context, Delta, string) error
}
type NativeProviderAssets struct {
	Recovery VolumeRecovery
	Compose  ComposeCheckpoint
}

// Recover restores the verified owned volume and original image before native
// reconciliation and semantic checks. Both native transaction drivers use it.
func (a NativeProviderAssets) Recover(ctx context.Context, d Delta, ops NativeProviderOps) error {
	if ops == nil || a.Recovery.Runtime == nil || a.Recovery.VerifyQuiesced == nil || a.Compose.Path == "" || a.Compose.Directory == "" {
		return errors.New("native provider recovery requires complete owned assets and lifecycle operations")
	}
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
}

// RunNativeProviderUpdates wires immutable provider-volume recovery, atomic
// pinned Compose staging, provider-native lifecycle and semantic verification
// into the durable journal. A missing hook or per-provider asset fails before
// the first provider is touched.
func RunNativeProviderUpdates(ctx context.Context, plan Plan, journalPath string, ops NativeProviderOps, assets map[string]NativeProviderAssets) error {
	if ops == nil {
		return errors.New("Core provider native operations are required")
	}
	if journalPath == "" {
		return errors.New("durable Core update journal path required")
	}
	for _, delta := range plan.Deltas {
		if delta.Classification == NoChange {
			continue
		}
		entry, ok := assets[JournalKey(delta)]
		if !ok || entry.Recovery.Runtime == nil || entry.Recovery.VerifyQuiesced == nil ||
			entry.Compose.Path == "" || entry.Compose.Directory == "" {
			return fmt.Errorf("Core %s missing provider-native recovery and Compose assets", delta.Installed.Instance)
		}
	}
	hooks := Hooks{
		Preflight: ops.Preflight,
		RecoveryPoint: func(ctx context.Context, d Delta) error {
			a := assets[JournalKey(d)]
			if err := ops.Quiesce(ctx, d); err != nil {
				return err
			}
			return a.Recovery.Capture(ctx, d)
		},
		Recover: func(ctx context.Context, d Delta, _ string) error {
			a := assets[JournalKey(d)]
			return a.Recover(ctx, d, ops)
		},
		Apply: func(ctx context.Context, d Delta) error {
			a := assets[JournalKey(d)]
			if err := a.Compose.Stage(map[string]Delta{d.Installed.Instance: d}); err != nil {
				return err
			}
			return ops.ReconcilePinned(ctx, d)
		},
		Verify: ops.VerifySemantics,
		Record: ops.Record,
	}

	err := ExecuteJournaled(ctx, plan, journalPath, hooks)
	if err == nil {
		return nil
	}
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
