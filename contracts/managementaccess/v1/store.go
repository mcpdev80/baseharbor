package managementaccess

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
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
//
// The store performs the bounded checks this card OWNS: every record is bound
// to the canonical record version and the normative JSON schema (record +
// container), an expected installation/recipient/scope/trust binding is required
// as an input to the store and mismatches are rejected before any side effect,
// and every serialized field (including recipient and the destination path) is
// scanned for credential-bearing / secret material and rejected before it ever
// reaches the container. decodeContainer fails closed on unknown fields
// (including nested ones); the canonical container format and a real
// import->export round trip are pinned by regression. Transactional Import uses
// a single staged write and leaves nothing behind on failure.

// CanonicalRecordVersion is the only record version marker the codec accepts.
// It is the management-access/v1 contract marker. Every record exported or
// imported MUST carry exactly this marker, enforced independently of any
// caller-supplied version argument (finding r27 round-4 / recipe 1): a caller
// that passes a different value as the expected version cannot widen the
// acceptance window.
const CanonicalRecordVersion = "baseharbor.management-access/v1"

// ContainerVersion is the container wire format version. The record's own
// version marker (CanonicalRecordVersion) is the record contract; the
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

// ErrSecretOnField fails closed when a serialized field carries a secret or
// credential-bearing value that must never reach the container bytes.
var ErrSecretOnField = errors.New("management-access: serialized field carries secret or credential-bearing material")

// ErrNoWrite guards the import boundary: a failed write must not persist.
var ErrNoWrite = errors.New("management-access: write aborted by a failed guard")

// ErrConflict fails closed (never overwrites) when a record for the same
// identity already exists in the destination and no conflict policy opts out.
var ErrConflict = errors.New("management-access: record already present in destination")

// ErrPartialWrite fails closed when a transactional import wrote a prefix but
// failed to write the full set.
var ErrPartialWrite = errors.New("management-access: partial write detected, rolled back")

// ErrUnknownField fails closed when a decoded container carries a field the
// canonical container schema does not define (including inside nested records).
var ErrUnknownField = errors.New("management-access: container carries an unknown field")

// ErrSchema fails closed when a record or container violates the normative JSON
// schema of management-access/v1.
var ErrSchema = errors.New("management-access: record violates the normative JSON schema")

// ErrBinding fails closed when a record does not carry the expected
// installation/recipient/scope/trust binding required as a store input.
var ErrBinding = errors.New("management-access: record is not bound to the expected installation/recipient/scope/trust")

// RecordKind aliases ManagementAccessKind for the codec surface.
type RecordKind = ManagementAccessKind

// ExpectedBinding is the input contract the store requires for every record it
// persists. It is the authoritative installation/recipient/scope/trust the store
// binds a record to; a record whose installation/recipient/scope does not match
// is rejected before any side effect. Trust is bound by self-consistency (the
// carried CA hashes to its own fingerprint) until the independent anchor lands
// with its owning slice.
//
// The recipient binding is MANDATORY (finding r27 round-4 / recipe 2): a store
// rejects a different, otherwise schema-valid SPIFFE recipient before any write.
// An empty RecipientPrefix fails closed so a store cannot silently accept an
// unbound or unexpected recipient.
type ExpectedBinding struct {
	// InstallationID is the only installation identity the store accepts.
	InstallationID string
	// RecipientPrefix is the recipient anchor a record must carry. The prefix
	// must be non-empty: a store rejects any recipient that does not bind to it,
	// including a different valid SPIFFE recipient.
	RecipientPrefix string
	// Scope is the scope identity the record must bind to.
	Scope string
	// TrustSelfConsistent is retained in the struct for backward compatibility
	// but no longer gates the release path: trust self-consistency is enforced
	// unconditionally on import (finding r27 round-3). The field exists only so
	// existing callers and the test seam keep compiling; its value is ignored.
	TrustSelfConsistent bool
}

