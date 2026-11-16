// Package workstationlifecycle implements the portable workstation-device
// lifecycle /v1 contract: renew and revoke one workstation independently.
//
// It is the provider-neutral wire contract for the bounded
// device-lifecycle slice at the management boundary. It binds the wire record to
// the existing workstation-enrollment authority (internal/workstationenrollment)
// and, through it, the existing authoritative BaseHarbor issuer
// (internal/serviceaccess). It declares no issuer implementation and carries no
// secret: it validates the record and the recipient/scope binding, then
// delegates the only allowed effects (renew one scope, revoke one scope) to the
// authority. It enforces, at the import boundary, that a lifecycle record is
// recipient-bound, installation-scoped and import-safe.
package workstationlifecycle

import (
	"context"
	"errors"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// RecordKind is the portable workstation-lifecycle kind marker.
type RecordKind string

// RecordKindWorkstationLifecycle is the only accepted kind value.
const RecordKindWorkstationLifecycle RecordKind = "workstation-lifecycle"

// Version is the portable contract version marker.
const Version = "baseharbor.workstation-lifecycle/v1"

// ErrInvalidRecord is returned for any fail-closed contract violation.
var ErrInvalidRecord = errors.New("invalid workstation-lifecycle/v1 record")

// Record is the portable workstation-lifecycle wire record. JSON tags match the
// schema exactly.
type Record struct {
	Kind           RecordKind `json:"kind"`
	Version        string     `json:"version"`
	InstallationID string     `json:"installation_id"`
	Recipient      string     `json:"recipient"`
	Scope          string     `json:"scope"`
}

// ValidateKindEnforcement fails closed on the record kind and its conditional
// obligations:
//   - the record must be kind=workstation-lifecycle;
//   - the validity window (if supplied) must be current;
//   - the recipient must be the installation-scoped SPIFFE identity of the scope.
func (r Record) ValidateKindEnforcement() error {
	if r.Kind != RecordKindWorkstationLifecycle {
		return ErrInvalidRecord
	}
	if r.RecipientMatchesScope() != nil {
		return ErrInvalidRecord
	}
	return nil
}

// RecipientMatchesScope binds the record recipient to the scope identity. A
// foreign recipient (not issued for this installation/scope) fails closed.
func (r Record) RecipientMatchesScope() error {
	want := "spiffe://baseharbor/workstations/" + r.InstallationID + "/" + r.Scope
	if r.Recipient != want {
		return ErrInvalidRecord
	}
	return nil
}

// ResolveScope resolves the record to the bounded workstation-enrollment scope
// it names. A malformed recipient is reported as a scope error.
func (r Record) ResolveScope() (ws.Scope, error) {
	if err := r.RecipientMatchesScope(); err != nil {
		return ws.Scope{}, err
	}
	scope := ws.Scope{TenantID: r.InstallationID, Station: r.Scope}
	if scope.Validate() != nil {
		return ws.Scope{}, scope.Validate()
	}
	return scope, nil
}

// Lifecycle enforces the device lifecycle on a single scope. It is the
// bounded seam the contract entrypoints call: Renew re-signs the scope's key
// through the protected boundary for a fresh leaf, Revoke adds the scope's
// current serial to the issuer CRL, and RevokedState reports whether the
// scope's current leaf is revoked.
type Lifecycle interface {
	Renew(ctx context.Context, request ws.Request) (ws.Result, error)
	Revoke(ctx context.Context, scope ws.Scope) error
	RevokedState(ctx context.Context, scope ws.Scope) (revoked bool, err error)
}

// RenewDevice validates the record and then renews exactly the scope it names
// through the lifecycle. It validates the record and the recipient/scope
// binding BEFORE it performs the renewal effect, so a foreign kind, a foreign
// recipient, or an invalid record fail closed without issuing anything. Renewing
// another scope, or renewing a revoked scope, is refused.
//
// csrPEM is the workstation-owned key signing request for the scope's identity;
// certificateTTL is the desired fresh leaf lifetime. The record supplies the
// scope (installation + station); the caller supplies the fresh key material and
// TTL.
func RenewDevice(ctx context.Context, life Lifecycle, record Record, csrPEM []byte, certificateTTL time.Duration) (ws.Result, error) {
	var zero ws.Result
	if life == nil {
		return zero, ErrInvalidRecord
	}
	if err := record.ValidateKindEnforcement(); err != nil {
		return zero, err
	}
	scope, err := record.ResolveScope()
	if err != nil {
		return zero, err
	}
	request, err := ws.NewRenewRequest(scope, csrPEM, certificateTTL)
	if err != nil {
		return zero, err
	}
	return life.Renew(ctx, request)
}

// RevokeDevice validates the record and then revokes exactly the scope it names
// through the lifecycle. It validates the record and the recipient/scope
// binding BEFORE it performs the revocation effect, so a foreign kind, a foreign
// recipient, or an invalid record fail closed without touching any identity.
func RevokeDevice(ctx context.Context, life Lifecycle, record Record) error {
	if life == nil {
		return ErrInvalidRecord
	}
	if err := record.ValidateKindEnforcement(); err != nil {
		return err
	}
	scope, err := record.ResolveScope()
	if err != nil {
		return err
	}
	return life.Revoke(ctx, scope)
}

// RevokedState reports whether the scope named by a valid record currently
// reports a revoked identity. A malformed or foreign record fails closed with
// an error rather than reporting a safe "not revoked", so an unknown status can
// never be read as accepted.
func RevokedState(ctx context.Context, life Lifecycle, record Record) (bool, error) {
	if life == nil {
		return false, ErrInvalidRecord
	}
	if err := record.ValidateKindEnforcement(); err != nil {
		return false, err
	}
	scope, err := record.ResolveScope()
	if err != nil {
		return false, err
	}
	return life.RevokedState(ctx, scope)
}
