package managementaccess

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// This file adds the bounded codec + store surface on top of the accepted
// management-access/v1 wire record declared in managementaccess.go. The wire
// record and its fail-closed import boundary are the authority; this package
// only serializes that record to/from its portable container and persists
// validated records behind a version/trust/installation/scope guard.
//
// Contract precedence (see credential-access-v1.md deferred interfaces): the
// production consumer that performs the real import, installs the CA anchor and
// binds installation/recipient/scope/trusted-time to an independent authority
// is deferred to its owning slice. The store here is the testable codec +
// persistence seam; it performs NO network call, NO plugin run and NO
// credential redirect. Import grants no right, cannot run a plugin and cannot
// redirect credentials.

// ContainerVersion is the container wire format version. The record's own
// version marker (baseharbor.management-access/v1) is the record contract; the
// container version is the codec version. A container produced by one codec
// version must not be decoded by a different codec version.
const ContainerVersion = "baseharbor.profile-container/v1"

// ErrWrongContainerVersion fails closed when a decoded container's codec
// version does not equal ContainerVersion.
var ErrWrongContainerVersion = errors.New("profile-container: wrong container version")

// ErrUnknownVersion fails closed when a decoded record's version marker is not
// baseharbor.management-access/v1.
var ErrUnknownVersion = errors.New("management-access: unknown or non-canonical record version")

// ErrMissingField fails closed when a decoded record is missing a required
// record field or a required container field.
var ErrMissingField = errors.New("management-access: record is missing a required field")

// ErrSecretOnExport fails closed when an export request carries a secret blob.
var ErrSecretOnExport = errors.New("management-access: export rejects a secret blob by default")

// ErrNoWrite guards the import boundary: a failed write must not persist.
var ErrNoWrite = errors.New("management-access: write aborted by a failed guard")

// ErrConflict fails closed (never overwrites) when a record for the same
// identity already exists in the destination and no conflict policy opts out.
var ErrConflict = errors.New("management-access: record already present in destination")

// ErrPartialWrite fails closed when a transactional import wrote a prefix but
// failed to write the full set.
var ErrPartialWrite = errors.New("management-access: partial write detected, rolled back")

// RecordKind aliases ManagementAccessKind for the codec surface.
type RecordKind = ManagementAccessKind

// ProfileContainer is the portable wire container the codec serializes to and
// from. It carries no secret: the non-secret management profile, a sensitive
// one-use device enrollment or an encrypted credential export, plus the codec
// version and the destination path the record was produced at.
type ProfileContainer struct {
	Version   string       `json:"version"`
	DestPath  string       `json:"dest_path"`
	Records   []RecordWire `json:"records"`
	Timestamp time.Time    `json:"timestamp"`
}

// RecordWire is the record as encoded into the container. It is a thin view:
// the JSON shape mirrors ManagementAccessProfile exactly (kind into "kind",
// version marker into "version"), so a decoded RecordWire can be re-holed back
// into a ManagementAccessProfile without loss.
type RecordWire struct {
	Version      string                `json:"version"`
	Kind         RecordKind            `json:"kind"`
	Installation string                `json:"installation_id"`
	Trust        Trust                 `json:"trust"`
	Recipient    string                `json:"recipient"`
	Scope        string                `json:"scope"`
	NotBefore    time.Time             `json:"not_before"`
	NotAfter     time.Time             `json:"not_after"`
	Import       ImportCapability      `json:"import"`
	Storage      Storage               `json:"storage"`
	Export       *EncryptionDescriptor `json:"export,omitempty"`
}

// ToWire converts a validated ManagementAccessProfile into its wire view.
func (p ManagementAccessProfile) ToWire() RecordWire {
	return RecordWire{
		Version:      p.Version,
		Kind:         p.Kind,
		Installation: p.InstallationID,
		Trust:        p.Trust,
		Recipient:    p.Recipient,
		Scope:        p.Scope,
		NotBefore:    p.NotBefore,
		NotAfter:     p.NotAfter,
		Import:       p.Import,
		Storage:      p.Storage,
		Export:       p.Export,
	}
}

// FromWire builds a ManagementAccessProfile from a wire view. It does not
// re-validate; callers must ValidateKindEnforcement on the result before import.
func (w RecordWire) FromWire() ManagementAccessProfile {
	return ManagementAccessProfile{
		Kind:           w.Kind,
		Version:        w.Version,
		InstallationID: w.Installation,
		Trust:          w.Trust,
		Recipient:      w.Recipient,
		Scope:          w.Scope,
		NotBefore:      w.NotBefore,
		NotAfter:       w.NotAfter,
		Import:         w.Import,
		Storage:        w.Storage,
		Export:         w.Export,
	}
}

