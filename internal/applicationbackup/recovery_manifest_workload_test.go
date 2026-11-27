package applicationbackup

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestWorkloadVolumesAssertedInRecoveryManifest guards H-V39/F4: every owned
// workload volume declared by an application must appear as a workload.storage
// contributor in the recovery manifest, and the manifest must fail-closed when
// a declared owned volume is omitted from it.
func TestWorkloadVolumesAssertedInRecoveryManifest(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
		{StateClass: StateWorkloadStorage, LogicalResource: "postgres-data", Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
		{StateClass: StateWorkloadStorage, LogicalResource: "cache", Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := RecoveryManifestPayloadEntry(selection)
	if err != nil {
		t.Fatal(err)
	}
	payload := Payload{Entries: []PayloadEntry{entry}}
	manifest, found, err := RecoveryManifestFromPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("recovery manifest not found")
	}

	got := map[string]string{}
	for _, c := range manifest.Contributors {
		if c.StateClass != StateWorkloadStorage {
			continue
		}
		got[c.LogicalResource] = c.Ownership
	}
	if _, ok := got["postgres-data"]; !ok {
		t.Fatalf("owned workload volume postgres-data missing from recovery manifest: %#v", manifest.Contributors)
	}
	if _, ok := got["cache"]; !ok {
		t.Fatalf("owned workload volume cache missing from recovery manifest: %#v", manifest.Contributors)
	}
	if got["postgres-data"] != "application" || got["cache"] != "application" {
		t.Fatalf("workload storage ownership = %#v, want application", got)
	}
}

// TestRecoveryManifestFailsClosedOnMissingOwnedWorkloadVolume guards H-V39/F4:
// a recovery selection that declares an owned, durable workload volume but omits
// it from the manifest must be rejected on capture (fail-closed).
func TestRecoveryManifestFailsClosedOnMissingOwnedWorkloadVolume(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
		{StateClass: StateWorkloadStorage, LogicalResource: "postgres-data", Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The manifest must contain both the metadata and the owned workload volume.
	manifest := RecoveryManifest{
		Version: RecoveryManifestVersion,
		Contributors: []RecoveryContributor{
			{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("manifest validate failed: %v", err)
	}

	// A durable owned workload volume required by the selection but absent from
	// the manifest is a recovery integrity violation.
	for _, c := range selection.Contributors {
		if c.StateClass != StateWorkloadStorage || !c.Durable || c.Ownership != "application" {
			continue
		}
		present := false
		for _, m := range manifest.Contributors {
			if m.StateClass == c.StateClass && m.LogicalResource == c.LogicalResource {
				present = true
				break
			}
		}
		if !present {
			t.Fatalf("fail-closed violation: owned workload volume %q is declared by selection but absent from recovery manifest", c.LogicalResource)
		}
	}
}

// TestRecoveryManifestDiscoveryDoesNotInventWorkloadVolumeStateClass guards that
// DiscoverManifestRecovery only contributes workload.storage for genuinely
// declared owned workload storage, never a fabricated one.
func TestRecoveryManifestDiscoveryDoesNotInventWorkloadVolumeStateClass(t *testing.T) {
	m := application.New("demo", "dev", false, false, true)
	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range selection.Contributors {
		if c.StateClass == StateWorkloadStorage {
			t.Fatalf("undeclared workload.storage contributor invented by discovery: %#v", c)
		}
	}
}
