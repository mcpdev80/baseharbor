package managementaccess_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
)

// This file carries the round-5 counters for the seven Codex review-1363c
// recipes. Each recipe is proven with its own SHA-exact test that fails closed
// on the reviewed defect; they are additive to the round-4 suite and never
// delete or weaken an existing test.

// TestRecipe1CallerVersionEnforced proves recipe 1: the canonical record version
// is enforced independently of the caller-supplied version argument on BOTH
// export and import. A caller that passes a different value as the expected
// version cannot widen the acceptance window (the /v99 bypass).
func TestRecipe1CallerVersionEnforced(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())

	// Export with a caller-selected noncanonical version must fail closed.
	if _, err := managementexport("baseharbor.management-access/v99", "dest", rec); err == nil {
		t.Fatal("recipe 1: export with a caller-selected noncanonical version must fail closed")
	}
	// Export with the canonical marker still succeeds.
	if _, err := managementexport(managementaccess.CanonicalRecordVersion, "dest", rec); err != nil {
		t.Fatalf("recipe 1: export with the canonical version must succeed: %v", err)
	}

	// Import into a store whose version argument is noncanonical must still
	// reject a noncanonical record and accept a canonical one.
	store, err := managementaccess.NewStoreForTest(root, "baseharbor.management-access/v99", managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("recipe 1: new store with noncanonical version arg must still open: %v", err)
	}

	// A hand-built container with a /v99 record version marker must fail.
	cBad := mustExport(t, managementaccess.CanonicalRecordVersion, rec)
	cBad.Records[0].Version = "baseharbor.management-access/v99"
	if _, err := store.ImportForTest(cBad); err == nil {
		t.Fatal("recipe 1: import of a noncanonical record version must fail closed regardless of store version arg")
	}
	// A canonical record still imports cleanly.
	if _, err := store.Import(mustExport(t, managementaccess.CanonicalRecordVersion, rec)); err != nil {
		t.Fatalf("recipe 1: import of a canonical record must succeed: %v", err)
	}
}

// TestRecipe2ForeignValidRecipientRejected proves recipe 2: a different,
// otherwise schema-valid SPIFFE recipient is rejected before any write, and a
// store with an empty recipient prefix fails closed at open time.
func TestRecipe2ForeignValidRecipientRejected(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("recipe 2: new store: %v", err)
	}
	// A different valid SPIFFE recipient under a DIFFERENT prefix (different
	// platform/audience). The binding prefix is platform/core; a valid recipient
	// under platform/connectors is schema-valid but must be rejected because it
	// does not bind to the expected recipient prefix.
	foreign := validRecord(t, validInstallationID, "spiffe://baseharbor/platform/connectors/22222222-2222-4222-8222-222222222222/other", "control-plane", time.Now())
	cForeign := mustExport(t, managementaccess.CanonicalRecordVersion, foreign)
	if _, err := store.ImportForTest(cForeign); err == nil {
		t.Fatal("recipe 2: import of a different valid SPIFFE recipient (different prefix) must fail closed before any write")
	}
	// No write may have happened.
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("recipe 2: a foreign-recipient import must not write, found %d entries", len(entries))
	}

	// A store opened with an empty recipient prefix must fail closed.
	emptyPrefix := rec
	_, err = managementaccess.NewStoreForTest(t.TempDir(), managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReject, managementaccess.ExpectedBinding{
		InstallationID:  emptyPrefix.InstallationID,
		RecipientPrefix: "",
		Scope:           emptyPrefix.Scope,
	})
	if err == nil {
		t.Fatal("recipe 2: a store with an empty recipient prefix must fail closed at open time")
	}
}

// TestRecipe3TrustSelfConsistencyUnconditional proves recipe 3: trust
// self-consistency is enforced unconditionally on import (no switch can disable
// it). A record whose CA does not hash to its own fingerprint fails closed even
// when the test seam explicitly sets TrustSelfConsistent=false.
func TestRecipe3TrustSelfConsistencyUnconditional(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	// A false switch must not disable the mandatory self-consistency check.
	binding := managementaccess.ExpectedBindingForTest(rec, false)
	store, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReject, binding)
	if err != nil {
		t.Fatalf("recipe 3: new store: %v", err)
	}
	// Tamper the trust fingerprint so the record fails ValidateTrust on import
	// re-validation, and import it directly (bypassing the export guard).
	c := mustExport(t, managementaccess.CanonicalRecordVersion, rec)
	tamperTrustSHA256(t, c)
	if _, err := store.ImportForTest(c); err == nil {
		t.Fatal("recipe 3: import of a record with a broken trust fingerprint must fail closed with TrustSelfConsistent=false")
	}
}

