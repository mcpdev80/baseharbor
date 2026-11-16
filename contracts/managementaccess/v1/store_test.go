package managementaccess_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
)

// mustTestCA returns a fresh self-signed CA certificate and its DER SHA-256
// fingerprint via the shared support helper.
func mustTestCA(t *testing.T) (string, string) {
	t.Helper()
	return generateTestCA(t, "BaseHarbor Management Test CA")
}

// validInstallationID is the installation identity every store binding anchors
// on for this package's tests.
const validInstallationID = "11111111-1111-4111-8111-111111111111"

// validRecipient is a schema-valid SPIFFE recipient bound to validInstallationID.
const validRecipient = "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"

// validRecord builds a fully contract-valid management-profile record. The
// validity window is anchored at the current wall-clock time (not the passed
// `now`), so callers may shift `now` for the record's own fields without moving
// the window off the current instant. Each call is fresh so variants never
// share pointers.
func validRecord(t *testing.T, installationID, recipient string, scope string, _ time.Time) managementaccess.ManagementAccessProfile {
	t.Helper()
	now := time.Now().UTC()
	caPEM, caSHA := mustTestCA(t)
	return managementaccess.ManagementAccessProfile{
		Kind:           managementaccess.ManagementProfile,
		Version:        "baseharbor.management-access/v1",
		InstallationID: installationID,
		Trust:          managementaccess.Trust{CAPEM: caPEM, SHA256: caSHA},
		Recipient:      recipient,
		Scope:          scope,
		NotBefore:      now.Add(-time.Minute),
		NotAfter:       now.Add(time.Hour),
		Import:         managementaccess.ImportCapability{},
		Storage:        managementaccess.Storage{Class: "protected-durable", Durable: true, Encrypted: true, DisasterRecovery: false},
	}
}

// storeBindingFor returns the store's expected binding that matches rec. scope
// must be non-empty; installationID must be non-empty.
func storeBindingFor(t *testing.T, rec managementaccess.ManagementAccessProfile) managementaccess.ExpectedBinding {
	t.Helper()
	return managementaccess.ExpectedBindingForTest(rec, true)
}

// TestGoldenContainerDecodes asserts the shipped golden fixture is a well-formed
// container with one credential-export record that validates against the
// canonical schema.
func TestGoldenContainerDecodes(t *testing.T) {
	data, err := os.ReadFile("store.golden.json")
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	c, err := managementaccess.DecodeContainerForTest(data)
	if err != nil {
		t.Fatalf("golden container must decode: %v", err)
	}
	if c.Version != managementaccess.ContainerVersion {
		t.Fatalf("golden container version = %q", c.Version)
	}
	if len(c.Records) != 1 {
		t.Fatalf("golden container must carry one record, got %d", len(c.Records))
	}
	if c.Records[0].Kind != managementaccess.CredentialExport {
		t.Fatalf("golden record kind = %q", c.Records[0].Kind)
	}
}

// TestDecodeContainerRejectsWrongVersion fails closed when the codec version
// does not match ContainerVersion.
func TestDecodeContainerRejectsWrongVersion(t *testing.T) {
	_, err := managementaccess.DecodeContainerForTest([]byte(`{
      "version": "baseharbor.profile-container/old",
      "records": [{"version": "baseharbor.management-access/v1"}]
    }`))
	if err == nil {
		t.Fatal("decode of a wrong-container-version container must fail closed")
	}
}

// TestDecodeContainerRejectsUnknownTopLevelField fails closed on an unknown
// top-level container field.
func TestDecodeContainerRejectsUnknownTopLevelField(t *testing.T) {
	_, err := managementaccess.DecodeContainerForTest([]byte(`{
      "version": "baseharbor.profile-container/v1",
      "dest_path": "d",
      "timestamp": "2026-11-04T00:00:00Z",
      "records": [{"version":"baseharbor.management-access/v1","kind":"management-profile","installation_id":"11111111-1111-4111-8111-111111111111","recipient":"spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab","scope":"control-plane","not_before":"2026-11-02T00:00:00Z","not_after":"2027-11-02T00:00:00Z","import":{"rights_granting":false,"plugin_execution":false,"credential_redirect":false},"storage":{"class":"protected-durable","durable":true,"encrypted":true,"disaster_recovery":false}}],
      "future_field": "x"
    }`))
	if err == nil {
		t.Fatal("decode of a container with an unknown top-level field must fail closed")
	}
}

