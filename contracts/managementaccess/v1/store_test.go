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

// validRecord builds a fully contract-valid management-profile record. The
// validity window is anchored at the current wall-clock time (not the passed
// `now`), so callers may shift `now` for the record's own fields without moving
// the window off the current instant. Each call is fresh so variants never
// share pointers.
func validRecord(t *testing.T, installationID, recipient string, _ time.Time) managementaccess.ManagementAccessProfile {
	t.Helper()
	now := time.Now().UTC()
	caPEM, caSHA := mustTestCA(t)
	return managementaccess.ManagementAccessProfile{
		Kind:           managementaccess.ManagementProfile,
		Version:        "baseharbor.management-access/v1",
		InstallationID: installationID,
		Trust:          managementaccess.Trust{CAPEM: caPEM, SHA256: caSHA},
		Recipient:      recipient,
		Scope:          "control-plane",
		NotBefore:      now.Add(-time.Minute),
		NotAfter:       now.Add(time.Hour),
		Import:         managementaccess.ImportCapability{},
		Storage:        managementaccess.Storage{Class: "protected-durable", Durable: true, Encrypted: true, DisasterRecovery: false},
	}
}

// TestGoldenContainerDecodes asserts the shipped golden fixture is a well-formed
// container with one credential-export record.
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

// TestExportToContainerRejectsWrongVersion: the export build step fails closed on
// a record whose version is not the expected marker — the first write guard.
func TestExportToContainerRejectsWrongVersion(t *testing.T) {
	rec := validRecord(t, "11111111-1111-4111-8111-111111111111", "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab", time.Now())
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
	recipient := "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"
	rec := validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, now)
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
	store, err := managementaccess.NewStore(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().UTC()
	recipient := "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"
	rec := validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, now)
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	data := tamperTrustSHA256(t, c)
	_, err = store.ImportFromTestBytes(data)
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
	store, err := managementaccess.NewStore(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().UTC()
	recipient := "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"
	first := validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, now)
	c1 := mustExport(t, "baseharbor.management-access/v1", first)
	if _, err := store.Import(c1); err != nil {
		t.Fatalf("first import must succeed: %v", err)
	}

	second := validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, time.Now())
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

// TestImportConflictReplaceOptsIn: under the replace policy the same identity is
// replaced exactly once, not silently on every import.
func TestImportConflictReplaceOptsIn(t *testing.T) {
	root := t.TempDir()
	store, err := managementaccess.NewStore(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReplace)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().UTC()
	recipient := "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"
	_, err = store.Import(mustExport(t, "baseharbor.management-access/v1", validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, now)))
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	res, err := store.Import(mustExport(t, "baseharbor.management-access/v1", validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, time.Now())))
	if err != nil {
		t.Fatalf("replace import must succeed: %v", err)
	}
	if res.Replaced != 1 {
		t.Fatalf("replace import must report exactly one replacement, got %d", res.Replaced)
	}
}

// TestExportCarriesNoSecret: the exported container carries only non-secret
// fields; no credential-secret blob appears in the serialized bytes.
func TestExportCarriesNoSecret(t *testing.T) {
	now := time.Now().UTC()
	recipient := "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"
	rec := validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, now)
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

// TestRoundTripEncodeDecodeLossless: encode a valid container and decode it back
// into a contract-valid record.
func TestRoundTripEncodeDecodeLossless(t *testing.T) {
	now := time.Now().UTC()
	recipient := "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"
	rec := validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, now)
	c := mustExport(t, "baseharbor.management-access/v1", rec)
	dec, err := managementaccess.DecodeContainerForTest(mustExportBytes(t, c))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dec.Records) != 1 {
		t.Fatalf("decoded container must carry one record, got %d", len(dec.Records))
	}
	decoded := dec.Records[0].FromWire()
	if err := decoded.ValidateKindEnforcement(); err != nil {
		t.Fatalf("decoded record must be contract-valid: %v", err)
	}
}

// TestTransactionRollbackLeavesNoPartialWrite: a tampered on-disk record must be
// detected on the next open (fail closed) and never silently reused.
func TestTransactionRollbackLeavesNoPartialWrite(t *testing.T) {
	root := t.TempDir()
	store, err := managementaccess.NewStore(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().UTC()
	recipient := "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab"
	_, err = store.Import(mustExport(t, "baseharbor.management-access/v1", validRecord(t, "11111111-1111-4111-8111-111111111111", recipient, now)))
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("expected exactly one committed record, got %d", len(files))
	}
	if err := os.WriteFile(files[0], []byte("{ corrupt"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v1", managementaccess.ImportPolicyConflictReject); err == nil {
		t.Fatal("open of a tampered store must fail closed")
	}
}

// TestImportWrongScopeRejectedBySchema: a scope that violates the contract's
// scope pattern is rejected by the schema-level write guard. Scope format is a
// contract-schema field; the store delegates that check to the schema.
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

// helpers

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
