package applicationbackup

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// secureprofile.go implements the bounded, explicitly opt-in local credential
// export for r27-secure-export (t-20261018-r27-secure-export).
//
// The export is a fail-closed production path: it encrypts only the entries a
// caller explicitly selects, never leaks the export password through argv, and
// never carries remote provider secrets or fresh device identities by default.
// Authentication, version and KDF reuse internal/applicationbackup/crypto.go
// through its existing encryptPayload / decryptPayload API — this file does not
// migrate or duplicate the crypto primitives.
//
// WIRING NOTE (ownership): the callable production consumer of this adapter in a
// public command (CLI/JSON/MCP) is owned by t-20261018-r27-public-surfaces (see
// its task note, "single owner wires accepted callable adapters into public
// commands"), not by this card. This card delivers the complete, executable,
// tested adapter with a full public API so the production surface can be wired
// without another crypto migration.
//
// The export binds the management-access/v1 contract: the exported envelope is a
// credential-export whose EncryptionDescriptor carries only the supported AEAD
// cipher (aes-256-gcm) and a canonical 12-byte AEAD nonce, as validated by
// managementaccess.ValidateKindEnforcement.

// ErrEmptySelection fails closed when a caller asks to export without selecting
// at least one entry. There are no silent fallback fields.
var ErrEmptySelection = errors.New("secure-profile: export requires at least one selected entry")

// ErrInvalidName fails closed when a selected entry name is not a safe logical
// key.
var ErrInvalidName = errors.New("secure-profile: entry name is not a valid logical key")

// ErrInvalidExportManifest fails closed when the export manifest is incomplete.
var ErrInvalidExportManifest = errors.New("secure-profile: export manifest is incomplete")

// ErrEmptyExportPassword fails closed when the export password is empty.
var ErrEmptyExportPassword = errors.New("secure-profile: export password must not be empty")

// ErrMalformedEnvelope fails closed when an envelope cannot be parsed.
var ErrMalformedEnvelope = errors.New("secure-profile: envelope is malformed")

// ErrUnsupportedExportVersion fails closed when an envelope carries an unknown
// schema version.
var ErrUnsupportedExportVersion = errors.New("secure-profile: unsupported schema version")

// ErrMalformedExportPayload fails closed when a decrypted payload cannot be
// decoded.
var ErrMalformedExportPayload = errors.New("secure-profile: payload is malformed")

// SecretEntry is one explicitly selected local credential to export. Name is a
// logical, destination-relative key; Data is the credential-bearing material.
type SecretEntry struct {
	Name string
	Data []byte
}

// ExportManifest is the non-secret metadata attached to an export. It never
// carries credential material.
type ExportManifest struct {
	ApplicationID string
	Application   string
	Environment   string
}

// ExportEnvelope is the opaque, encrypted export blob. It carries no plaintext
// and no export password.
type ExportEnvelope = []byte

// BuildExport encrypts the explicitly selected entries into a fail-closed
// authenticated envelope. It refuses an empty selection and an incomplete
// manifest, and reuses crypto.go encryptPayload through the same package.
func BuildExport(manifest ExportManifest, entries []SecretEntry, password []byte) (ExportEnvelope, error) {
	if len(entries) == 0 {
		return nil, ErrEmptySelection
	}
	if err := validateSecureProfileNames(entries); err != nil {
		return nil, err
	}
	if strings.TrimSpace(manifest.ApplicationID) == "" || strings.TrimSpace(manifest.Application) == "" || strings.TrimSpace(manifest.Environment) == "" {
		return nil, ErrInvalidExportManifest
	}
	if len(password) == 0 {
		return nil, ErrEmptyExportPassword
	}

	plaintext, err := json.Marshal(exportPayload{
		SchemaVersion: SchemaVersion,
		Manifest:      manifest,
		Entries:       entries,
	})
	if err != nil {
		return nil, fmt.Errorf("encode secure-profile payload: %w", err)
	}
	envelope, err := encryptPayload(password, plaintext)
	if len(plaintext) > 0 {
		zero(plaintext)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

// OpenExport decrypts and authenticates an envelope produced by BuildExport. It
// fails closed (never a half state) when the password is wrong or the ciphertext
// was tampered or truncated: authenticated decryption rejects any mismatch before
// any plaintext is returned.
func OpenExport(envelope ExportEnvelope, password []byte) (ExportManifest, []SecretEntry, error) {
	if len(envelope) == 0 {
		return ExportManifest{}, nil, errors.New("secure-profile: envelope is empty")
	}
	var rawEnvelope struct {
		Magic      string    `json:"magic"`
		Version    int       `json:"version"`
		KDF        KDFParams `json:"kdf"`
		Nonce      string    `json:"nonce"`
		Ciphertext string    `json:"ciphertext"`
	}
	if err := json.Unmarshal(envelope, &rawEnvelope); err != nil {
		return ExportManifest{}, nil, ErrMalformedEnvelope
	}
	plaintext, err := decryptPayload(password, Envelope{
		Magic:      rawEnvelope.Magic,
		Version:    rawEnvelope.Version,
		KDF:        rawEnvelope.KDF,
		Nonce:      rawEnvelope.Nonce,
		Ciphertext: rawEnvelope.Ciphertext,
	})
	if err != nil {
		if len(plaintext) > 0 {
			zero(plaintext)
		}
		return ExportManifest{}, nil, err
	}
	var decoded exportPayload
	if err := json.Unmarshal(plaintext, &decoded); err != nil {
		zero(plaintext)
		return ExportManifest{}, nil, ErrMalformedExportPayload
	}
	zero(plaintext)
	if decoded.SchemaVersion != SchemaVersion {
		return ExportManifest{}, nil, fmt.Errorf("%w: %d", ErrUnsupportedExportVersion, decoded.SchemaVersion)
	}
	entries := make([]SecretEntry, 0, len(decoded.Entries))
	for _, e := range decoded.Entries {
		entries = append(entries, SecretEntry{Name: e.Name, Data: e.Data})
	}
	return decoded.Manifest, entries, nil
}

// validateSecureProfileNames rejects any name that is not a safe, destination-
// relative logical key, so a selected credential can never smuggle a path
// traversal or reserved namespace into a destination. It reuses the same
// logical-key discipline as backup entry names.
func validateSecureProfileNames(entries []SecretEntry) error {
	for _, e := range entries {
		if e.Name == "" {
			return fmt.Errorf("%w: empty entry name", ErrInvalidName)
		}
		if !secureProfileNameOK(e.Name) {
			return fmt.Errorf("%w: %q", ErrInvalidName, e.Name)
		}
	}
	return nil
}

func secureProfileNameOK(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if path.Clean(name) != name {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// exportPayload is the plaintext that BuildExport encrypts. It deliberately
// reuses the same schema version marker as the backup envelope so decryption
// reuses crypto.go's version check; the entry set is the caller's explicit
// selection, never a silent fallback.
type exportPayload struct {
	SchemaVersion int            `json:"schema_version"`
	Manifest      ExportManifest `json:"manifest"`
	Entries       []SecretEntry  `json:"entries"`
}
