package devaccess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// RecoveryResult reports whether a device rebind could restore the authoritative
// developer credential for a target. It is returned by Recover and never
// carries secret material.
type RecoveryResult string

const (
	// Recovered means the rebind restored or reconciled the authoritative
	// developer credential for the target.
	Recovered RecoveryResult = "recovered"
	// Unrecovered means no opt-in local credential exists to rebind; the caller
	// must fail closed and never prompt for or accept new material.
	Unrecovered RecoveryResult = "unrecovered"
)

// ErrNoRecoverableCredential documents the documented-absent case: no opt-in
// local developer credential exists to rebind on a fresh device. It never
// prompts and never leaks material.
var ErrNoRecoverableCredential = errors.New("dev-access: no recoverable local developer credential for target")

// devEnvironment is the only environment developer access serves. It is fixed by
// the devaccess authority and not a caller choice.
const devEnvironment = "dev"

// Recovery binds a device rebind to the authoritative developer credential
// authority. Managed resolves an existing managed-secret object (where one
// already exists) without duplicating it; the opt-in local store is retained
// and recoverable by key. The zero value is a valid no-op binder.
type Recovery struct {
	// Managed reports whether a managed-secret object already exists for the
	// target. When nil the managed check is skipped and only the local store is
	// consulted.
	Managed ManagedAuthorityChecker
	// Store persists the local opt-in credential on first rebind so a fresh
	// device that never reconciled can seed the managed authority exactly once.
	// When nil the local store is read-only (recover only).
	Store LocalCredentialStore
}

// ManagedAuthorityChecker reports whether a managed-secret object already exists
// for target. Kept narrow so the binder depends only on the part of the
// OpenBao authority it calls.
type ManagedAuthorityChecker interface {
	Exists(ctx context.Context, target string) (bool, error)
}

// LocalCredentialStore persists a fresh local opt-in credential so a device
// rebind can seed a managed authority that was never reconciled.
type LocalCredentialStore interface {
	Persist(ctx context.Context, target, username, password string) error
}

// Recover performs the device-rebind recovery for a target.
//
//   - Local opt-in credential present and a managed object already exists: the
//     existing managed path is authoritative and is used as-is (no duplicate).
//     Recovered.
//   - Local opt-in credential present but no managed object yet (fresh device that
//     never reconciled): the local credential is persisted once and Recovered.
//   - No local opt-in credential: fail closed with ErrNoRecoverableCredential,
//     never prompting and never leaking material. Unrecovered.
func (r Recovery) Recover(ctx context.Context, target string) (RecoveryResult, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return Unrecovered, ErrNoRecoverableCredential
	}

	local, err := Load(target, devEnvironment)
	// A missing local store is the documented absent case: fail closed. A fresh
	// device that never opted in has no recoverable authority and must never
	// prompt for or accept new material.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Unrecovered, ErrNoRecoverableCredential
		}
		return Unrecovered, fmt.Errorf("load local developer credential for rebind: %w", err)
	}
	if local.Username == "" || local.Password == "" {
		return Unrecovered, ErrNoRecoverableCredential
	}

	if r.Managed != nil {
		exists, err := r.Managed.Exists(ctx, target)
		if err != nil {
			return Unrecovered, fmt.Errorf("inspect managed credential authority for rebind: %w", err)
		}
		if exists {
			// A managed-secret object already exists; that path is authoritative.
			// Use it as-is — do not duplicate or reseed. Recovered.
			return Recovered, nil
		}
		if r.Store == nil {
			// No local store wired: recover purely from the local opt-in store.
			return Recovered, nil
		}
		// Fresh device, local opt-in present, no managed object yet: seed the
		// managed authority once from the local opt-in credential.
		if err := r.Store.Persist(ctx, target, local.Username, local.Password); err != nil {
			return Unrecovered, fmt.Errorf("seed managed credential authority for rebind: %w", err)
		}
		return Recovered, nil
	}

	// No managed authority wired: recover purely from the local opt-in store.
	return Recovered, nil
}
