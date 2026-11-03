// Package managementaccess implements the portable management-access/v1 contract
// as a callable, fail-closed production path. It does not implement a provider;
// it declares the non-secret management profile, the sensitive one-use device
// enrollment, and the explicitly encrypted local credential export, and wires
// each to the existing authoritative BaseHarbor interfaces:
//
//   - internal/hosttrust  : CA fingerprint + trusted-anchor binding.
//   - internal/targetenrollment : one-use node enrollment bound to Core scope.
//   - internal/serviceaccess : CSR signing and signed-CSR material validation.
//
// The wire contract is contracts/managementaccess/v1/profile.schema.json. Go
// struct tags mirror that JSON exactly, including the nested trust object
// (ca_pem / sha256) and the import capability flags. Importing a record grants
// no access right, cannot run a plugin, and cannot redirect credentials.
package managementaccess

import (
	"context"
	"errors"
	"time"

	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// ManagementAccessKind enumerates the three portable record kinds.
type ManagementAccessKind string

const (
	// ManagementProfile is the non-secret descriptor of an installation's
	// management access (identity / trust / recipient / scope / validity window).
	ManagementProfile ManagementAccessKind = "management-profile"
	// DeviceEnrollment is the sensitive one-use registration of a node.
	DeviceEnrollment ManagementAccessKind = "device-enrollment"
	// CredentialExport is the explicitly encrypted local credential payload.
	CredentialExport ManagementAccessKind = "credential-export"
)

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

// ImportCapability reports the import guarantees. The contract requires every
// field to be false; importing the record must not grant rights, run a plugin,
// or redirect credentials.
type ImportCapability struct {
	RightsGranting     bool `json:"rights_granting"`
	PluginExecution    bool `json:"plugin_execution"`
	CredentialRedirect bool `json:"credential_redirect"`
}

// FailClosedImport reports whether the import capability violates the contract.
// A record is only valid for import when every flag is false; any true flag is
// a violation and must be rejected before it reaches a provider.
func (c ImportCapability) FailClosedImport() bool {
	return c.RightsGranting || c.PluginExecution || c.CredentialRedirect
}

// Trust binds a managed CA certificate to its authoritative fingerprint. The
// fingerprint is computed from the certificate itself, never assumed.
type Trust struct {
	// CAPEM is the PEM-encoded X.509 certificate (not the private key).
	CAPEM string `json:"ca_pem"`
	// SHA256 is the SHA-256 of the certificate DER, as produced by
	// hosttrust.ParseCA.
	SHA256 string `json:"sha256"`
}

// Storage records how a record is persisted. A device-enrollment must be
// sensitive (protected-durable or session-only, never a plaintext durable
// export); a management-profile storage must not advertise unsafe durable
// plaintext.
type Storage struct {
	Class            StorageClass `json:"class"`
	Durable          bool         `json:"durable"`
	Encrypted        bool         `json:"encrypted"`
	DisasterRecovery bool         `json:"disaster_recovery"`
}

// FailClosedStorage reports a storage configuration that violates the contract:
// a durable record that is neither protected nor encrypted.
func (s Storage) FailClosedStorage() bool {
	return s.Durable && !s.Encrypted
}

// EncryptionDescriptor is the explicit encryption envelope of a credential
// export. Every field is required and the payload must be encrypted.
type EncryptionDescriptor struct {
	Algorithm string `json:"algorithm"` // aes-256-gcm | chacha20-poly1305
	Encrypted bool   `json:"encrypted"`
	Nonce     string `json:"nonce"`
}

// ManagementAccessProfile is the wire record. JSON tags match the schema exactly,
// including the nested trust object. The kind value is serialized into the JSON
// key "kind" and the version marker into the JSON key "version", which is the
// schema's required key layout (schema.additionalProperties=false enumerates
// exactly "version" with its const and "kind" with its enum).
type ManagementAccessProfile struct {
	Kind           ManagementAccessKind  `json:"kind"`    // management-profile | device-enrollment | credential-export
	Version        string                `json:"version"` // baseharbor.management-access/v1 marker
	InstallationID string                `json:"installation_id"`
	Trust          Trust                 `json:"trust"`
	Recipient      string                `json:"recipient"`
	Scope          string                `json:"scope"`
	NotBefore      time.Time             `json:"not_before"`
	NotAfter       time.Time             `json:"not_after"`
	Import         ImportCapability      `json:"import"`
	Storage        Storage               `json:"storage"`
	Export         *EncryptionDescriptor `json:"export,omitempty"`
}

// ErrInvalidManagementAccess is returned for any fail-closed contract violation.
var ErrInvalidManagementAccess = errors.New("invalid management-access/v1 record")

// ValidateKindEnforcement fails closed on the record kind and its conditional
// obligations:
//   - management-profile : trust and storage must be present and safe.
//   - device-enrollment  : storage must be sensitive (never plaintext durable).
//   - credential-export  : an encrypted export descriptor must be present.
func (p ManagementAccessProfile) ValidateKindEnforcement() error {
	if p.Import.FailClosedImport() {
		return ErrInvalidManagementAccess
	}
	switch p.Kind {
	case ManagementProfile:
		if p.Trust.CAPEM == "" || p.Trust.SHA256 == "" {
			return ErrInvalidManagementAccess
		}
		if p.Storage.FailClosedStorage() {
			return ErrInvalidManagementAccess
		}
	case DeviceEnrollment:
		if p.Storage.Class == "" || p.Storage.FailClosedStorage() {
			return ErrInvalidManagementAccess
		}
	case CredentialExport:
		if p.Export == nil || !p.Export.Encrypted || p.Export.Algorithm == "" || p.Export.Nonce == "" {
			return ErrInvalidManagementAccess
		}
	default:
		return ErrInvalidManagementAccess
	}
	return nil
}

// TrustFingerprint returns the authoritative SHA-256 fingerprint of the stored
// CA certificate, or an error if the certificate cannot be parsed as a CA.
func (p ManagementAccessProfile) TrustFingerprint() (string, error) {
	_, fingerprint, err := hosttrust.ParseCA([]byte(p.Trust.CAPEM))
	return fingerprint, err
}

// ValidityWindow returns an error when the validity window is malformed
// (not_before after not_after) or when now is outside [not_before, not_after].
// An elapsed or future record fails closed.
func (p ManagementAccessProfile) ValidityWindow(now time.Time) error {
	if p.NotBefore.After(p.NotAfter) {
		return ErrInvalidManagementAccess
	}
	if now.Before(p.NotBefore) || now.After(p.NotAfter) {
		return ErrInvalidManagementAccess
	}
	return nil
}

// ErrUntrustedTrust is returned when the stored fingerprint does not match the
// certificate, i.e. the trust anchor is not bound to its own certificate.
var ErrUntrustedTrust = errors.New("management-access trust fingerprint does not match the certificate")

// ValidateTrust resolves the fingerprint from the actual certificate and binds
// it to the stored fingerprint. This is the fail-closed trust check: a record
// whose ca_pem does not hash to trust.SHA256 is rejected.
func (p ManagementAccessProfile) ValidateTrust() error {
	fingerprint, err := p.TrustFingerprint()
	if err != nil {
		return err
	}
	if fingerprint != p.Trust.SHA256 {
		return ErrUntrustedTrust
	}
	return nil
}

// EnrollResult is the outcome of a one-use device enrollment against the
// authoritative target-enrollment authority.
type EnrollResult struct {
	Certificate []byte
	Trust       serviceaccess.TrustBundle
}

// Enroller binds a one-use device enrollment to an existing Core issuer and a
// persistent, single-use store. It is satisfied by
// targetenrollment.Authority.
type Enroller interface {
	Enroll(ctx context.Context, request targetenrollment.Request) (targetenrollment.Result, error)
}

// EnrollDevice performs a one-use device enrollment through the authoritative
// authority. A foreign or consumed one-use pin fails closed via
// targetenrollment.ErrDenied.
func EnrollDevice(ctx context.Context, authority Enroller, request targetenrollment.Request) (EnrollResult, error) {
	var zero EnrollResult
	if authority == nil {
		return zero, ErrInvalidManagementAccess
	}
	result, err := authority.Enroll(ctx, request)
	if err != nil {
		return zero, err
	}
	return EnrollResult{
		Certificate: result.Certificate.Certificate,
		Trust:       result.Trust,
	}, nil
}

// ErrForeignPin is returned when a record's recipient does not match the
// authorized enrollment scope identity.
var ErrForeignPin = errors.New("management-access recipient is not bound to the authorized enrollment scope")

// ValidateRecipientBind binds the record recipient to the enrollment scope
// identity. A foreign pin (recipient not issued for this scope) fails closed.
func ValidateRecipientBind(recipient string, scope targetenrollment.Scope) error {
	if recipient == "" || scope.Validate() != nil {
		return ErrInvalidManagementAccess
	}
	if recipient != scope.Identity() {
		return ErrForeignPin
	}
	return nil
}