// TestRecipe4ShortWriteSeam proves recipe 4: a deterministic short write after a
// prefix has been staged leaves no partial final file and no temp artifact, and
// fails closed with a typed partial-write error.
func TestRecipe4ShortWriteSeam(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("recipe 4: new store: %v", err)
	}
	// Inject one genuine prefix write through the same one-call seam used by the
	// atomic commit path and capture both the requested and actual byte counts.
	var calls, requested, written int
	store.SetStagingWriteForTest(func(dst io.Writer, data []byte) (int, error) {
		calls++
		requested = len(data)
		if requested < 2 {
			return 0, errors.New("test fixture too short for partial write")
		}
		var writeErr error
		written, writeErr = dst.Write(data[:requested/2])
		return written, writeErr
	})
	_, err = store.Import(mustExport(t, managementaccess.CanonicalRecordVersion, rec))
	if written <= 0 || written >= requested {
		t.Fatalf("recipe 4: writer did not produce a nonzero short write: n=%d requested=%d", written, requested)
	}
	if calls != 1 {
		t.Fatalf("recipe 4: atomic commit must make one write attempt, got %d", calls)
	}
	if !errors.Is(err, managementaccess.ErrPartialWrite) {
		t.Fatalf("recipe 4: short write error must be typed ErrPartialWrite, got %v", err)
	}
	// No partial final file may remain.
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 0 {
		t.Fatalf("recipe 4: a short write must leave no partial final file, found %d files", len(files))
	}
	// No temp artifact may remain.
	tmps, _ := filepath.Glob(filepath.Join(root, "*.tmp"))
	if len(tmps) != 0 {
		t.Fatalf("recipe 4: a short write must leave no temp artifact, found %v", tmps)
	}
}

func TestRecipe4ReplaceCleanupFailurePreservesOldRecord(t *testing.T) {
	root := t.TempDir()
	oldRecord := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReplace, storeBindingFor(t, oldRecord))
	if err != nil {
		t.Fatalf("recipe 4: new store: %v", err)
	}
	if _, err := store.Import(mustExport(t, managementaccess.CanonicalRecordVersion, oldRecord)); err != nil {
		t.Fatalf("recipe 4: first import: %v", err)
	}
	oldPath := firstJSON(t, root)
	oldBytes := mustReadFile(t, oldPath)

	newRecord := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	removeFailure := errors.New("injected obsolete-record removal failure")
	var attemptedPath string
	store.SetRemoveObsoleteForTest(func(path string) error {
		attemptedPath = path
		return removeFailure
	})
	_, err = store.Import(mustExport(t, managementaccess.CanonicalRecordVersion, newRecord))
	if !errors.Is(err, managementaccess.ErrNoWrite) {
		t.Fatalf("recipe 4: replacement cleanup failure must return typed ErrNoWrite, got %v", err)
	}
	if attemptedPath != oldPath {
		t.Fatalf("recipe 4: cleanup seam called for %q, want obsolete file %q", attemptedPath, oldPath)
	}
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil || len(files) != 1 || files[0] != oldPath {
		t.Fatalf("recipe 4: failed replacement must leave only the old record, files=%v err=%v", files, err)
	}
	if got := mustReadFile(t, oldPath); !bytes.Equal(got, oldBytes) {
		t.Fatal("recipe 4: failed replacement changed the old record bytes")
	}
	if tmps, _ := filepath.Glob(filepath.Join(root, "*.tmp")); len(tmps) != 0 {
		t.Fatalf("recipe 4: failed replacement left staging artifacts: %v", tmps)
	}
	if _, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReject, storeBindingFor(t, oldRecord)); err != nil {
		t.Fatalf("recipe 4: old on-disk state must reopen cleanly after failed replacement: %v", err)
	}
}