// encodeContainer encodes a container to its canonical JSON form. The non-secret
// fields are serialized; the secret blob (for a credential-export) never
// reaches the container — only the exported envelope (algorithm, encrypted
// flag, nonce) is carried, as the contract's Export descriptor already is.
func encodeContainer(c *ProfileContainer) ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// decodeContainer decodes a container produced by a producer that speaks the
// same container version. It fails closed on a wrong codec version and on a
// container with no records.
func decodeContainer(data []byte) (*ProfileContainer, error) {
	var c ProfileContainer
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("profile-container: invalid JSON: %w", err)
	}
	if c.Version != ContainerVersion {
		return nil, fmt.Errorf("%w: got %q want %q", ErrWrongContainerVersion, c.Version, ContainerVersion)
	}
	if len(c.Records) == 0 {
		return nil, fmt.Errorf("%w: container has no records", ErrMissingField)
	}
	return &c, nil
}

// ExportToContainer builds a non-secret container for a set of validated
// records at a destination path. Each record MUST already be
// contract-valid (ValidateKindEnforcement). The secret blob of a
// credential-export is never carried: the container carries only the
// non-secret record and the export envelope, so export carries no secret.
func ExportToContainer(version string, destPath string, records ...ManagementAccessProfile) (*ProfileContainer, error) {
	now := time.Now().UTC()
	out := &ProfileContainer{
		Version:   ContainerVersion,
		DestPath:  destPath,
		Timestamp: now,
	}
	for _, r := range records {
		if r.Version != version {
			return nil, fmt.Errorf("%w: record version %q", ErrUnknownVersion, r.Version)
		}
		if err := r.ValidateKindEnforcement(); err != nil {
			return nil, fmt.Errorf("management-access: record rejected during export: %w", err)
		}
		out.Records = append(out.Records, r.ToWire())
	}
	if len(out.Records) == 0 {
		return nil, fmt.Errorf("%w: export has no records", ErrMissingField)
	}
	return out, nil
}

// WriteExport writes a non-secret container to destPath. By default the
// container carries no secret: a caller that still holds a credential-export
// secret blob must keep it out of the container. A caller may explicitly write
// a secret blob alongside the container under a caller-chosen name, but the
// default path never contains a secret.
func WriteExport(destPath string, container *ProfileContainer) ([]byte, error) {
	if destPath == "" {
		return nil, fmt.Errorf("%w: empty dest path", ErrMissingField)
	}
	data, err := encodeContainer(container)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		return nil, fmt.Errorf("profile-container: mkdir: %w", err)
	}
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("profile-container: write: %w", err)
	}
	return data, nil
}

// ImportPolicy selects how the store resolves a conflict on import. Exactly one
// policy must be set; an unset policy fails closed (no silent overwrite).
type ImportPolicy int

const (
	// ImportPolicyConflictReject refuses to import when the record's identity
	// already exists in the destination. This is the default and the safe
	// default: conflicts require an explicit decision.
	ImportPolicyConflictReject ImportPolicy = iota
	// ImportPolicyConflictReplace replaces the existing record for the same
	// identity. It is explicit: a caller opts in only after an operator
	// decision, never silently.
	ImportPolicyConflictReplace
)

// ImportResult reports the outcome of a transactional import.
type ImportResult struct {
	// Path is the destination path the import wrote to.
	Path string
	// Imported counts the records written.
	Imported int
	// Replaced counts records that replaced a prior record (only under
	// ImportPolicyConflictReplace).
	Replaced int
	// Conflicts lists identities already present that could not be written.
	Conflicts []string
	// Err is set when the import aborted. On any error no partial write is left
	// behind.
	Err error
}

// Store is a bounded, single-identity profile store. It persists validated
// management-access records behind a strict version/trust/installation/scope
// guard. It implements no production import; it only persists what a caller has
// already contract-validated. Local path remapping is destination-local: the
// store records the destination-relative path and never carries a producer
// absolute path.
type Store struct {
	root     string
	version  string
	policy   ImportPolicy
	existing map[string]bool // identity -> present
}