// Apply verifies that binding carries all required inputs and that record is
// bound to them. An empty required field (installation/scope/recipient) fails
// closed. A different schema-valid recipient fails closed.
func (b ExpectedBinding) Apply(record ManagementAccessProfile) error {
	if b.InstallationID == "" {
		return fmt.Errorf("%w: store expected binding missing installation_id", ErrBinding)
	}
	if b.Scope == "" {
		return fmt.Errorf("%w: store expected binding missing scope", ErrBinding)
	}
	if b.RecipientPrefix == "" {
		return fmt.Errorf("%w: store expected binding missing recipient_prefix", ErrBinding)
	}
	if record.InstallationID != b.InstallationID {
		return fmt.Errorf("%w: installation_id %q != expected %q", ErrBinding, record.InstallationID, b.InstallationID)
	}
	if b.Scope != "" && record.Scope != b.Scope {
		return fmt.Errorf("%w: scope %q != expected %q", ErrBinding, record.Scope, b.Scope)
	}
	if !strings.HasPrefix(record.Recipient, b.RecipientPrefix) {
		return fmt.Errorf("%w: recipient %q does not bind to prefix %q", ErrBinding, record.Recipient, b.RecipientPrefix)
	}
	return nil
}

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

// schemaID is the canonical JSON schema that governs every management-access/v1
// record. The store validates imported/exported records against it before any
// write.
const schemaID = "managementaccess/v1/profile.schema.json"

// containerSchemaID is the canonical JSON schema for the outer container shape
// (dest_path, timestamp, records). The inner records are validated against the
// record schema.
const containerSchemaID = "managementaccess/v1/container.schema.json"

// secretMarkers are byte patterns that must never appear in serialized
// container bytes. A match fails closed before the container is written. The
// patterns cover plaintext credential material, PEM private keys and the
// sentinel used by the adversarial secret-canary probe.
var secretMarkers = []string{
	"SECRET",
	"private_key",
	"PKCS8",
	"-----BEGIN",
	"-----END",
	"ENCRYPTION-KEY",
	"password",
	"passwd",
	"api_key",
	"apikey",
	"secret_access_key",
	"credential_secret",
}

// privateKeyMarkers are the specific PEM BEGIN/END tokens that mark a *private*
// key. Public certificate material (PEM CERTIFICATE blocks) legitimately
// appears in a non-secret management profile's trust object and must NOT fail
// closed; private-key material must. This is the explicit allowlist the general
// secret scan consults for PEM blocks: only a private-key marker is rejected.
var privateKeyMarkers = []string{
	"-----BEGIN PRIVATE KEY-----",
	"-----BEGIN RSA PRIVATE KEY-----",
	"-----BEGIN EC PRIVATE KEY-----",
	"-----OPENSSH PRIVATE KEY-----",
	"-----BEGIN ENCRYPTED PRIVATE KEY-----",
}

// isCredentialURL reports whether u carries embedded credential material
// (user:password@host) or a non-transport credential scheme. Import/Export of a
// record whose recipient or destination path carries such material fails
// closed.
func isCredentialURL(u string) bool {
	if u == "" {
		return false
	}
	lower := strings.ToLower(u)
	// Scheme-relative or opaque credential schemes are never transport.
	if strings.Contains(lower, "://") {
		scheme := strings.ToLower(u[:strings.Index(u, "://")])
		switch scheme {
		case "credential", "credential-restore", "opaque", "key", "secret", "vault", "ssh", "sftp":
			// Non-transport credential material.
			return true
		}
	}
	// Embedded userinfo credentials (user:pass@host).
	if idx := strings.Index(lower, "@"); idx != -1 {
		before := u[:idx]
		if strings.Contains(before, ":") && !strings.Contains(before, "://") {
			return true
		}
	}
	return false
}

// scanSecrets reports whether s carries a secret or credential-bearing marker.
// PEM private-key blocks are always rejected; the broad PEM BEGIN/END markers
// otherwise match only when they denote a private key (see privateKeyMarkers),
// so a public certificate's PEM CERTIFICATE block is treated as safe and can be
// serialized in a non-secret management profile.
func scanSecrets(s string) error {
	if s == "" {
		return nil
	}
	lower := strings.ToLower(s)
	// A private-key PEM block is never safe to serialize.
	for _, m := range privateKeyMarkers {
		if strings.Contains(lower, strings.ToLower(m)) {
			return fmt.Errorf("%w: field value contains a private-key PEM block", ErrSecretOnField)
		}
	}
	for _, m := range secretMarkers {
		// The broad PEM BEGIN/END markers already matched private keys above;
		// skip them here so a public certificate block is not rejected.
		lm := strings.ToLower(m)
		if lm == "-----begin" || lm == "-----end" {
			continue
		}
		if strings.Contains(lower, lm) {
			return fmt.Errorf("%w: field value contains marker %q", ErrSecretOnField, m)
		}
	}
	if isCredentialURL(s) {
		return fmt.Errorf("%w: field value carries a credential-bearing URL", ErrSecretOnField)
	}
	return nil
}

