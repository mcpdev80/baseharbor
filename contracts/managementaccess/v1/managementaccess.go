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
// a durable record that is neither protected nor encrypted. The schema enforces
// `durable=true` implies `encrypted=true` via storage `if/then`; this Go check
// mirrors that at the import boundary.
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

// Supported AEAD algorithms. These are exactly the schema's `export.algorithm`
// enum values; keeping them here lets the Go import boundary reject anything the
// schema would reject.
const (
	AlgorithmAES256GCM       = "aes-256-gcm"
	AlgorithmChacha20Poly1305 = "chacha20-poly1305"
)

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
//   - credential-export  : an encrypted export descriptor must be present, with
//     an algorithm in the supported AEAD set and a 43-character base64url nonce,
//     matching the schema's export value domain exactly.
//   - all kinds          : the record must be trust-consistent and its validity
//     window must be ordered, current and non-elapsed.
func (p ManagementAccessProfile) ValidateKindEnforcement() error {
	if p.Import.FailClosedImport() {
		return ErrInvalidManagementAccess
	}
	// A record outside its validity window fails closed at the import boundary.
	if err := p.ValidityWindow(time.Now().UTC()); err != nil {
		return err
	}
	// Trust must bind to its own certificate before any kind-specific check.
	if err := p.ValidateTrust(); err != nil {
		return err
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
		// The Go import boundary mirrors the schema's export value domain exactly:
		// the algorithm must be one of the supported AEAD ciphers and the nonce
		// must be a 43-character base64url value (43 base64url chars decode to 32
		// bytes — the documented golden fixture decodes to 32 bytes, not 12).
		// Arbitrary nonempty algorithms or nonces are rejected so the Go check and
		// the schema cannot drift.
		if p.Export.Algorithm != AlgorithmAES256GCM && p.Export.Algorithm != AlgorithmChacha20Poly1305 {
			return ErrInvalidManagementAccess
		}
		if !nonceIsBase64Url43(p.Export.Nonce) {
			return ErrInvalidManagementAccess
		}
	default:
		return ErrInvalidManagementAccess
	}
	// Export is a credential-export-only envelope. A management-profile or
	// device-enrollment that carries an export descriptor violates the contract;
	// the schema forbids it and this check enforces it at the Go import boundary
	// (the JSON Schema implementation rejects a non-credential-export kind with
	// an export object only via the import/encryption rules, so guard here).
	if p.Kind != CredentialExport && p.Export != nil {
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

// nonceIsBase64Url43 reports whether nonce is a 43-character base64url value
// (characters A-Z, a-z, 0-9, `-`, `_`), the exact wire domain the schema
// declares. A 43-character base64url string decodes to 32 bytes (the documented
// golden fixture decodes to 32 bytes, not 12); the schema only accepts 43-char
// values, so the Go check rejects any nonce of a different length or alphabet.
func nonceIsBase64Url43(nonce string) bool {
	if len(nonce) != 43 {
		return false
	}
	for i := 0; i < len(nonce); i++ {
		c := nonce[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_':
		default:
			return false
		}
	}
	return true
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

// EnrollResult is the outcome of a validated one-use device enrollment: the
// signed leaf certificate, the trust bundle of the issuing CA, and the leaf's
// expiry as reported by the issuer.
type EnrollResult struct {
	Certificate []byte
	Trust       serviceaccess.TrustBundle
	ExpiresAt   time.Time
}

// Enforcer binds a one-use device enrollment to the existing authoritative
// BaseHarbor surfaces on develop:
//
//   - internal/targetenrollment.Authority binds the one-use enrollment to a Core
//     issuer and a persistent, single-use store. Its Enroll consumes the store
//     once, signs the client-owned key through the protected Core signing
//     boundary (serviceaccess.CSRIssuer), and verifies the signed leaf against
//     the authoritative trust bundle (serviceaccess.ValidateSignedCSR).
//   - internal/hosttrust.ParseCA resolves the CA fingerprint from the stored
//     certificate and binds it to its own certificate (anchor binding).
//
// The one-use store behind the authority enforces single use; a consumed pin
// fails closed. A foreign CA, a mismatched fingerprint, or an expired/future
// leaf all fail closed.
type Enforcer interface {
	// ResolveFingerprint resolves the authoritative fingerprint of the trust
	// certificate bound to the record.
	ResolveFingerprint(caPEM string) (string, error)
	// Enroll performs a one-use enrollment through the authoritative authority.
	Enroll(ctx context.Context, request targetenrollment.Request) (targetenrollment.Result, error)
}

// EnrollDevice binds a validated management profile to a one-use device
// enrollment through the authoritative authority. It validates the profile, the
// pinned CA, the recipient and the validity window BEFORE it performs the
// enrollment effect (consuming the single-use store), so a foreign CA, an
// elapsed window, a foreign recipient or an invalid profile fail closed without
// spending the one-use pin. After the enrollment effect it verifies the issued
// leaf against the authoritative trust bundle, so a mismatched fingerprint, an
// expired/future leaf or a consumed one-use pin still fail closed.
//
// The profile is the enrollment's input contract: it carries the installation
// ID, the pinned CA (trust), the recipient and the profile expiry (validity
// window). Enforcer.ResolveFingerprint is invoked here to resolve the pinned CA
// to its authoritative fingerprint; it is the only caller.
func EnrollDevice(ctx context.Context, enforcer Enforcer, profile ManagementAccessProfile, request targetenrollment.Request) (EnrollResult, error) {
	var zero EnrollResult
	if enforcer == nil {
		return zero, ErrInvalidManagementAccess
	}
	now := time.Now().UTC()

	// 0. Enrollment kind: a device-enrollment entry must be Kind=device-enrollment.
	// Any other kind (management-profile, credential-export) is rejected at the
	// boundary before the single-use store is consumed, so an unrecognized or
	// foreign kind cannot be enrolled through this entry.
	if profile.Kind != DeviceEnrollment {
		return zero, ErrInvalidManagementAccess
	}

	// 1. Pinned CA: resolve the certificate to its authoritative fingerprint and
	// confirm it matches the fingerprint the profile pins. This rejects a
	// foreign or corrupt CA before the enrollment effect.
	pinned, err := enforcer.ResolveFingerprint(profile.Trust.CAPEM)
	if err != nil {
		return zero, ErrForeignPin
	}
	if pinned != profile.Trust.SHA256 {
		return zero, ErrForeignPin
	}

	// 2. Validity window: an elapsed, future or inverted window fails closed
	// before the enrollment effect.
	if err := profile.ValidityWindow(now); err != nil {
		return zero, err
	}

	// 3. Recipient binding: the profile recipient must bind to the enrollment
	// scope identity; a foreign pin fails closed before the enrollment effect.
	if err := ValidateRecipientBind(profile.Recipient, request.Scope); err != nil {
		return zero, err
	}

	// 4. Kind enforcement: the full profile (import, trust, storage) must be
	// contract-valid before the enrollment effect.
	if err := profile.ValidateKindEnforcement(); err != nil {
		return zero, err
	}

	// Enrollment effect: only now is the single-use store consumed.
	result, err := enforcer.Enroll(ctx, request)
	if err != nil {
		return zero, err
	}
	// Verify the signed leaf against the authoritative trust rather than a CA
	// supplied alongside the leaf.
	if err := serviceaccess.ValidateSignedCSR(csrFromRequest(request), result.Certificate, result.Trust, now); err != nil {
		return zero, err
	}
	return EnrollResult{
		Certificate: result.Certificate.Certificate,
		Trust:       result.Trust,
		ExpiresAt:   result.Certificate.ExpiresAt,
	}, nil
}

// csrFromRequest converts an enrollment request into the CSR signing request the
// Core signing boundary expects, binding the CSR identity to the scope identity.
func csrFromRequest(request targetenrollment.Request) serviceaccess.CSRSigningRequest {
	return serviceaccess.CSRSigningRequest{
		CSRPEM:   request.CSRPEM,
		Identity: request.Scope.Identity(),
		TTL:      time.Hour,
	}
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