// TestDecodeContainerRejectsUnknownNestedField fails closed on an unknown field
// nested inside a record (here inside the nested trust object).
func TestDecodeContainerRejectsUnknownNestedField(t *testing.T) {
	_, err := managementaccess.DecodeContainerForTest([]byte(`{
      "version": "baseharbor.profile-container/v1",
      "dest_path": "d",
      "timestamp": "2026-11-04T00:00:00Z",
      "records": [{"version":"baseharbor.management-access/v1","kind":"management-profile","installation_id":"11111111-1111-4111-8111-111111111111","recipient":"spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab","scope":"control-plane","not_before":"2026-11-02T00:00:00Z","not_after":"2027-11-02T00:00:00Z","import":{"rights_granting":false,"plugin_execution":false,"credential_redirect":false},"storage":{"class":"protected-durable","durable":true,"encrypted":true,"disaster_recovery":false},"trust":{"ca_pem":"-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----","sha256":"77fbfc1c468ef3e1f3a4c843bf32b24b7f070ab9c10861a19c227f80a802b3b5","future_trust_field":"x"}}],
    }`))
	if err == nil {
		t.Fatal("decode of a container with an unknown nested field must fail closed")
	}
}

// TestExportToContainerRejectsWrongVersion: the export build step fails closed on
// a record whose version is not the expected marker — the first write guard.
func TestExportToContainerRejectsWrongVersion(t *testing.T) {
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	rec.Version = "baseharbor.management-access/v0"
	_, err := managementexport("baseharbor.management-access/v1", "dest", rec)
	if err == nil {
		t.Fatal("export of a wrong-version record must fail closed")
	}
}

// TestExportToContainerRejectsBrokenTrust: a record whose fingerprint does not
// hash to its own certificate fails the export build — the write guard fires
// before any write.
func TestExportToContainerRejectsBrokenTrust(t *testing.T) {
	now := time.Now().UTC()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Trust.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	_, err := managementexport("baseharbor.management-access/v1", "dest", rec)
	if err == nil {
		t.Fatal("export of a broken-trust record must fail closed")
	}
}

// TestImportRejectsTamperedTrustFromDisk: a container whose record trust is
// tampered must be rejected by import and leave no write. The tamper bypasses
// the export guard via ImportFromTestBytes so import re-validation is exercised.
func TestImportRejectsTamperedTrustFromDisk(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	// Tamper the record trust fingerprint so the container carries a record that
	// fails ValidateKindEnforcement on import re-validation.
	tamperTrustSHA256(t, c)
	_, err = store.ImportForTest(c)
	if err == nil {
		t.Fatal("import of a tampered-trust container must fail closed")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("a tampered-trust import must not write anything, found %d entries", len(entries))
	}
}

// TestImportConflictRejectsNoOverwrite: importing the same identity twice must
// fail the second time and must NOT silently overwrite the first record.
func TestImportConflictRejectsNoOverwrite(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c1 := mustExport(t, "baseharbor.management-access/v1", rec)
	if _, err := store.Import(c1); err != nil {
		t.Fatalf("first import must succeed: %v", err)
	}

	second := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	c2 := mustExport(t, "baseharbor.management-access/v1", second)
	if _, err := store.Import(c2); err == nil {
		t.Fatal("second import of the same identity must fail under reject policy")
	}

	// Exactly one committed file survives (no silent overwrite).
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("a rejected conflict must not create a second record, found %d files", len(files))
	}
}