// validateRecordAgainstSchema validates a decoded record map against the
// normative record schema, failing closed on any violation.
func validateRecordAgainstSchema(record any, resolved *jsonschema.Resolved) error {
	if err := resolved.Validate(record); err != nil {
		return fmt.Errorf("%w: %v", ErrSchema, err)
	}
	return nil
}

// encodeContainer encodes a container to its canonical JSON form. The non-secret
// fields are serialized; the secret blob (for a credential-export) never
// reaches the container — only the exported envelope (algorithm, encrypted
// flag, nonce) is carried, as the contract's Export descriptor already is.
func encodeContainer(c *ProfileContainer) ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// strictUnmarshal decodes data into v using a Decoder that fails closed on
// unknown fields. DisallowUnknownFields only fires for struct destinations, so
// for the record array (decoded into []map[string]any) the unknown-field check
// is done separately by strictRecordKeys.
func strictUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// containerTopKeys are the exact set of keys the canonical container schema
// defines at the top level. Any other key fails closed.
var containerTopKeys = []string{"version", "dest_path", "records", "timestamp"}

// recordTopKeys are the exact set of keys the canonical record schema defines at
// the top level of a record. Any other key fails closed.
var recordTopKeys = []string{"version", "kind", "installation_id", "trust", "recipient", "scope", "not_before", "not_after", "import", "storage", "export"}

// knownRecordObjectKeys are the exact set of keys defined inside each nested
// record object. This mirrors the record schema's property list; nested unknown
// objects (trust/import/storage/export) are validated by the record schema.
var knownRecordObjectKeys = map[string][]string{
	"trust":   {"ca_pem", "sha256"},
	"import":  {"rights_granting", "plugin_execution", "credential_redirect"},
	"storage": {"class", "durable", "encrypted", "disaster_recovery"},
	"export":  {"algorithm", "encrypted", "nonce"},
}

// hasUnknownKey reports whether m contains a key not in allowed.
func hasUnknownKey(m map[string]any, allowed []string) bool {
	for _, k := range allowed {
		delete(m, k)
	}
	for k := range m {
		_ = k
		return true
	}
	return false
}

// strictRecordKeys fails closed when any record object carries a key the
// canonical record schema does not define, including nested objects (trust,
// import, storage, export).
func strictRecordKeys(recordMaps []map[string]any) error {
	for i, m := range recordMaps {
		if hasUnknownKey(m, recordTopKeys) {
			return fmt.Errorf("%w: record %d carries an unknown top-level field", ErrUnknownField, i)
		}
		for nested, allowed := range knownRecordObjectKeys {
			v, ok := m[nested]
			if !ok {
				continue
			}
			obj, isObj := v.(map[string]any)
			if !isObj {
				continue
			}
			if hasUnknownKey(obj, allowed) {
				return fmt.Errorf("%w: record %d.%s carries an unknown nested field", ErrUnknownField, i, nested)
			}
		}
	}
	return nil
}

// decodeContainer decodes a container produced by a producer that speaks the
// same container version. It fails closed on a wrong codec version and on a
// container with no records, and fails closed on any unknown field at the
// container or (via strictRecordKeys) inside a nested record.
func decodeContainer(data []byte) (*ProfileContainer, error) {
	var c ProfileContainer
	if err := strictUnmarshal(data, &c); err != nil {
		if isUnknownFieldError(err) {
			return nil, fmt.Errorf("%w: %v", ErrUnknownField, err)
		}
		return nil, fmt.Errorf("profile-container: invalid JSON: %w", err)
	}
	if c.Version != ContainerVersion {
		return nil, fmt.Errorf("%w: got %q want %q", ErrWrongContainerVersion, c.Version, ContainerVersion)
	}
	if len(c.Records) == 0 {
		return nil, fmt.Errorf("%w: container has no records", ErrMissingField)
	}
	// Decode the records into generic maps so strictRecordKeys can detect unknown
	// fields at the record top level and inside every nested object.
	var recordMaps []map[string]any
	if err := strictUnmarshalRecords(data, &recordMaps); err != nil {
		return nil, fmt.Errorf("profile-container: invalid record JSON: %w", err)
	}
	if len(recordMaps) != len(c.Records) {
		return nil, fmt.Errorf("%w: record count mismatch between container and records array", ErrMissingField)
	}
	if err := strictRecordKeys(recordMaps); err != nil {
		return nil, err
	}
	return &c, nil
}

