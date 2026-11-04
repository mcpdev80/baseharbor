// Test seams (package managementaccess internal test): small, deliberately-named
// (Test suffix) exports that let the component tests exercise the codec and
// store internals without duplicating logic. They carry the Test suffix so
// production callers (and reviewers) can tell they are test-only and never
// import them in shipped code.

package managementaccess

import (
	"io"
	"os"
	"path/filepath"
)

// DecodeContainerForTest decodes a container from bytes, exposing the package
// internal decodeContainer guard (wrong codec version / empty record set) to
// tests.
func DecodeContainerForTest(data []byte) (*ProfileContainer, error) {
	return decodeContainer(data)
}

// ExportContainerJSONForTest encodes a container to its canonical JSON bytes,
// exposing encodeContainer to tests.
func ExportContainerJSONForTest(c *ProfileContainer) ([]byte, error) {
	return encodeContainer(c)
}

// IdentityKeyForTest returns the conflict-detection key for a record, exposing
// identityKey to tests.
func IdentityKeyForTest(r RecordWire) string {
	return identityKey(r)
}

// NewStoreForTest opens a store; exposed so tests that need a fresh open after a
// mutation can do so without reaching into internal fields. The binding is the
// store's required installation/recipient/scope/trust input.
func NewStoreForTest(root string, version string, policy ImportPolicy, binding ExpectedBinding) (*Store, error) {
	return NewStore(root, version, policy, binding)
}

// ExpectedBindingForTest builds the canonical store binding for a valid record,
// so tests can request a matching or intentionally-mismatched binding.
func ExpectedBindingForTest(rec ManagementAccessProfile, selfConsistent bool) ExpectedBinding {
	return ExpectedBinding{
		InstallationID:      rec.InstallationID,
		RecipientPrefix:     "spiffe://baseharbor/platform/core/",
		Scope:               rec.Scope,
		TrustSelfConsistent: selfConsistent,
	}
}

// ScanForTest re-runs the store's on-disk scan, exposing scan to tests.
func (s *Store) ScanForTest() error {
	return s.scan()
}

// IdentityPresentForTest reports whether the given identity key is currently
// present in the store after a scan. Exposed to tests.
func (s *Store) IdentityPresentForTest(key string) bool {
	return s.existing[key]
}

// ImportFromTestBytes decodes a container from bytes and imports it, letting
// tests exercise import against a container built outside ExportToContainer
// (e.g. a tampered one).
func (s *Store) ImportFromTestBytes(data []byte) (*ImportResult, error) {
	c, err := decodeContainer(data)
	if err != nil {
		return nil, err
	}
	return s.Import(c)
}

// ImportWithBindingForTest opens a store with an arbitrary binding and imports a
// container built outside ExportToContainer, so tests can exercise the
// installation/recipient/scope/trust binding guard directly.
func ImportWithBindingForTest(root string, version string, policy ImportPolicy, binding ExpectedBinding, data []byte) (*ImportResult, error) {
	store, err := NewStore(root, version, policy, binding)
	if err != nil {
		return nil, err
	}
	c, err := decodeContainer(data)
	if err != nil {
		return nil, err
	}
	return store.Import(c)
}

// ImportForTest imports a container built outside ExportToContainer directly, so
// tests can exercise import against a tampered or hand-built container.
func (s *Store) ImportForTest(container *ProfileContainer) (*ImportResult, error) {
	return s.Import(container)
}

// SetStagingWriteForTest replaces the one staging write for deterministic
// short-write tests. Passing nil restores the production file writer.
func (s *Store) SetStagingWriteForTest(write func(io.Writer, []byte) (int, error)) {
	s.stagingWrite = write
}

// SetRemoveObsoleteForTest replaces the obsolete-record removal step so tests
// can inject a deterministic replacement-cleanup failure. Passing nil restores
// os.Remove.
func (s *Store) SetRemoveObsoleteForTest(remove func(string) error) {
	s.removeObsolete = remove
}

// WriteRawContainerForTest writes a raw container bytes to a file under root
// without validation, letting tests simulate a record that reaches the import
// boundary already on disk (e.g. a tampered or version-mismatched record).
func WriteRawContainerForTest(root string, data []byte) (string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(root, "raw-import.json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	return p, nil
}
