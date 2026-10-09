package coreupdate

import (
	"context"
	"errors"
	"fmt"
)

// DCSRecoveryEvidence is the minimum cross-session contract with the etcd
// adapter (Session 1B). Its identity must be bound to the managed installation,
// target, cluster and release. SHA256 covers the complete immutable snapshot bytes.
type DCSRecoveryEvidence struct {
	Installation string
	Target       string
	Cluster      string
	Release      string
	SnapshotID   string
	SHA256       string
}

// DCSRecoveryAdapter is supplied by Session 1B. Core orchestration owns
// sequencing and never substitutes a live volume copy for etcd snapshotting.
type DCSRecoveryAdapter interface {
	Snapshot(context.Context) (DCSRecoveryEvidence, error)
	Validate(context.Context, DCSRecoveryEvidence) error
	VerifyRestorable(context.Context, DCSRecoveryEvidence) error
	Restore(context.Context, DCSRecoveryEvidence) error
}

var ErrDCSUnsupported = errors.New("UNSUPPORTED: durable etcd DCS snapshot and verified restore adapter unavailable")
var ErrDCSInvalidEvidence = errors.New("invalid etcd DCS recovery evidence")

func VerifyDCSEvidence(ctx context.Context, adapter DCSRecoveryAdapter, evidence DCSRecoveryEvidence, installation, target, cluster, release string) error {
	if err := validateDCSEvidence(ctx, adapter, evidence, installation, target, cluster, release); err != nil {
		return err
	}
	if err := adapter.VerifyRestorable(ctx, evidence); err != nil {
		return fmt.Errorf("etcd snapshot recovery verification failed: %w", err)
	}
	return nil
}

// Snapshot validation is safe after activation. Isolated boot verification
// must not restart data directories that now belong to the active quorum.
func validateDCSEvidence(ctx context.Context, adapter DCSRecoveryAdapter, evidence DCSRecoveryEvidence, installation, target, cluster, release string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if adapter == nil {
		return ErrDCSUnsupported
	}
	if installation == "" || target == "" || cluster == "" || release == "" || evidence.Installation != installation || evidence.Target != target || evidence.Cluster != cluster || evidence.Release != release || evidence.SnapshotID == "" || !validDigest("sha256:"+evidence.SHA256) {
		return ErrDCSInvalidEvidence
	}
	if err := adapter.Validate(ctx, evidence); err != nil {
		return fmt.Errorf("etcd snapshot validation failed: %w", err)
	}
	return nil
}

// CaptureAndVerifyDCS delegates snapshot creation exclusively to Session 1B.
// No cluster mutation is authorized until the same artifact is both validated
// and independently checked for restorable content.
func CaptureAndVerifyDCS(ctx context.Context, adapter DCSRecoveryAdapter, installation, target, cluster, release string) (DCSRecoveryEvidence, error) {
	if adapter == nil {
		return DCSRecoveryEvidence{}, ErrDCSUnsupported
	}
	evidence, err := adapter.Snapshot(ctx)
	if err != nil {
		return DCSRecoveryEvidence{}, fmt.Errorf("etcd DCS snapshot creation failed: %w", err)
	}
	if err := VerifyDCSEvidence(ctx, adapter, evidence, installation, target, cluster, release); err != nil {
		return DCSRecoveryEvidence{}, err
	}
	return evidence, nil
}
