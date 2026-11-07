package application

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

const (
	// MaintenanceAuthorityContractVersion is the durable record version binding a
	// submitted maintenance operation to the installation that owns a deployment.
	MaintenanceAuthorityContractVersion = "baseharbor.maintenance-authority/v1"
	// MaintenanceAuthorityFileName is written under the protected deployment
	// state root after a verified maintenance mutation.
	MaintenanceAuthorityFileName = "maintenance-authority.json"
	// MaintenanceAuthorityLimit bounds the protected record the same way other
	// deployment receipts are bounded.
	MaintenanceAuthorityLimit = 1 << 20
)

// ErrMaintenanceAuthorityForeign reports a durable maintenance window that
// belongs to a different application deployment. It is never adopted or extended.
var ErrMaintenanceAuthorityForeign = errors.New("retained maintenance window belongs to a different deployment")

// MaintenanceOperations enumerates the lifecycle operations a maintenance
// submission may name. Capability discovery and the client share this list so a
// typo cannot widen the accepted set.
var MaintenanceOperations = []string{"backup", "restore", "update", "destroy"}

// MaintenanceAuthorityRecord is the durable maintenance-window record for one
// deployment. It exists so an interrupted or lost maintenance submission
// resolves to the same durable operation instead of a replayed mutation. Revision
// numbers the window's committed operations and strictly increases with each new
// one, so a superseded revision can never be re-stamped.
type MaintenanceAuthorityRecord struct {
	ContractVersion string `json:"contract_version"`
	ApplicationID   string `json:"application_id"`
	DeploymentID    string `json:"deployment_id"`
	Target          string `json:"target"`
	Environment     string `json:"environment"`
	// Operation is the maintenance operation this window records.
	Operation string `json:"operation"`
	// Revision numbers the durable operations committed for this window.
	// Revision numbers the durable operations committed for this window. The
	// retained record is the only durable counter; the authority's verified
	// observation gives an unrecorded deployment its initial revision floor.
	Revision int `json:"revision"`
	// Digest binds the record to the exact authority revision that produced it.
	Digest string `json:"digest"`
	// ArchiveBinding is the content digest of the recovery unit the operation
	// produced, or empty when the operation has no archive. It is a digest, never
	// a server path and never secret material.
	ArchiveBinding string `json:"archive_binding,omitempty"`
}

func (r MaintenanceAuthorityRecord) Validate() error {
	if r.ContractVersion != MaintenanceAuthorityContractVersion {
		return fmt.Errorf("unsupported maintenance authority contract %q", r.ContractVersion)
	}
	if err := stableid.ValidateUUIDv4("application", r.ApplicationID); err != nil {
		return err
	}
	if err := stableid.ValidateUUIDv4("deployment", r.DeploymentID); err != nil {
		return err
	}
	if strings.TrimSpace(r.Target) == "" || strings.TrimSpace(r.Environment) == "" {
		return errors.New("maintenance authority requires a Target and environment")
	}
	switch r.Operation {
	case "backup", "restore", "update", "destroy", "":
	default:
		return fmt.Errorf("unsupported maintenance operation %q", r.Operation)
	}
	if err := ValidateMaintenanceDigest(r.Digest); err != nil {
		return err
	}
	if r.Revision < 0 {
		return errors.New("maintenance authority revision cannot be negative")
	}
	if r.Revision == 0 && r.Operation != "" {
		return errors.New("a recorded maintenance operation requires a positive revision")
	}
	return nil
}

func IsMaintenanceOperation(operation string) bool {
	for _, candidate := range MaintenanceOperations {
		if candidate == operation {
			return true
		}
	}
	return false
}

// ValidateMaintenanceDigest requires a well-formed bound digest so a caller
// cannot substitute an empty or truncated binding.
func ValidateMaintenanceDigest(digest string) error {
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) {
		return errors.New("maintenance authority digest must be a sha256 binding")
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(digest, prefix))
	if err != nil || len(raw) != sha256.Size {
		return errors.New("maintenance authority digest must be a full sha256 binding")
	}
	return nil
}