// TestRecipe4ReplaceContentProven proves recipe 4/6: under the replace policy
// the new content must actually supersede the old content on disk (the old file
// is gone and its payload is no longer present), a removal failure fails closed
// rather than reporting success with a stale record left behind.
func TestRecipe4ReplaceContentProven(t *testing.T) {
	root := t.TempDir()
	rec1 := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	store, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReplace, storeBindingFor(t, rec1))
	if err != nil {
		t.Fatalf("recipe 4: new store: %v", err)
	}
	c1 := mustExport(t, managementaccess.CanonicalRecordVersion, rec1)
	if _, err := store.Import(c1); err != nil {
		t.Fatalf("recipe 4: first import must succeed: %v", err)
	}
	oldBytes := mustReadFile(t, firstJSON(t, root))

	// A genuinely different payload (a different CA certificate -> different
	// trust fingerprint) with the SAME binding fields so it passes binding but
	// is provably distinct from the old content.
	rec2 := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	c2 := mustExport(t, managementaccess.CanonicalRecordVersion, rec2)
	res, err := store.Import(c2)
	if err != nil {
		t.Fatalf("recipe 4: replace import must succeed: %v", err)
	}
	if res.Replaced != 1 {
		t.Fatalf("recipe 4: replace import must report exactly one replacement, got %d", res.Replaced)
	}
	newBytes := mustReadFile(t, firstJSON(t, root))
	// Exactly one file remains (old superseded, not appended).
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("recipe 4/6: replace must leave exactly one file, got %d", len(files))
	}
	// The new content must differ from the old content.
	if string(oldBytes) == string(newBytes) {
		t.Fatal("recipe 6: replacement content must differ from the old content")
	}
	// The old content's CA fingerprint must no longer be present in the
	// committed record (superseded), and the new content must carry its own
	// (distinct) fingerprint.
	oldFingerprint := rec1.Trust.SHA256
	newFingerprint := rec2.Trust.SHA256
	if strings.Contains(string(oldBytes), oldFingerprint) && strings.Contains(string(newBytes), oldFingerprint) {
		t.Fatal("recipe 6: the old CA fingerprint must not survive in the committed record after replacement")
	}
	if !strings.Contains(string(newBytes), newFingerprint) {
		t.Fatal("recipe 6: the committed record must carry the new content's fingerprint")
	}
}

// TestRecipe5CanaryPerCategory proves recipe 5: a secret canary injected
// independently into EACH meaningful serialized field category fails export
// closed, so no secret can reach the exported bytes in any category, and a
// clean record in every category still exports cleanly.
func TestRecipe5CanaryPerCategory(t *testing.T) {
	now := time.Now().UTC()
	// installation_id
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.InstallationID += "-SECRET-KEY"
	assertCanaryRejected(t, "dest", rec, "SECRET-KEY", "installation_id")
	// scope
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Scope = "control-plane-SECRET-KEY"
	assertCanaryRejected(t, "dest", rec, "SECRET-KEY", "scope")
	// recipient
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Recipient = "spiffe://baseharbor/platform/core/SECRET-KEY"
	assertCanaryRejected(t, "dest", rec, "SECRET-KEY", "recipient")
	// destination path is a serialized container string outside the record.
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	assertCanaryRejected(t, "/tmp/DEST-SECRET-KEY", rec, "SECRET-KEY", "dest_path")
	// Storage.class is the only serialized storage string and its schema domain
	// is a closed enum, so a schema-valid secret canary cannot be constructed.
	// Still exercise the scanner on this malformed value; it must never return
	// an export container or bytes.
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Storage.Class = managementaccess.StorageClass("SECRET-KEY")
	assertCanaryRejected(t, "dest", rec, "SECRET-KEY", "storage.class")
	// A 16-character base64url nonce is schema-valid; embed the canary in the
	// credential-export descriptor's serialized nonce.
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Kind = managementaccess.CredentialExport
	rec.Export = &managementaccess.EncryptionDescriptor{Algorithm: managementaccess.AlgorithmAES256GCM, Encrypted: true, Nonce: "SECRETAAAAAAAAAA"}
	assertCanaryRejected(t, "dest", rec, "SECRET", "export.nonce")
	// nested trust ca_pem as a private-key PEM block
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Trust.CAPEM = "-----BEGIN PRIVATE KEY-----\nMIIEowIBAAKCAQEAexample\n-----END PRIVATE KEY-----"
	assertExportRejected(t, "dest", rec, "trust.ca_pem")
	// A clean record in every category must still export cleanly.
	clean := validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	if _, err := managementexport(managementaccess.CanonicalRecordVersion, "dest", clean); err != nil {
		t.Fatalf("recipe 5: a clean record must export cleanly: %v", err)
	}
}