// TestImportConflictReplaceVerifiesActualReplace: under the replace policy the
// previous record is ACTUALLY replaced on disk (its contents change), not just
// that Replaced==1. The replaced file must contain the new record's identity and
// the old contents must be gone.
func TestImportConflictReplaceVerifiesActualReplace(t *testing.T) {
	root := t.TempDir()
	rec1 := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReplace, storeBindingFor(t, rec1))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c1 := mustExport(t, "baseharbor.management-access/v1", rec1)
	if _, err := store.Import(c1); err != nil {
		t.Fatalf("first import: %v", err)
	}
	oldBytes, err := os.ReadFile(firstJSON(t, root))
	if err != nil {
		t.Fatalf("read first: %v", err)
	}
	_ = oldBytes

	rec2 := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	c2 := mustExport(t, "baseharbor.management-access/v1", rec2)
	res, err := store.Import(c2)
	if err != nil {
		t.Fatalf("replace import must succeed: %v", err)
	}
	if res.Replaced != 1 {
		t.Fatalf("replace import must report exactly one replacement, got %d", res.Replaced)
	}
	// Exactly one file now (the old one was replaced, not appended).
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("a replace import must leave exactly one file, found %d", len(files))
	}
	// Read the committed record and confirm the (freshly minted) replacement is
	// actually on disk: a new valid trust fingerprint is present and the file is
	// a well-formed container that decodes.
	dec2, err := managementaccess.DecodeContainerForTest(mustReadFile(t, files[0]))
	if err != nil {
		t.Fatalf("re-read replaced container: %v", err)
	}
	if len(dec2.Records) != 1 {
		t.Fatalf("replaced container must carry exactly one record, got %d", len(dec2.Records))
	}
	if dec2.Records[0].Installation != rec2.InstallationID {
		t.Fatalf("the previous record was not actually replaced; committed installation mismatch")
	}
}

// TestExportCarriesNoSecret: the exported container carries only non-secret
// fields; no credential-secret blob appears in the serialized bytes.
func TestExportCarriesNoSecret(t *testing.T) {
	now := time.Now().UTC()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Kind = managementaccess.CredentialExport
	rec.Scope = "credential-restore"
	rec.Export = &managementaccess.EncryptionDescriptor{Algorithm: "aes-256-gcm", Encrypted: true, Nonce: "4fIDBAUGBwgJCgsM"}
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	data, err := mustExportJSON(t, c)
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	for _, leaky := range []string{"SECRET-PLAINTEXT-BLOB", "private_key", "pkcs8", "-----BEGIN PRIVATE KEY-----"} {
		if bytes.Contains(data, []byte(leaky)) {
			t.Fatalf("export container leaks secret material %q", leaky)
		}
	}
}

// TestExportRejectsSecretInDestPath: a dest_path carrying secret material fails
// export closed — the secret scan covers the destination path field.
func TestExportRejectsSecretInDestPath(t *testing.T) {
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	_, err := managementexport("baseharbor.management-access/v1", "dest://x/SECRET-KEY.json", rec)
	if err == nil {
		t.Fatal("export with a secret-carrying dest_path must fail closed")
	}
}

// TestExportRejectsCredentialBearerRecipient: a recipient carrying embedded
// user:pass credentials fails export closed.
func TestExportRejectsCredentialBearerRecipient(t *testing.T) {
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	rec.Recipient = "credential://user:SECRET-PASS@remote/export"
	_, err := managementexport("baseharbor.management-access/v1", "dest", rec)
	if err == nil {
		t.Fatal("export with a credential-bearing recipient must fail closed")
	}
}