// MaintenanceAuthority is the resolved view of one deployment's durable window
// together with the deployment revision it was bound to.
type MaintenanceAuthority struct {
	MaintenanceAuthorityRecord
	// Revision is the authority revision the digest binds.
	Revision int
	// Candidate is the exact candidate the retained deployment record was verified
	// against, so a new window revision always binds the current candidate.
	Candidate string
	// RecordExists reports whether a durable window was already persisted.
	RecordExists bool
}

// ResolveMaintenanceAuthority binds the durable window for a deployment. The
// window's own revision is the counter: a retained record reports the revision it
// actually reached, and a deployment with nothing recorded starts at zero so its
// first operation claims revision one. No caller-supplied value can move it.
func ResolveMaintenanceAuthority(record, identity MaintenanceAuthorityRecord) (MaintenanceAuthority, error) {
	if record.ContractVersion != "" {
		if err := record.Validate(); err != nil {
			return MaintenanceAuthority{}, err
		}
		return MaintenanceAuthority{
			MaintenanceAuthorityRecord: record,
			Revision:                   record.Revision,
			RecordExists:               true,
		}, nil
	}
	// Nothing durable has been recorded for this deployment yet: the window is
	// bindable at revision zero and is explicitly not durable.
	return MaintenanceAuthority{
		MaintenanceAuthorityRecord: MaintenanceAuthorityRecord{
			ContractVersion: MaintenanceAuthorityContractVersion,
			ApplicationID:   identity.ApplicationID, DeploymentID: identity.DeploymentID,
			Target: identity.Target, Environment: identity.Environment,
		},
		Revision: 0, RecordExists: false,
	}, nil
}

// OperationRevision is the revision a new operation for this window must claim:
// exactly one past the revision the durable record has reached.
func (a MaintenanceAuthority) OperationRevision() int {
	return a.Revision + 1
}

// Digest is the binding value for a proposed operation in this window.
func (a MaintenanceAuthority) Digest(operation, archive string) string {
	return MaintenanceDigest(a.Target, a.Environment, operation, a.Revision, a.Candidate, archive)
}

// MaintenanceWindowCandidate is the exact candidate the retained deployment
// record was verified against, or empty when no candidate consent was recorded.
func MaintenanceWindowCandidate(record deployment.DeploymentRecord) string {
	if record.Consent != nil {
		return record.Consent.Digest
	}
	return ""
}

// maintenanceStateFile resolves the protected deployment path. Roots that are
// relative or not owner-only are refused before any read or write. The
// maintenance window is durable deployment state, so it lives in the same
// protected deployment directory as the authority record itself.
func maintenanceStateFile(stateRoot string) (string, error) {
	if !filepath.IsAbs(stateRoot) || filepath.Clean(stateRoot) != stateRoot {
		return "", errors.New("maintenance authority requires protected absolute deployment state")
	}
	info, err := os.Lstat(stateRoot)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("maintenance authority deployment state must remain owner-only")
	}
	return filepath.Join(stateRoot, MaintenanceAuthorityFileName), nil
}