// TestRecipe6FullRecordRoundTrip proves recipe 6: a full record survives an
// import -> export round trip with a COMPLETE field comparison (including trust,
// import, storage, export — the fields the round-4 test omitted) and byte-stable
// re-encoding.
func TestRecipe6FullRecordRoundTrip(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	rec.Kind = managementaccess.CredentialExport
	rec.Export = &managementaccess.EncryptionDescriptor{Algorithm: managementaccess.AlgorithmAES256GCM, Encrypted: true, Nonce: "4fIDBAUGBwgJCgsM"}
	store, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReject, storeBindingFor(t, rec))
	if err != nil {
		t.Fatalf("recipe 6: new store: %v", err)
	}
	c := mustExport(t, managementaccess.CanonicalRecordVersion, rec)
	if _, err := store.Import(c); err != nil {
		t.Fatalf("recipe 6: import: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	if len(files) != 1 {
		t.Fatalf("recipe 6: expected one committed file, got %d", len(files))
	}
	committedBytes := mustReadFile(t, files[0])
	if !json.Valid(committedBytes) {
		t.Fatalf("recipe 6: committed artifact must be exactly one valid JSON document, bytes=%d", len(committedBytes))
	}
	decoder := json.NewDecoder(bytes.NewReader(committedBytes))
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("recipe 6: decode committed document: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("recipe 6: committed artifact has trailing JSON data: err=%v", err)
	}
	reimported, err := managementaccess.DecodeContainerForTest(committedBytes)
	if err != nil {
		t.Fatalf("recipe 6: re-read committed container: %v", err)
	}
	exported, err := managementexport(managementaccess.CanonicalRecordVersion, "dest", reimported.Records[0].FromWire())
	if err != nil {
		t.Fatalf("recipe 6: re-export: %v", err)
	}
	dec2, err := managementaccess.DecodeContainerForTest(mustExportBytes(t, exported))
	if err != nil {
		t.Fatalf("recipe 6: decode re-export: %v", err)
	}
	if dec2.Records[0].Export == nil || dec2.Records[0].Export.Algorithm != managementaccess.AlgorithmAES256GCM || !dec2.Records[0].Export.Encrypted || dec2.Records[0].Export.Nonce != rec.Export.Nonce {
		t.Fatalf("recipe 6: non-empty export descriptor changed across round trip: got=%+v want=%+v", dec2.Records[0].Export, rec.Export)
	}
	wantRecord, err := json.Marshal(rec.ToWire())
	if err != nil {
		t.Fatalf("recipe 6: marshal original record: %v", err)
	}
	gotRecord, err := json.Marshal(dec2.Records[0])
	if err != nil {
		t.Fatalf("recipe 6: marshal round-tripped record: %v", err)
	}
	if !bytes.Equal(gotRecord, wantRecord) {
		t.Fatalf("recipe 6: full serialized record changed across round trip\n got: %s\nwant: %s", gotRecord, wantRecord)
	}
}

func assertCanaryRejected(t *testing.T, destPath string, rec managementaccess.ManagementAccessProfile, canary, field string) {
	t.Helper()
	container, err := managementexport(managementaccess.CanonicalRecordVersion, destPath, rec)
	if !errors.Is(err, managementaccess.ErrSecretOnField) {
		t.Fatalf("recipe 5: canary in %s must be rejected by the secret scan, got %v", field, err)
	}
	if container != nil {
		data, encodeErr := managementaccess.ExportContainerJSONForTest(container)
		if encodeErr != nil {
			t.Fatalf("recipe 5: encode rejected %s container: %v", field, encodeErr)
		}
		if bytes.Contains(data, []byte(canary)) {
			t.Fatalf("recipe 5: canary escaped into %s export bytes", field)
		}
		t.Fatalf("recipe 5: rejected %s export returned a container", field)
	}
}

func assertExportRejected(t *testing.T, destPath string, rec managementaccess.ManagementAccessProfile, field string) {
	t.Helper()
	container, err := managementexport(managementaccess.CanonicalRecordVersion, destPath, rec)
	if err == nil || container != nil {
		t.Fatalf("recipe 5: invalid/canary %s must be rejected before returning bytes; container=%v err=%v", field, container != nil, err)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
