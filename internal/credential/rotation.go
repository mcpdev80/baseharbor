package credential

import (
	"context"
	"errors"
	"fmt"
)

// Rotation performs one non-HA credential/material replacement.
//
// Prepare must make new material usable without retiring the old path.
// Reconcile moves the managed consumer to the new material.
// Verify proves readiness/authentication with the new material.
// Retire runs only after successful verification.
type Rotation struct {
	Prepare   func(context.Context) error
	Reconcile func(context.Context) error
	Verify    func(context.Context) error
	Retire    func(context.Context) error
	Rollback  func(context.Context) error
	Journal   RotationJournal
	Prepared  PreparedMaterialStore
	Key       string
}

func (r Rotation) Run(ctx context.Context) error {
	for name, fn := range map[string]func(context.Context) error{
		"prepare": r.Prepare, "reconcile": r.Reconcile, "verify": r.Verify, "retire": r.Retire,
	} {
		if fn == nil {
			return fmt.Errorf("credential rotation %s step is required", name)
		}
	}

	phase := RotationPhase("")
	if r.Journal != nil {
		if _, err := validateRotationKey(r.Key); err != nil {
			return err
		}
		loaded, err := r.Journal.Load(r.Key)
		if err != nil {
			return fmt.Errorf("load credential rotation journal: %w", err)
		}
		phase = loaded
	}

	if !rotationPhaseAtLeast(phase, RotationPrepared) {
		if err := r.Prepare(ctx); err != nil {
			return fmt.Errorf("prepare new credential material: %w", err)
		}
		if err := r.savePhase(RotationPrepared); err != nil {
			return err
		}
		phase = RotationPrepared
	}
	if !rotationPhaseAtLeast(phase, RotationReconciled) {
		if err := r.Reconcile(ctx); err != nil {
			return errors.Join(fmt.Errorf("reconcile credential consumers: %w", err), r.rollbackAndReset(ctx))
		}
		if err := r.savePhase(RotationReconciled); err != nil {
			return err
		}
		phase = RotationReconciled
	}
	if !rotationPhaseAtLeast(phase, RotationVerified) {
		if err := r.Verify(ctx); err != nil {
			return errors.Join(fmt.Errorf("verify rotated credential: %w", err), r.rollbackAndReset(ctx))
		}
		if err := r.savePhase(RotationVerified); err != nil {
			return err
		}
		phase = RotationVerified
	}
	if !rotationPhaseAtLeast(phase, RotationRetired) {
		if err := r.Retire(ctx); err != nil {
			// Verification already succeeded; keep the new path valid and preserve
			// VERIFIED in the journal so a retry only attempts retirement.
			return fmt.Errorf("retire old credential material: %w", err)
		}
		if err := r.savePhase(RotationRetired); err != nil {
			return err
		}
	}
	if r.Journal != nil {
		if err := r.Journal.Clear(r.Key); err != nil {
			return fmt.Errorf("clear completed credential rotation journal: %w", err)
		}
	}
	if r.Prepared != nil {
		if err := r.Prepared.Clear(r.Key); err != nil {
			return fmt.Errorf("clear completed prepared credential material: %w", err)
		}
	}
	return nil
}

func (r Rotation) savePhase(phase RotationPhase) error {
	if r.Journal == nil {
		return nil
	}
	if err := r.Journal.Save(r.Key, phase); err != nil {
		return fmt.Errorf("persist credential rotation phase %s: %w", phase, err)
	}
	return nil
}

func (r Rotation) rollbackAndReset(ctx context.Context) error {
	if err := r.rollback(ctx); err != nil {
		return err
	}
	if r.Journal != nil {
		if err := r.Journal.Clear(r.Key); err != nil {
			return fmt.Errorf("clear rolled-back credential rotation journal: %w", err)
		}
	}
	if r.Prepared != nil {
		if err := r.Prepared.Clear(r.Key); err != nil {
			return fmt.Errorf("clear rolled-back prepared credential material: %w", err)
		}
	}
	return nil
}

func (r Rotation) rollback(ctx context.Context) error {
	if r.Rollback == nil {
		return errors.New("rotation failed before verification and no rollback path is available")
	}
	if err := r.Rollback(ctx); err != nil {
		return fmt.Errorf("restore previous valid credential path: %w", err)
	}
	return nil
}
