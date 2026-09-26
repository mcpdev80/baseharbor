package applicationbackup

import (
	"strings"
	"testing"
)

func TestRecoverySelectionDefaultsOnlySupportedContributors(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateSQL, LogicalResource: "primary", Ownership: "application", Support: RecoverySupported, DefaultSelected: true},
		{StateClass: StateLogs, LogicalResource: "default", Ownership: "application", Support: RecoveryUnsupported, Reason: "provider has no scoped recovery"},
		{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	selected := selection.Selected()
	if len(selected) != 2 {
		t.Fatalf("selected %d contributors, want 2", len(selected))
	}
	if selected[0].StateClass != StateApplicationMetadata || selected[1].StateClass != StateSQL {
		t.Fatalf("unexpected selected contributors: %#v", selected)
	}
}

func TestRecoverySelectionIncludeFailsClosedForUnsupportedState(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateObjectStorage, LogicalResource: "uploads", Ownership: "application", Support: RecoveryUnsupported, Reason: "capture not implemented"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = selection.Apply([]RecoveryStateClass{StateObjectStorage}, nil)
	if err == nil || !strings.Contains(err.Error(), "no supported") {
		t.Fatalf("expected unsupported include failure, got %v", err)
	}
}

func TestRecoverySelectionRejectsUnknownOrConflictingSelectors(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateSQL, LogicalResource: "primary", Ownership: "application", Support: RecoverySupported, DefaultSelected: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selection.Apply([]RecoveryStateClass{"unknown"}, nil); err == nil {
		t.Fatal("expected unknown state class failure")
	}
	if _, err := selection.Apply([]RecoveryStateClass{StateSQL}, []RecoveryStateClass{StateSQL}); err == nil {
		t.Fatal("expected conflicting selector failure")
	}
}

func TestRecoveryContributorRequiresLogicalResourceOutsideMetadata(t *testing.T) {
	_, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateSecrets, Ownership: "application", Support: RecoverySupported},
	})
	if err == nil {
		t.Fatal("expected logical resource validation failure")
	}
}

func TestRecoverySelectionRequiresExplicitExclusionForUnsupportedDurableState(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
		{StateClass: StateObjectStorage, LogicalResource: "uploads", Ownership: "application", Support: RecoveryUnsupported, Durable: true, Reason: "capture not implemented"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := selection.ValidateForCapture(); err == nil {
		t.Fatal("expected unsupported durable state to block capture")
	}
	selection, err = selection.Apply(nil, []RecoveryStateClass{StateObjectStorage})
	if err != nil {
		t.Fatal(err)
	}
	if err := selection.ValidateForCapture(); err != nil {
		t.Fatalf("explicit exclusion should allow partial recovery unit: %v", err)
	}
	if !selection.Contributors[1].ExplicitlyExcluded {
		t.Fatal("explicit exclusion was not recorded")
	}
}

func TestRecoverySelectionCannotExcludeApplicationMetadata(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true, Durable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selection.Apply(nil, []RecoveryStateClass{StateApplicationMetadata}); err == nil {
		t.Fatal("expected application metadata exclusion to fail")
	}
}


func TestRecoverySelectionIncludesOwnedResourcesWithoutClaimingExternalPeers(t *testing.T) {
	selection, err := NewRecoverySelection([]RecoveryContributor{
		{StateClass: StateApplicationMetadata, Ownership: "application", Support: RecoverySupported, DefaultSelected: true},
		{StateClass: StateWorkloadStorage, LogicalResource: "data", Ownership: "application", Support: RecoverySupported},
		{StateClass: StateWorkloadStorage, LogicalResource: "host-cache", Ownership: "external", Support: RecoveryExternal, ExplicitlyExcluded: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err = selection.Apply([]RecoveryStateClass{StateWorkloadStorage}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Contributors[1].Selected {
		t.Fatal("owned workload storage was not selected")
	}
	if selection.Contributors[2].Selected || !selection.Contributors[2].ExplicitlyExcluded {
		t.Fatalf("external workload storage ownership changed: %#v", selection.Contributors[2])
	}
}
