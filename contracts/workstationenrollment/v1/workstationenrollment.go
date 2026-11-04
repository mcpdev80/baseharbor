// Package workstationenrollment implements the portable workstation-enrollment
// /v1 contract: a recipient-bound, short-lived, one-use workstation registration
// and the separate device identity it issues.
//
// It is the provider-neutral wire contract for the bounded
// workstation-enrollment slice at the management boundary. It binds the wire
// record to the existing authoritative BaseHarbor interfaces (internal/serviceaccess
// issuer + internal/workstationenrollment one-use authority) and enforces, at
// the import boundary, that a workstation record is recipient-bound, scoped,
// time-windowed, single-use, and import-safe. It declares no provider
// implementation and carries no secret.
package workstationenrollment

import (
	"context"
	"errors"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// RecordKind is the portable workstation-enrollment kind marker.
type RecordKind string

// RecordKindWorkstationEnrollment is the only accepted kind value.
const RecordKindWorkstationEnrollment RecordKind = "workstation-enrollment"

// Version is the portable contract version marker.
const Version = "baseharbor.workstation-enrollment/v1"

// ErrInvalidRecord is returned for any fail-closed contract violation.
var ErrInvalidRecord = errors.New("invalid workstation-enrollment/v1 record")

// ImportCapability reports the import guarantees. The contract requires every
// field to be false; importing the record must not grant rights, run a plugin,
// or redirect credentials.
type ImportCapability struct {
	RightsGranting     bool `json:"rights_granting"`
	PluginExecution    bool `json:"plugin_execution"`
	CredentialRedirect bool `json:"credential_redirect"`
}

// FailClosedImport reports whether the import capability violates the contract.
func (c ImportCapability) FailClosedImport() bool {
	return c.RightsGranting || c.PluginExecution || c.CredentialRedirect
}

// StorageClass is the storage durability class of a record.
type StorageClass string

const (
	// SessionOnly storage lives in memory for the lifetime of the process and
	// is never persisted.
	SessionOnly StorageClass = "session-only"
	// ProtectedDurable storage is persisted behind host protection (keyring,
	// OS-protected volume).
	ProtectedDurable StorageClass = "protected-durable"
)

// Storage records how a record is persisted. A workstation-enrollment must be
// sensitive (protected-durable or session-only, never a plaintext durable
// export).
type Storage struct {
	Class            StorageClass `json:"class"`
	Durable          bool         `json:"durable"`
	Encrypted        bool         `json:"encrypted"`
	DisasterRecovery bool         `json:"disaster_recovery"`
}

// FailClosedStorage reports a storage configuration that violates the contract:
// a durable record that is neither protected nor encrypted. The schema enforces
// `durable=true` implies `encrypted=true`; this Go check mirrors that at the
// import boundary.
func (s Storage) FailClosedStorage() bool {
	return s.Durable && !s.Encrypted
}

// Bootstrap window constrains the one-use authorization and leaf lifetime.
type Bootstrap struct {
	TTLSeconds            int `json:"ttl_seconds"`
	CertificateTTLSeconds int `json:"certificate_ttl_seconds"`
}

// Record is the portable workstation-enrollment wire record. JSON tags match
// the schema exactly.
type Record struct {
	Kind           RecordKind       `json:"kind"`
	Version        string           `json:"version"`
	InstallationID string           `json:"installation_id"`
	Recipient      string           `json:"recipient"`
	Scope          string           `json:"scope"`
	NotBefore      time.Time        `json:"not_before"`
	NotAfter       time.Time        `json:"not_after"`
	Import         ImportCapability `json:"import"`
	Storage        Storage          `json:"storage"`
	Bootstrap      Bootstrap        `json:"bootstrap"`
}

// ValidateKindEnforcement fails closed on the record kind and its conditional
// obligations:
//   - the record must be kind=workstation-enrollment;
//   - import must be all-false;
//   - the validity window must be ordered, current and non-elapsed;
//   - the recipient must be installation-scoped SPIFFE and match the scope
//     identity;
//   - storage must be sensitive (never plaintext durable);
//   - the one-use bootstrap and leaf TTLs must be within their bounds.
func (r Record) ValidateKindEnforcement() error {
	if r.Kind != RecordKindWorkstationEnrollment {
		return ErrInvalidRecord
	}
	if r.Import.FailClosedImport() {
		return ErrInvalidRecord
	}
	if r.ValidityWindow(time.Now().UTC()) != nil {
		return ErrInvalidRecord
	}
	if err := r.RecipientMatchesScope(); err != nil {
		return ErrInvalidRecord
	}
	if r.Storage.FailClosedStorage() {
		return ErrInvalidRecord
	}
	if r.Bootstrap.TTLSeconds <= 0 || r.Bootstrap.TTLSeconds > 300 {
		return ErrInvalidRecord
	}
	if r.Bootstrap.CertificateTTLSeconds <= 0 || r.Bootstrap.CertificateTTLSeconds > 86400 {
		return ErrInvalidRecord
	}
	return nil
}

// ValidityWindow returns an error when the validity window is malformed or when
// now is outside [not_before, not_after]. An elapsed or future record fails
// closed.
func (r Record) ValidityWindow(now time.Time) error {
	if r.NotBefore.After(r.NotAfter) {
		return ErrInvalidRecord
	}
	if now.Before(r.NotBefore) || now.After(r.NotAfter) {
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

// Enforcer binds a validated workstation record to the existing one-use
// workstation-enrollment authority (internal/workstationenrollment).
type Enforcer interface {
	Create(ctx context.Context, scope ws.Scope, lifetime, certificateTTL time.Duration) (ws.Bootstrap, error)
	Enroll(ctx context.Context, request ws.Request) (ws.Result, error)
}

// EnrollDevice validates the workstation record against an issuer and performs a
// single-use enrollment through the authority. It validates the record, the
// recipient/scope binding and the validity window BEFORE it performs the
// enrollment effect (consuming the single-use store), so a foreign CA, an
// elapsed window, a foreign recipient or an invalid record fail closed without
// spending the one-use bootstrap.
func EnrollDevice(ctx context.Context, enforcer Enforcer, issuer serviceaccess.CSRIssuer, record Record, request ws.Request) (ws.Result, error) {
	var zero ws.Result
	if enforcer == nil || issuer == nil {
		return zero, ErrInvalidRecord
	}
	if err := record.ValidateKindEnforcement(); err != nil {
		return zero, err
	}
	if err := record.ValidityWindow(time.Now().UTC()); err != nil {
		return zero, err
	}
	if err := record.RecipientMatchesScope(); err != nil {
		return zero, err
	}
	return enforcer.Enroll(ctx, request)
}
