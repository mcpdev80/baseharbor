package coreupdate

import (
	"context"
	"errors"
	"fmt"
)

// ExecuteJournaled runs an already validated release plan against a durable
// per-realization journal. An interrupted apply is NEVER skipped: a resumed
// attempt must re-run provider-native idempotent reconciliation and verification.
// A verified step is also re-verified before it can be skipped, preventing
// stale disk state from being mistaken for a live ready provider.
func ExecuteJournaled(ctx context.Context, plan Plan, journalPath string, hooks Hooks) error {
	journal, err := LoadJournal(journalPath, plan.Release)
	if err != nil {
		return err
	}
	originalVerify := hooks.Verify
	originalRecord := hooks.Record
	if originalVerify == nil || originalRecord == nil {
		return errors.New("journaled Core updates require live verification and journal hooks")
	}
	if err := validateExecutionPlan(plan, hooks); err != nil {
		return err
	}
	// Never replay an incomplete provider migration unless its recovery hook
	// explicitly permits idempotent resume. Ordinary application is not safe.
	for _, delta := range plan.Deltas {
		status := journal.Steps[JournalKey(delta)]
		if status == "applying" || status == "apply_failed" || status == "verify_failed" {
			if hooks.RecoveryPoint == nil {
				return fmt.Errorf("incomplete Core provider %s requires explicit recovery before replay", JournalKey(delta))
			}
		}
	}
	remaining := Plan{Release: plan.Release}
	for _, delta := range plan.Deltas {
		if delta.Classification != NoChange && journal.Steps[JournalKey(delta)] == "verified" {
			if err := originalVerify(ctx, delta); err != nil {
				return fmt.Errorf("previously verified Core provider %s is no longer ready: %w", JournalKey(delta), err)
			}
			continue
		}
		remaining.Deltas = append(remaining.Deltas, delta)
	}
	hooks.Record = func(ctx context.Context, delta Delta, state string) error {
		// Publish to the external receipt projection before marking verified on disk.
		// If publication fails, the journal remains replayable on restart.
		if err := originalRecord(ctx, delta, state); err != nil {
			return err
		}
		return journal.Record(journalPath, delta, state)
	}
	// Execute replays incomplete mutations, but skips no provider implicitly.
	return Execute(ctx, remaining, hooks)
}