// LoadMaintenanceAuthority reads the durable window. The boolean distinguishes
// "no window recorded yet" from a usable protected record: an absent file is not
// an error, a corrupt or unowned one always is. A record whose identity fields
// are empty is not a window: an unsubmitted deployment has none.
func LoadMaintenanceAuthority(stateRoot string) (MaintenanceAuthorityRecord, bool, error) {
	path, err := maintenanceStateFile(stateRoot)
	if err != nil {
		return MaintenanceAuthorityRecord{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return MaintenanceAuthorityRecord{}, false, nil
	}
	if err != nil {
		return MaintenanceAuthorityRecord{}, false, fmt.Errorf("read maintenance authority record: %w", err)
	}
	if len(data) > MaintenanceAuthorityLimit {
		return MaintenanceAuthorityRecord{}, false, errors.New("maintenance authority record exceeds protected state limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var record MaintenanceAuthorityRecord
	if err := decoder.Decode(&record); err != nil {
		return MaintenanceAuthorityRecord{}, false, fmt.Errorf("decode maintenance authority record: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return MaintenanceAuthorityRecord{}, false, errors.New("trailing maintenance authority data")
	}
	if err := record.Validate(); err != nil {
		return MaintenanceAuthorityRecord{}, false, fmt.Errorf("validate maintenance authority record: %w", err)
	}
	return record, true, nil
}

// CommitMaintenanceAuthority durably records one submitted maintenance
// operation for the deployment window. Re-submitting the identical operation,
// revision and binding is idempotent and reports "not new": a lost response
// resolves to the same durable operation rather than a second mutation. A first
// window always starts at sequence one while a new operation at the current
// revision advances the count, so a caller cannot claim history it does not have.
func CommitMaintenanceAuthority(stateRoot string, submitted MaintenanceAuthorityRecord) (MaintenanceAuthorityRecord, bool, error) {
	if err := submitted.Validate(); err != nil {
		return MaintenanceAuthorityRecord{}, false, err
	}
	path, err := maintenanceStateFile(stateRoot)
	if err != nil {
		return MaintenanceAuthorityRecord{}, false, err
	}
	current, exists, err := LoadMaintenanceAuthority(stateRoot)
	if err != nil {
		return MaintenanceAuthorityRecord{}, false, err
	}
	// A retained record's identity fields and binding are immutable: an invalid,
	// foreign or otherwise unusable retained window is refused before anything is
	// read, replaced or written.
	if exists {
		if err := current.Validate(); err != nil {
			return MaintenanceAuthorityRecord{}, false, fmt.Errorf("inspect the protected deployment state: %w", err)
		}
		if current.ApplicationID != submitted.ApplicationID || current.DeploymentID != submitted.DeploymentID {
			// Another deployment's window is never adopted or replaced, whatever the
			// caller claims: existing protected state must be inspected.
			return MaintenanceAuthorityRecord{}, false, ErrMaintenanceAuthorityForeign
		}
	}
	if exists {
		if submitted.Revision <= current.Revision {
			// A revision the window has already reached belongs to recorded history.
			// Re-affirming it reports the durable operation at that revision and never
			// rewrites the current window; a mismatched binding for that revision is
			// refused outright.
			historical, found, err := loadMaintenanceRevision(stateRoot, submitted.Revision)
			if err != nil {
				return MaintenanceAuthorityRecord{}, false, err
			}
			if !found {
				return MaintenanceAuthorityRecord{}, false, errors.New("maintenance window has no durable record at that revision")
			}
			if !MaintenanceDigestMatches(historical.Digest, submitted.Digest) {
				return MaintenanceAuthorityRecord{}, false, errors.New("a recorded maintenance binding cannot change its revision")
			}
			return historical, false, nil
		}
		if submitted.Revision != current.Revision+1 {
			// A new operation continues the window at exactly the next revision.
			return MaintenanceAuthorityRecord{}, false, errors.New("maintenance window revision does not continue the retained durable window")
		}
	} else if submitted.Revision > 1 {
		// A deployment's first durable window is revision one: a caller cannot claim
		// history that was never recorded.
		return MaintenanceAuthorityRecord{}, false, errors.New("maintenance window revision does not continue the retained durable window")
	} else if submitted.Revision == 0 {
		submitted.Revision = 1
	}
	data, err := json.MarshalIndent(submitted, "", "  ")
	if err != nil {
		return MaintenanceAuthorityRecord{}, false, fmt.Errorf("encode maintenance authority record: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return MaintenanceAuthorityRecord{}, false, fmt.Errorf("write maintenance authority record: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return MaintenanceAuthorityRecord{}, false, fmt.Errorf("protect maintenance authority record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return MaintenanceAuthorityRecord{}, false, fmt.Errorf("commit maintenance authority record: %w", err)
	}
	// The window file holds only the latest operation. Recording the committed
	// operation in the window's history is what keeps an older revision answerable
	// after the window has moved on; a replayed operation must never be reported
	// as unknown merely because a newer one superseded it.
	if err := appendMaintenanceHistory(stateRoot, submitted); err != nil {
		return MaintenanceAuthorityRecord{}, false, err
	}
	return submitted, true, nil
}

// MaintenanceDigest binds a maintenance submission to one exact authority
// revision and artifact. Equal inputs always yield equal digests; the digest is
// the value that makes a replay of the same window operation comparable.
func MaintenanceDigest(target, environment, operation string, revision int, candidate, archive string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		MaintenanceAuthorityContractVersion, target, environment, operation,
		fmt.Sprint(revision), candidate, archive,
	}, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// MaintenanceDigestMatches compares bindings without leaking content through an
// early byte exit.
func MaintenanceDigestMatches(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