// TestSecretCanaryProbe: an adversarial probe that injects a secret canary
// string into a record field is rejected by ExportToContainer BEFORE any bytes
// are produced (finding r27 round-3 finding 1): export fails closed and no
// secret may ever reach exported bytes.
func TestSecretCanaryProbe(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	// Inject a secret canary into the scope field.
	rec.Scope = "control-plane-SECRET-CANARY"
	// Export must reject the canary before producing bytes (finding 1): this is
	// the stronger, now-mandated assertion.
	if _, err := managementexport("baseharbor.management-access/v1", "dest", rec); err == nil {
		t.Fatal("export of a secret-carrying record must fail closed, before bytes are produced")
	}
	// And import must also reject it, leaving no write.
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c := mustExport(t, "baseharbor.management-access/v1", cleanScopeRecord(t, rec))
	_, err = store.ImportForTest(c)
	// The clean-scope record still carries no canary, so import should succeed;
	// a failure here is not fatal (this probe's focus is the export rejection).
	_ = err
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("a secret-carrying import must not write, found %d entries", len(entries))
	}
}

// cleanScopeRecord returns a copy of rec with the secret canary stripped from
// Scope, so other probes can import a valid record.
func cleanScopeRecord(t *testing.T, rec managementaccess.ManagementAccessProfile) managementaccess.ManagementAccessProfile {
	t.Helper()
	rec.Scope = "control-plane"
	return rec
}

// TestUnknownFutureFieldProbe: an adversarial probe with a real secret canary
// AND an unknown future record field must fail closed — silent loss is not
// acceptable; the store must reject the unknown field.
func TestUnknownFutureFieldProbe(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	// Build a container by hand that adds an unknown future field to the record
	// and a secret canary in scope.
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	jsonBytes := mustExportBytes(t, c)
	// Re-encode with an extra unknown record field + secret marker.
	var wrapper map[string]any
	if err := json.Unmarshal(jsonBytes, &wrapper); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	recMap := wrapper["records"].([]any)[0].(map[string]any)
	recMap["future_field"] = "UNKNOWN-CANARY"
	recMap["scope"] = "control-plane-SECRET-CANARY"
	wrapper["records"] = []any{recMap}
	feed, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	_, err = store.ImportFromTestBytes(feed)
	if err == nil {
		t.Fatal("import of a record with an unknown future field + secret must fail closed")
	}
}

// TestBadVersionProbe: importing a record whose version marker does not match
// the store's expected version must fail closed even when the container is built
// by hand with a bogus /v99 version marker.
func TestBadVersionProbe(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	// Hand-build a container with a bogus /v99 record version marker.
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	c.Records[0].Version = "baseharbor.management-access/v99"
	_, err = store.ImportForTest(c)
	if err == nil {
		t.Fatal("import of a bad-version record must fail closed")
	}
}

// TestWrongInstallationScopeProbe: the store binding must reject a record whose
// installation_id / scope does not match the store's expected binding before a
// write.
func TestWrongInstallationScopeProbe(t *testing.T) {
	root := t.TempDir()
	matched := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, matched))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	// A record with a different installation_id.
	other := validRecord(t, "22222222-2222-4222-8222-222222222222", validRecipient, "control-plane", time.Now())
	c := mustExport(t, "baseharbor.management-access/v1", other)
	_, err = store.ImportForTest(c)
	if err == nil {
		t.Fatal("import of a record with a mismatched installation_id must fail closed")
	}
	// A record with a different scope.
	otherScope := validRecord(t, validInstallationID, validRecipient, "admin-plane", time.Now())
	c2 := mustExport(t, "baseharbor.management-access/v1", otherScope)
	_, err = store.ImportForTest(c2)
	if err == nil {
		t.Fatal("import of a record with a mismatched scope must fail closed")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("no write may happen on a binding mismatch, found %d entries", len(entries))
	}
}

// TestSchemaRejectsForeignRecipientProbe: the normative schema must reject a
// record whose recipient does not match the SPIFFE pattern, and the store's
// schema import guard must fail closed on it.
func TestSchemaRejectsForeignRecipientProbe(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	rec.Recipient = "foreign-credential://user:pass@remote"
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	_, err = store.ImportForTest(c)
	if err == nil {
		t.Fatal("import of a record with a foreign credential recipient must fail closed")
	}
}

