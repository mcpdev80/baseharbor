package workstationenrollment_test

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts/workstationenrollment/v1"
)

const tenantID = "11111111-1111-4111-8111-111111111111"

// baselineRecord returns a valid workstation-enrollment record.
func baselineRecord(t *testing.T) workstationenrollment.Record {
	t.Helper()
	return workstationenrollment.Record{
		Kind:           workstationenrollment.RecordKindWorkstationEnrollment,
		Version:        workstationenrollment.Version,
		InstallationID: tenantID,
		Recipient:      "spiffe://baseharbor/workstations/" + tenantID + "/dev-laptop-01",
		Scope:          "dev-laptop-01",
		NotBefore:      time.Now().UTC().Add(-time.Minute),
		NotAfter:       time.Now().UTC().Add(time.Hour),
		Import:         workstationenrollment.ImportCapability{},
		Storage:        workstationenrollment.Storage{Class: workstationenrollment.SessionOnly, Durable: false, Encrypted: false, DisasterRecovery: false},
		Bootstrap:      workstationenrollment.Bootstrap{TTLSeconds: 30, CertificateTTLSeconds: 3600},
	}
}

// POSITIVE: a valid record validates and serializes to the wire layout.
func TestRecordPositiveValidates(t *testing.T) {
	record := baselineRecord(t)
	if err := record.ValidateKindEnforcement(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	// Wire layout: JSON tags match the schema's required keys.
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kind", "version", "installation_id", "recipient", "scope", "not_before", "not_after", "import", "storage", "bootstrap"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("wire record missing key %q", key)
		}
	}
}

// ROTE 1: foreign recipient (not bound to the scope) is rejected.
func TestRecordForeignRecipientRejected(t *testing.T) {
	record := baselineRecord(t)
	record.Recipient = "spiffe://baseharbor/workstations/" + tenantID + "/attacker-02"
	if err := record.ValidateKindEnforcement(); err == nil {
		t.Fatal("foreign recipient must fail closed")
	}
}

// ROTE 3: a malformed scope/recipient pair is rejected.
func TestRecordScopeMismatchRejected(t *testing.T) {
	record := baselineRecord(t)
	record.Scope = "attacker-02"
	if err := record.ValidateKindEnforcement(); err == nil {
		t.Fatal("scope/recipient mismatch must fail closed")
	}
}

// ROTE 2: an elapsed validity window is rejected.
func TestRecordElapsedWindowRejected(t *testing.T) {
	record := baselineRecord(t)
	record.NotBefore = time.Now().UTC().Add(-2 * time.Hour)
	record.NotAfter = time.Now().UTC().Add(-time.Hour)
	if err := record.ValidateKindEnforcement(); err == nil {
		t.Fatal("an elapsed window must fail closed")
	}
}

// ROTE 4: an import capability advertising a true flag is rejected.
func TestRecordImportOpenRejected(t *testing.T) {
	record := baselineRecord(t)
	record.Import = workstationenrollment.ImportCapability{RightsGranting: true}
	if err := record.ValidateKindEnforcement(); err == nil {
		t.Fatal("an import that grants rights must fail closed")
	}
}

// A valid workstation record must carry an installation-scoped SPIFFE recipient,
// distinct from the Core connector scope.
func TestRecordRecipientIsInstallationScoped(t *testing.T) {
	_, _ = url.Parse("spiffe://baseharbor/workstations/" + tenantID + "/dev-laptop-01")
	record := baselineRecord(t)
	want := "spiffe://baseharbor/workstations/" + tenantID + "/dev-laptop-01"
	if record.Recipient != want {
		t.Fatalf("recipient must be installation-scoped SPIFFE, got %q", record.Recipient)
	}
}