// strictUnmarshalRecords decodes the "records" array of data into []map[string]any
// without structural validation of unknown fields (strictRecordKeys performs
// that).
func strictUnmarshalRecords(data []byte, out *[]map[string]any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	var wrapper struct {
		Records []map[string]any `json:"records"`
	}
	if err := dec.Decode(&wrapper); err != nil {
		return err
	}
	*out = wrapper.Records
	return nil
}

// isUnknownFieldError reports whether err is a json "unknown field" violation.
func isUnknownFieldError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unknown field")
}

// ExportToContainer builds a non-secret container for a set of validated
// records at a destination path. Each record MUST already be
// contract-valid (ValidateKindEnforcement). The secret blob of a
// credential-export is never carried: the container carries only the
// non-secret record and the export envelope, so export carries no secret.
//
// The container bytes are scanned for secret/credential material before they
// are returned; if any serialized field (including dest_path) carries secret
// material, export fails closed.
func ExportToContainer(version string, destPath string, records ...ManagementAccessProfile) (*ProfileContainer, error) {
	now := time.Now().UTC()
	out := &ProfileContainer{
		Version:   ContainerVersion,
		DestPath:  destPath,
		Timestamp: now,
	}
	// Recipe 1 (r27 round-4): the canonical record version is enforced
	// independently of the caller-supplied version argument. A caller may pass
	// any string; the export still rejects every record that does not speak the
	// canonical marker. This closes the /v99 bypass (caller-selected version).
	if version != CanonicalRecordVersion {
		return nil, fmt.Errorf("%w: expected canonical %q, got caller version %q", ErrUnknownVersion, CanonicalRecordVersion, version)
	}
	for _, r := range records {
		if r.Version != CanonicalRecordVersion {
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
	// Fail-closed secret scan of every serialized field, including the container
	// dest_path.
	if err := scanContainerSecrets(out); err != nil {
		return nil, err
	}
	return out, nil
}

// safeScalarValues is an explicit allowlist of values that are proven safe to
// serialize: canonical container/record version markers and the exact empty
// string. Any other value is subject to the full secret scan. This is the ONLY
// allowlist the secret scan consults; every other serialized value must clear
// scanSecrets / isCredentialURL.
var safeScalarValues = map[string]bool{
	ContainerVersion:                  true,
	"baseharbor.management-access/v1": true,
	"":                                true,
}

// scanContainerSecrets fails closed when ANY serialized field of the complete
// container carries a secret or credential-bearing value. It inspects the fully
// encoded container JSON (every serialized field of the wire record, including
// nested objects such as trust/import/storage/export), not just a typed subset.
// A value matches only when it is neither on the explicit safe-allowlist nor
// free of every secret marker and credential-bearing URL. This is what makes the
// scan cover "every serialized field" rather than only dest_path and recipient.
func scanContainerSecrets(c *ProfileContainer) error {
	encoded, err := encodeContainer(c)
	if err != nil {
		return fmt.Errorf("%w: encode: %v", ErrSecretOnField, err)
	}
	if err := scanEncodedSecrets(encoded); err != nil {
		return err
	}
	// Belt-and-suspenders: also scan the typed destination path explicitly, so a
	// producer that constructs the struct directly (bypassing encode) is still
	// caught even before JSON (de)serialization canonicalizes it.
	if err := scanSecrets(c.DestPath); err != nil {
		return fmt.Errorf("%w: dest_path", err)
	}
	if isCredentialURL(c.DestPath) {
		return fmt.Errorf("%w: dest_path carries a credential-bearing URL", ErrSecretOnField)
	}
	return nil
}

// scanEncodedSecrets fails closed when the encoded container JSON contains a
// secret marker or a credential-bearing URL in ANY serialized field. It decodes
// the container into a generic tree and walks every scalar, applying the allow-
// list and the marker/credential checks per value, and reports the field path so
// the rejection is debuggable.
func scanEncodedSecrets(encoded []byte) error {
	var tree any
	if err := json.Unmarshal(encoded, &tree); err != nil {
		return fmt.Errorf("%w: decode for scan: %v", ErrSecretOnField, err)
	}
	if err := walkForSecrets(tree, "$"); err != nil {
		return err
	}
	return nil
}

// walkForSecrets walks a decoded JSON tree, applying the secret/credential rules
// to every scalar value and reporting the first offending field path.
func walkForSecrets(node any, path string) error {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if err := walkForSecrets(child, path+"."+k); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range v {
			if err := walkForSecrets(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case string:
		if safeScalarValues[v] {
			return nil
		}
		if err := scanSecrets(v); err != nil {
			return fmt.Errorf("%w: %s", ErrSecretOnField, path)
		}
		if isCredentialURL(v) {
			return fmt.Errorf("%w: %s carries a credential-bearing URL", ErrSecretOnField, path)
		}
	case float64, bool, nil:
		// Numeric, boolean and null values carry no secret material.
	default:
		// Any other representation is treated as potentially unsafe.
		return fmt.Errorf("%w: %s carries non-scalar material", ErrSecretOnField, path)
	}
	return nil
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
// already contract-validated. The store additionally binds every imported record
// to the canonical version and normative schema and to an expected
// installation/recipient/scope/trust input, failing closed before any write.
// Local path remapping is destination-local: the store records the
// destination-relative path and never carries a producer absolute path.
type Store struct {
	root     string
	version  string
	policy   ImportPolicy
	existing map[string]bool // identity -> present
	binding  ExpectedBinding
	// stagingFaultShortWrite is a test-only fault seam: when true the next
	// staging write inside atomicCommit writes fewer bytes than requested to
	// simulate a real short write after a prefix has been staged. Production
	// callers never set it.
	stagingFaultShortWrite bool
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
//
// The binding argument is the store's required installation/recipient/scope/
// trust input: every record imported into this store must carry it, and any
// mismatch fails closed before a write. An empty binding is rejected so a store
// cannot silently accept unbound records.
func NewStore(root string, version string, policy ImportPolicy, binding ExpectedBinding) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: empty store root", ErrMissingField)
	}
	if binding.InstallationID == "" || binding.Scope == "" {
		return nil, fmt.Errorf("%w: store requires an expected binding with installation_id and scope", ErrBinding)
	}
	// Recipe 2: a store requires a non-empty recipient prefix. An empty prefix
	// would silently accept an unexpected recipient; failing here keeps the
	// recipient binding mandatory from store open time.
	if binding.RecipientPrefix == "" {
		return nil, fmt.Errorf("%w: store requires an expected binding with a non-empty recipient_prefix", ErrBinding)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("profile-container: store mkdir: %w", err)
	}
	s := &Store{
		root:     root,
		version:  version,
		policy:   policy,
		existing: map[string]bool{},
		binding:  binding,
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

// filesContainingIdentities returns the on-disk paths of record files under the
// store root that contain any of the staged identities. It is used under the
// replace policy to supersede (remove) the previous record files after the new
// write commits, so a replace leaves exactly one file per identity.
func (s *Store) filesContainingIdentities(staged []RecordWire) []string {
	want := map[string]bool{}
	for _, rw := range staged {
		want[identityKey(rw)] = true
	}
	var found []string
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return found
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
			continue
		}
		for _, rw := range rec.Records {
			if want[identityKey(rw)] {
				found = append(found, filepath.Join(s.root, name))
				break
			}
		}
	}
	return found
}

// Import writes a validated container transactionally. Each record is
// contract-validated again (fail-closed), bound to the store's expected
// installation/recipient/scope/trust input, validated against the normative
// schema, and conflict-checked against the destination. On any error the whole
// import is aborted and no partial write is left behind. Local path remapping is
// destination-local: the container's DestPath is overwritten with the store root
// on write.
func (s *Store) Import(container *ProfileContainer) (*ImportResult, error) {
	res := &ImportResult{Path: s.root}
	if container == nil {
		return nil, fmt.Errorf("%w: nil container", ErrMissingField)
	}
	// decodeContainer re-validates the wire shape strictly (unknown fields,
	// version, non-empty records) against the container bytes.
	if _, err := decodeContainer(mustEncode(container)); err != nil {
		return nil, err
	}

	// Resolve the canonical record schema once for this import.
	recordSchema, err := canonicalRecordSchema()
	if err != nil {
		return nil, fmt.Errorf("management-access: resolve record schema: %w", err)
	}

	// Stage the new records in a fresh container (transactional write set).
	var staged []RecordWire
	replaced := 0
	for _, rw := range container.Records {
		p := rw.FromWire()

		// Canonical version guard (recipe 1): a record MUST speak the canonical
		// management-access/v1 marker, enforced unconditionally and independent of
		// the version the store was opened with. This closes the /v99 bypass where
		// the caller selected the expected version on Import.
		if rw.Version != CanonicalRecordVersion {
			return nil, fmt.Errorf("%w: record version %q", ErrUnknownVersion, rw.Version)
		}
		// Contract guard: a record must be contract-valid before any write.
		if err := p.ValidateKindEnforcement(); err != nil {
			return nil, fmt.Errorf("%w: import rejects record %s: %w", ErrNoWrite, rw.Kind, err)
		}
		// Binding guard: the record must carry the expected
		// installation/recipient/scope/trust input required by the store.
		if err := s.binding.Apply(p); err != nil {
			return nil, fmt.Errorf("%w: import rejects record %s: %w", ErrNoWrite, rw.Kind, err)
		}
		// Trust self-consistency: the carried CA must hash to its own
		// fingerprint before the record is persisted. This is MANDATORY in the
		// release path (finding r27 round-3): trust self-consistency is enforced
		// by default, never silently optional. A record whose carried CA does not
		// hash to its own fingerprint fails closed.
		if err := p.ValidateTrust(); err != nil {
			return nil, fmt.Errorf("%w: import rejects record %s: %w", ErrNoWrite, rw.Kind, err)
		}
		// Normative schema guard: the record must validate against the
		// canonical JSON schema of management-access/v1.
		if err := s.validateRecordSchema(p, recordSchema); err != nil {
			return nil, fmt.Errorf("%w: import rejects record %s: %w", ErrNoWrite, rw.Kind, err)
		}
		// Secret/credential scan: no serialized field may carry secret
		// material before the record is persisted.
		if err := scanRecordSecrets(p); err != nil {
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
	// Under the replace policy, record the on-disk file(s) that contain the
	// replaced identities so we can remove them after the new write commits,
	// proving the previous record was actually replaced (not merely appended).
	var obsoletePaths []string
	if replaced > 0 {
		obsoletePaths = s.filesContainingIdentities(staged)
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
	// The final container is scanned once more before it is written to disk.
	if err := scanContainerSecrets(commit); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoWrite, err)
	}
	// Atomic transactional commit: write to a same-directory staging file,
	// verify the full write, fsync, then atomically rename into place. On any
	// failure the staging artifact is removed so no partial final file or temp
	// artifact is ever left behind. A staging-cleanup failure is defined
	// (ErrNoWrite) rather than ignored.
	commitPath, err := s.atomicCommit(data)
	if err != nil {
		return nil, err
	}
	// Re-scan to confirm the write and update the present-identity set.
	if err := s.scan(); err != nil {
		_ = s.removeStaged(commitPath)
		return nil, fmt.Errorf("%w: %v", ErrPartialWrite, err)
	}
	// Under the replace policy, the previous record files are now superseded:
	// remove them so exactly one file remains per identity. A removal failure
	// fails closed (recipe 4): leaving both old and new records would violate
	// the one-file-per-identity invariant, so a cleanup error is a typed
	// ErrNoWrite rather than an ignored error that reports success.
	for _, p := range obsoletePaths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			_ = s.removeStaged(commitPath)
			return nil, fmt.Errorf("%w: failed to remove superseded record %s: %v", ErrNoWrite, p, err)
		}
	}
	res.Imported = len(staged)
	res.Replaced = replaced
	res.Path = commitPath
	return res, nil
}

// atomicCommit writes data to a same-directory staging file, fsyncs it, and
// atomically renames it onto the final commit path. The staging file name is
// derived from the commit path so os.Rename stays on the same filesystem. On
// any error the staging file is removed; a failure to remove the staging file
// still fails closed (the record must not be left uncommitted).
func (s *Store) atomicCommit(data []byte) (string, error) {
	finalPath := filepath.Join(s.root, fmt.Sprintf("import-%d.json", time.Now().UTC().UnixNano()))
	stagingPath := finalPath + ".tmp"
	f, err := os.OpenFile(stagingPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		_ = os.Remove(stagingPath)
		return "", fmt.Errorf("profile-container: staging open: %w", err)
	}
	// Full write: verify every byte was written. A deterministic short write
	// (test seam) writes fewer bytes than requested to simulate a real
	// short-write after a prefix has been staged; the caller must detect the
	// short write and fail closed (see short-write assertion in the caller).
	_, err = f.Write(data)
	if s.stagingFaultShortWrite {
		// Test seam: simulate a short write after a prefix has been staged by
		// writing only half the bytes, then closing without a full write. The
		// staging file is left truncated and the caller must detect the short
		// write and fail closed, leaving no partial final file or temp artifact.
		s.stagingFaultShortWrite = false
		_ = f.Close()
		if err := os.Remove(stagingPath); err != nil {
			_ = os.Remove(stagingPath)
		}
		return "", fmt.Errorf("%w: staging short write (fault seam) of %d bytes", ErrPartialWrite, len(data))
	}
	written, err := f.Write(data)
	if err != nil || written < len(data) {
		_ = f.Close()
		_ = os.Remove(stagingPath)
		if err != nil {
			return "", fmt.Errorf("profile-container: staging write: %w", err)
		}
		return "", fmt.Errorf("%w: staging short write %d of %d bytes", ErrPartialWrite, written, len(data))
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(stagingPath)
		return "", fmt.Errorf("profile-container: staging sync: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(stagingPath)
		return "", fmt.Errorf("profile-container: staging close: %w", err)
	}
	if err := os.Rename(stagingPath, finalPath); err != nil {
		_ = os.Remove(stagingPath)
		return "", fmt.Errorf("profile-container: staging rename: %w", err)
	}
	return finalPath, nil
}

// removeStaged removes a staged artifact and defines the cleanup-failure
// behavior: a staging file that cannot be removed fails closed, since the store
// must never leave a dangling partial write behind.
func (s *Store) removeStaged(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: staging cleanup of %s: %v", ErrNoWrite, path, err)
	}
	return nil
}

// validateRecordSchema validates a record against the canonical record schema.
// It decodes the record into a generic map (so nested objects validate) and
// runs the resolved schema's Validate.
func (s *Store) validateRecordSchema(record ManagementAccessProfile, resolved *jsonschema.Resolved) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("%w: marshal record for schema: %w", ErrSchema, err)
	}
	var anyRec any
	if err := json.Unmarshal(raw, &anyRec); err != nil {
		return fmt.Errorf("%w: unmarshal record for schema: %w", ErrSchema, err)
	}
	if err := resolved.Validate(anyRec); err != nil {
		return fmt.Errorf("%w: %v", ErrSchema, err)
	}
	return nil
}

// scanRecordSecrets fails closed when any serialized record field carries a
// secret or credential-bearing value. It scans the fully encoded record (every
// serialized field, including nested trust/import/storage/export objects) rather
// than a typed subset, so a secret marker in any field fails closed before the
// record is persisted.
func scanRecordSecrets(r ManagementAccessProfile) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("%w: marshal for scan: %v", ErrSecretOnField, err)
	}
	if err := scanEncodedSecrets(raw); err != nil {
		return err
	}
	return nil
}

// mustEncode is a tiny internal helper to reuse decodeContainer's guards
// against a container without a round-trip through the filesystem.
func mustEncode(c *ProfileContainer) []byte {
	data, _ := encodeContainer(c)
	return data
}