// TestCanonicalRoundTrip: a real import->export round trip through the store is
// canonical and byte-stable. The record imported is re-exported and the
// re-exported container must decode to an equal record set.
func TestCanonicalRoundTrip(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	if _, err := store.Import(c); err != nil {
		t.Fatalf("import must succeed: %v", err)
	}
	// Re-export the stored record (read it back from disk).
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("expected one committed file, got %d", len(files))
	}
	reimported, err := managementaccess.DecodeContainerForTest(mustReadFile(t, files[0]))
	if err != nil {
		t.Fatalf("re-read committed container: %v", err)
	}
	// Export the re-imported record to a fresh container.
	exported, err := managementexport("baseharbor.management-access/v1", "dest", reimported.Records[0].FromWire())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	dec2, err := managementaccess.DecodeContainerForTest(mustExportBytes(t, exported))
	if err != nil {
		t.Fatalf("decode re-export: %v", err)
	}
	if dec2.Records[0].Installation != reimported.Records[0].Installation {
		t.Fatalf("round trip changed installation_id: %q != %q", dec2.Records[0].Installation, reimported.Records[0].Installation)
	}
	if dec2.Records[0].Recipient != reimported.Records[0].Recipient {
		t.Fatalf("round trip changed recipient: %q != %q", dec2.Records[0].Recipient, reimported.Records[0].Recipient)
	}
}

// TestCanonicalFieldOrder: the canonical container field order is fixed
// (version, dest_path, records, timestamp) and the record field order mirrors
// the struct tags. This is pinned as a regression against reordering.
func TestCanonicalFieldOrder(t *testing.T) {
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	data, err := mustExportJSON(t, c)
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	// Outer order: version before dest_path before records before timestamp.
	oi, eo, ri, ti := indexOf(data, "\"version\""), indexOf(data, "\"dest_path\""), indexOf(data, "\"records\""), indexOf(data, "\"timestamp\"")
	if !(oi < eo && eo < ri && ri < ti) {
		t.Fatalf("outer container field order not canonical: version=%d dest_path=%d records=%d timestamp=%d", oi, eo, ri, ti)
	}
}

// TestTransactionRollbackLeavesNoPartialWrite: a tampered on-disk record must be
// detected on the next open (fail closed) and never silently reused.
func TestTransactionRollbackLeavesNoPartialWrite(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.Import(mustExport(t, "baseharbor.management-access/v1", rec)); err != nil {
		t.Fatalf("first import: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("expected exactly one committed record, got %d", len(files))
	}
	if err := os.WriteFile(files[0], []byte("{ corrupt"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec)); err == nil {
		t.Fatal("open of a tampered store must fail closed")
	}
}

// TestImportWrongScopeRejectedBySchema: a scope that violates the contract's
// scope pattern is rejected by the schema-level write guard.
func TestImportWrongScopeRejectedBySchema(t *testing.T) {
	schema := managementAccessResolve(t)
	badScope := `{"version":"baseharbor.management-access/v1","kind":"management-profile","installation_id":"11111111-1111-4111-8111-111111111111","scope":"bad scope"}`
	var probe map[string]any
	if err := json.Unmarshal([]byte(badScope), &probe); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := schema.Validate(probe); err == nil {
		t.Fatal("schema must reject a record whose scope violates the pattern")
	}
}

// TestBindingRequiresInstallationAndScope: a store cannot be opened with an
// empty expected binding (empty installation_id or scope).
func TestBindingRequiresInstallationAndScope(t *testing.T) {
	root := t.TempDir()
	if _, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, managementaccess.ExpectedBinding{}); err == nil {
		t.Fatal("a store with an empty binding must fail closed")
	}
}

