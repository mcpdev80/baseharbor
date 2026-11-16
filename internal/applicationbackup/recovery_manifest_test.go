package applicationbackup

import "testing"

func TestRecoveryManifestRoundTrip(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true},
		{StateClass: StateSQL, LogicalResource: "primary", Ownership: "application", Support: RecoverySupported, DefaultSelected: true},
		{StateClass: StateLogs, LogicalResource: "api", Ownership: "application", Support: RecoveryUnsupported, Reason: "not supported"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := RecoveryManifestPayloadEntry(selection)
	if err != nil {
		t.Fatal(err)
	}
	payload := Payload{Entries: []PayloadEntry{entry}}
	got, found, err := RecoveryManifestFromPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("recovery manifest not found")
	}
	if got.Version != RecoveryManifestVersion || len(got.Contributors) != 3 {
		t.Fatalf("unexpected recovery manifest: %#v", got)
	}
}

func TestRecoveryManifestRejectsSelectedUnsupportedContributor(t *testing.T) {
	manifest := RecoveryManifest{
		Version: RecoveryManifestVersion,
		Contributors: []RecoveryContributor{{
			StateClass: StateObjectStorage, LogicalResource: "uploads", Ownership: "application",
			Support: RecoveryUnsupported, Selected: true,
		}},
	}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected selected unsupported contributor rejection")
	}
}