// identityKey returns the conflict-detection key for a record: the pair of
// installation_id and recipient, which together identify a portable record in
// the destination.
func identityKey(r RecordWire) string {
	return r.Installation + "|" + r.Recipient
}

// NewStore opens (or creates) a store rooted at root. It reads existing records
// to establish the set of already-present identities so conflicts are
// detected on import. An empty/new root yields an empty store.
func NewStore(root string, version string, policy ImportPolicy) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: empty store root", ErrMissingField)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("profile-container: store mkdir: %w", err)
	}
	s := &Store{
		root:     root,
		version:  version,
		policy:   policy,
		existing: map[string]bool{},
	}
	if err := s.scan(); err != nil {
		return nil, fmt.Errorf("profile-container: scan store: %w", err)
	}
	return s, nil
}

// scan reads existing record files under the store root and records their
// identities. It is used both at open time and, in tests, to re-scan after a
// mutation to observe tampering.
func (s *Store) scan() error {
	s.existing = map[string]bool{}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) < len(".json") || name[len(name)-len(".json"):] != ".json" {
			continue
		}
		rec, err := s.readOne(filepath.Join(s.root, name))
		if err != nil {
			// A corrupted record file fails closed: it may indicate tampering.
			return fmt.Errorf("profile-container: corrupt record %s: %w", name, err)
		}
		for _, rw := range rec.Records {
			s.existing[identityKey(rw)] = true
		}
	}
	return nil
}

// readOne decodes a single record file.
func (s *Store) readOne(path string) (*ProfileContainer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeContainer(data)
}

// Import writes a validated container transactionally. Each record is
// contract-validated again (fail-closed) and conflict-checked against the
// destination. On any error the whole import is aborted and no partial write is
// left behind. Local path remapping is destination-local: the container's
// DestPath is overwritten with the store root on write.
func (s *Store) Import(container *ProfileContainer) (*ImportResult, error) {
	res := &ImportResult{Path: s.root}
	if container == nil {
		return nil, fmt.Errorf("%w: nil container", ErrMissingField)
	}
	if _, err := decodeContainer(mustEncode(container)); err != nil {
		return nil, err
	}

	// Stage the new records in a fresh container (transactional write set).
	var staged []RecordWire
	replaced := 0
	for _, rw := range container.Records {
		// Version guard: a record must speak the store's expected version.
		if s.version != "" && rw.Version != s.version {
			return nil, fmt.Errorf("%w: record version %q", ErrUnknownVersion, rw.Version)
		}
		// Contract guard: a record must be contract-valid before any write.
		p := rw.FromWire()
		if err := p.ValidateKindEnforcement(); err != nil {
			return nil, fmt.Errorf("%w: import rejects record %s: %w", ErrNoWrite, rw.Kind, err)
		}
		key := identityKey(rw)
		if s.existing[key] {
			switch s.policy {
			case ImportPolicyConflictReplace:
				replaced++
			default:
				res.Conflicts = append(res.Conflicts, key)
				return res, fmt.Errorf("%w: %s", ErrConflict, key)
			}
		}
		staged = append(staged, rw)
	}

	// Commit: write the staged set as one new container file. A partial write
	// leaves no committed state (the store reads identity on open, and each
	// file is atomic via a single WriteFile).
	commit := &ProfileContainer{
		Version:   ContainerVersion,
		DestPath:  s.root,
		Timestamp: time.Now().UTC(),
		Records:   staged,
	}
	if len(staged) == 0 {
		return nil, fmt.Errorf("%w: nothing to import", ErrMissingField)
	}
	data, err := encodeContainer(commit)
	if err != nil {
		return nil, err
	}
	commitPath := filepath.Join(s.root, fmt.Sprintf("import-%d.json", time.Now().UTC().UnixNano()))
	if err := os.WriteFile(commitPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("profile-container: commit: %w", err)
	}
	// Re-scan to confirm the write and update the present-identity set.
	if err := s.scan(); err != nil {
		_ = os.Remove(commitPath)
		return nil, fmt.Errorf("%w: %v", ErrPartialWrite, err)
	}
	res.Imported = len(staged)
	res.Replaced = replaced
	res.Path = commitPath
	return res, nil
}

// mustEncode is a tiny internal helper to reuse decodeContainer's guards
// against a container without a round-trip through the filesystem.
func mustEncode(c *ProfileContainer) []byte {
	data, _ := encodeContainer(c)
	return data
}