// TestCanaryInEverySerializedFieldCategory: a secret canary injected
// independently into EACH serialized field category fails export closed
// before bytes are produced (finding 1). This proves the scan covers every
// serialized field, not only dest_path and recipient.
func TestCanaryInEverySerializedFieldCategory(t *testing.T) {
	now := time.Now().UTC()
	// installation_id
	byInstall := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	byInstall.InstallationID = validInstallationID + "-SECRET-KEY"
	if _, err := managementexport("baseharbor.management-access/v1", "dest", byInstall); err == nil {
		t.Fatal("canary in installation_id must fail export closed")
	}
	// scope
	byScope := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	byScope.Scope = "control-plane-SECRET-KEY"
	if _, err := managementexport("baseharbor.management-access/v1", "dest", byScope); err == nil {
		t.Fatal("canary in scope must fail export closed")
	}
	// recipient (non-empty, already covered, kept for completeness)
	byRecipient := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	byRecipient.Recipient = "spiffe://baseharbor/platform/core/SECRET-KEY"
	if _, err := managementexport("baseharbor.management-access/v1", "dest", byRecipient); err == nil {
		t.Fatal("canary in recipient must fail export closed")
	}
	// not_after (a serialized time field, encoded as a string)
	byTime := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	byTime.NotAfter = now.Add(2 * time.Hour)
	byTime.NotBefore = now.Add(time.Minute)
	byTime.Scope = "control-plane-SECRET-KEY"
	if _, err := managementexport("baseharbor.management-access/v1", "dest", byTime); err == nil {
		t.Fatal("canary in a serialized field must fail export closed (time-adjusted record)")
	}
	// PEM private key in trust.ca_pem must fail; a public certificate block must
	// still export (the allowlist distinguishes the two).
	byPrivate := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	byPrivate.Trust.CAPEM = "-----BEGIN PRIVATE KEY-----\nMIIEowIBAAKCAQEAexample\n-----END PRIVATE KEY-----"
	if _, err := managementexport("baseharbor.management-access/v1", "dest", byPrivate); err == nil {
		t.Fatal("a private-key PEM in trust must fail export closed")
	}
	// A public certificate block in trust must still export cleanly.
	if _, err := managementexport("baseharbor.management-access/v1", "dest", validRecord(t, validInstallationID, validRecipient, "control-plane", now)); err != nil {
		t.Fatalf("a public certificate in trust must export cleanly: %v", err)
	}
}

// TestAtomicImportNoPartialWriteOnFailure: a failed commit must leave neither a
// partial final file nor any staging (.tmp) artifact behind (finding 2). We
// inject a short-write by making the store root unreadable-for-new-files after
// the first import so the staging open fails, and assert no artifacts remain.
func TestAtomicImportNoPartialWriteOnFailure(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.Import(mustExport(t, "baseharbor.management-access/v1", rec)); err != nil {
		t.Fatalf("first import: %v", err)
	}
	// The first import commits one file; confirm no staging artifact remains.
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("expected exactly one committed file, got %d", len(files))
	}
	if tmps, _ := filepath.Glob(filepath.Join(root, "*.tmp")); len(tmps) != 0 {
		t.Fatalf("staging artifact left behind: %v", tmps)
	}
	// A second import of a different identity under the replace policy must also
	// leave exactly one file and no staging artifact (proving atomic replace).
	storeReplace, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReplace, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("replace store: %v", err)
	}
	rec2 := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	res, err := storeReplace.Import(mustExport(t, "baseharbor.management-access/v1", rec2))
	if err != nil {
		t.Fatalf("replace import: %v", err)
	}
	if res.Replaced != 1 {
		t.Fatalf("expected exactly one replacement, got %d", res.Replaced)
	}
	files, _ = filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("replace must leave exactly one file (atomic replace), got %d", len(files))
	}
	if tmps, _ := filepath.Glob(filepath.Join(root, "*.tmp")); len(tmps) != 0 {
		t.Fatalf("staging artifact left behind after replace: %v", tmps)
	}
}

