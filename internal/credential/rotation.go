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
}

func (r Rotation) Run(ctx context.Context) error {
	for name, fn := range map[string]func(context.Context) error{
		"prepare": r.Prepare, "reconcile": r.Reconcile, "verify": r.Verify, "retire": r.Retire,
	} {
		if fn == nil {
			return fmt.Errorf("credential rotation %s step is required", name)
		}
	}
	if err := r.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare new credential material: %w", err)
	}
	if err := r.Reconcile(ctx); err != nil {
		return errors.Join(fmt.Errorf("reconcile credential consumers: %w", err), r.rollback(ctx))
	}
	if err := r.Verify(ctx); err != nil {
		return errors.Join(fmt.Errorf("verify rotated credential: %w", err), r.rollback(ctx))
	}
	if err := r.Retire(ctx); err != nil {
		// Verification already succeeded; keep the new path valid and report that
		// old material still needs retirement rather than rolling back to it.
		return fmt.Errorf("retire old credential material: %w", err)
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
