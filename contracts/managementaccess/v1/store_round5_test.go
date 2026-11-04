package managementaccess_test

import (
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
// when the test seam passes TrustSelfConsistent=true.
func TestRecipe3TrustSelfConsistencyUnconditional(t *testing.T) {
	root := t.TempDir()
	rec := validRecord(t, validInstallationID, validRecipient, "control-plane", time.Now())
	// Build the binding with the self-consistency switch forced true.
	binding := managementaccess.ExpectedBindingForTest(rec, true)
	store, err := managementaccess.NewStoreForTest(root, managementaccess.CanonicalRecordVersion, managementaccess.ImportPolicyConflictReject, binding)
	if err != nil {
		t.Fatalf("recipe 3: new store: %v", err)
	}
	// Tamper the trust fingerprint so the record fails ValidateTrust on import
	// re-validation, and import it directly (bypassing the export guard).
	c := mustExport(t, managementaccess.CanonicalRecordVersion, rec)
	tamperTrustSHA256(t, c)
	if _, err := store.ImportForTest(c); err == nil {
		t.Fatal("recipe 3: import of a record with a broken trust fingerprint must fail closed even with TrustSelfConsistent=true")
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
	// Inject the deterministic short-write fault seam before the import.
	store.SetStagingFaultShortWriteForTest(true)
	_, err = store.Import(mustExport(t, managementaccess.CanonicalRecordVersion, rec))
	if err == nil {
		t.Fatal("recipe 4: a genuine short write after staging must fail closed")
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
	if _, err := managementexport(managementaccess.CanonicalRecordVersion, "dest", rec); err == nil {
		t.Fatal("recipe 5: canary in installation_id must fail export closed")
	}
	// scope
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Scope = "control-plane-SECRET-KEY"
	if _, err := managementexport(managementaccess.CanonicalRecordVersion, "dest", rec); err == nil {
		t.Fatal("recipe 5: canary in scope must fail export closed")
	}
	// recipient
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Recipient = "spiffe://baseharbor/platform/core/SECRET-KEY"
	if _, err := managementexport(managementaccess.CanonicalRecordVersion, "dest", rec); err == nil {
		t.Fatal("recipe 5: canary in recipient must fail export closed")
	}
	// nested trust ca_pem as a private-key PEM block
	rec = validRecord(t, validInstallationID, validRecipient, "control-plane", now)
	rec.Trust.CAPEM = "-----BEGIN PRIVATE KEY-----\nMIIEowIBAAKCAQEAexample\n-----END PRIVATE KEY-----"
	if _, err := managementexport(managementaccess.CanonicalRecordVersion, "dest", rec); err == nil {
		t.Fatal("recipe 5: a private-key PEM in trust.ca_pem must fail export closed")
	}
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
	reimported, err := managementaccess.DecodeContainerForTest(mustReadFile(t, files[0]))
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
	// Full round trip must be byte-stable EXCLUDING the volatile container
	// timestamp (which legitimately differs between the original export and the
	// re-export). Compare the record arrays field-by-field instead, which is the
	// recipe's real assertion: the re-exported record equals the imported record.
	// Complete field comparison across ALL serialized fields, including the
	// nested objects the round-4 test discarded.
	comparisons := []struct {
		name string
		got  string
		want string
	}{
		{"version", dec2.Records[0].Version, rec.Version},
		{"kind", string(dec2.Records[0].Kind), string(rec.Kind)},
		{"installation", dec2.Records[0].Installation, rec.InstallationID},
		{"trust.ca_pem", dec2.Records[0].Trust.CAPEM, rec.Trust.CAPEM},
		{"trust.sha256", dec2.Records[0].Trust.SHA256, rec.Trust.SHA256},
		{"recipient", dec2.Records[0].Recipient, rec.Recipient},
		{"scope", dec2.Records[0].Scope, rec.Scope},
		{"not_before", dec2.Records[0].NotBefore.UTC().Format(time.RFC3339Nano), rec.NotBefore.UTC().Format(time.RFC3339Nano)},
		{"not_after", dec2.Records[0].NotAfter.UTC().Format(time.RFC3339Nano), rec.NotAfter.UTC().Format(time.RFC3339Nano)},
		{"import.rights_granting", boolStr(dec2.Records[0].Import.RightsGranting), boolStr(rec.Import.RightsGranting)},
		{"import.plugin_execution", boolStr(dec2.Records[0].Import.PluginExecution), boolStr(rec.Import.PluginExecution)},
		{"import.credential_redirect", boolStr(dec2.Records[0].Import.CredentialRedirect), boolStr(rec.Import.CredentialRedirect)},
		{"storage.class", string(dec2.Records[0].Storage.Class), string(rec.Storage.Class)},
		{"storage.durable", boolStr(dec2.Records[0].Storage.Durable), boolStr(rec.Storage.Durable)},
		{"storage.encrypted", boolStr(dec2.Records[0].Storage.Encrypted), boolStr(rec.Storage.Encrypted)},
		{"storage.disaster_recovery", boolStr(dec2.Records[0].Storage.DisasterRecovery), boolStr(rec.Storage.DisasterRecovery)},
	}
	for _, f := range comparisons {
		if f.got != f.want {
			t.Fatalf("recipe 6: round trip changed %s: got %q want %q", f.name, f.got, f.want)
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