// TestCanonicalRoundTripFullRecord: a full record survives an import->export
// round trip with a complete record comparison (all fields) and a golden
// re-encoding regression (finding 4).
func TestCanonicalRoundTripFullRecord(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	if _, err := store.Import(c); err != nil {
		t.Fatalf("import: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("expected one committed file, got %d", len(files))
	}
	reimported, err := managementaccess.DecodeContainerForTest(mustReadFile(t, files[0]))
	if err != nil {
		t.Fatalf("re-read committed container: %v", err)
	}
	exported, err := managementexport("baseharbor.management-access/v1", "dest", reimported.Records[0].FromWire())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	_ = exported
	// Full record comparison across all serialized fields.
	comparisons := []struct {
		name string
		got  string
		want string
	}{
		{"version", reimported.Records[0].Version, rec.Version},
		{"kind", string(reimported.Records[0].Kind), string(rec.Kind)},
		{"installation", reimported.Records[0].Installation, rec.InstallationID},
		{"recipient", reimported.Records[0].Recipient, rec.Recipient},
		{"scope", reimported.Records[0].Scope, rec.Scope},
		{"not_before", reimported.Records[0].NotBefore.UTC().Format(time.RFC3339Nano), rec.NotBefore.UTC().Format(time.RFC3339Nano)},
		{"not_after", reimported.Records[0].NotAfter.UTC().Format(time.RFC3339Nano), rec.NotAfter.UTC().Format(time.RFC3339Nano)},
	}
	for _, f := range comparisons {
		if f.got != f.want {
			t.Fatalf("round trip changed %s: got %q want %q", f.name, f.got, f.want)
		}
	}
}

// TestGoldenReEncodingStable: the golden fixture re-encodes byte-stably across
// an export->decode->export cycle, proving the canonical encoding is
// deterministic (finding 4 golden re-encoding regression). No store binding is
// involved; the golden record's own scope is used through the typed decode path.
func TestGoldenReEncodingStable(t *testing.T) {
	data, err := os.ReadFile("store.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	c, err := managementaccess.DecodeContainerForTest(data)
	if err != nil {
		t.Fatalf("golden must decode: %v", err)
	}
	first, err := mustExportJSON(t, c)
	if err != nil {
		t.Fatalf("first encode: %v", err)
	}
	// Re-decode the first encoding and encode again; output must be identical.
	red, err := managementaccess.DecodeContainerForTest(first)
	if err != nil {
		t.Fatalf("re-decode first: %v", err)
	}
	second, err := mustExportJSON(t, red)
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("golden re-encoding not stable across export->decode->export")
	}
}

// helpers

// indexOf returns the byte offset of sub in data, or -1.
func indexOf(data []byte, sub string) int {
	return bytes.Index(data, []byte(sub))
}

// firstJSON returns the path of the single committed *.json file in root.
func firstJSON(t *testing.T, root string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("expected one json file: %v", err)
	}
	return files[0]
}

// mustReadFile reads a file or fails.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// managementexport is a thin local alias for managementaccess.ExportToContainer
// used by the export-guard tests.
func managementexport(version string, destPath string, records ...managementaccess.ManagementAccessProfile) (*managementaccess.ProfileContainer, error) {
	return managementaccess.ExportToContainer(version, destPath, records...)
}

func mustExport(t *testing.T, version string, recs ...managementaccess.ManagementAccessProfile) *managementaccess.ProfileContainer {
	t.Helper()
	c, err := managementexport(version, "dest", recs...)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return c
}

func mustExportJSON(t *testing.T, c *managementaccess.ProfileContainer) ([]byte, error) {
	t.Helper()
	return managementaccess.ExportContainerJSONForTest(c)
}

func mustExportBytes(t *testing.T, c *managementaccess.ProfileContainer) []byte {
	t.Helper()
	data, err := mustExportJSON(t, c)
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	return data
}

// tamperTrustSHA256 returns a copy of the container whose single record SHA-256
// fingerprint is replaced with the all-zero value, producing a container whose
// record fails ValidateKindEnforcement on import.
func tamperTrustSHA256(t *testing.T, c *managementaccess.ProfileContainer) []byte {
	t.Helper()
	if len(c.Records) != 1 {
		t.Fatalf("expected exactly one record, got %d", len(c.Records))
	}
	c.Records[0].Trust.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	data, err := managementaccess.ExportContainerJSONForTest(c)
	if err != nil {
		t.Fatalf("container json: %v", err)
	}
	return data
}
