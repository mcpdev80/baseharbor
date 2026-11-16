package delivery

import (
	"encoding/json"
	"testing"
)

func TestDirectSelectionDefaultsAndValidates(t *testing.T) {
	got := (Selection{}).Normalize()
	if got != Direct() {
		t.Fatalf("normalize empty = %#v, want %#v", got, Direct())
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryOwnershipIsSingleAndModeBound(t *testing.T) {
	cases := []struct {
		name string
		sel  Selection
		ok   bool
	}{
		{"direct", Direct(), true},
		{"delegated", Selection{Mode: ModeDelegated, Provider: "gitops/example", Owner: OwnerExternal, Placement: PlacementApplication}, true},
		{"direct-external-owner", Selection{Mode: ModeDirect, Provider: "direct", Owner: OwnerExternal}, false},
		{"delegated-baseharbor-owner", Selection{Mode: ModeDelegated, Provider: "gitops/example", Owner: OwnerBaseHarbor}, false},
		{"delegated-direct-provider", Selection{Mode: ModeDelegated, Provider: "direct", Owner: OwnerExternal}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sel.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// Exactly one reconciliation owner per delivery mode: direct is BaseHarbor
// bound, delegated is external bound. A direct selection that carries an
// external owner, or a delegated selection that carries a BaseHarbor owner,
// is a closed-failure at the typed selection level.
func TestDelegatedOwnerDenialAtSelectionLevel(t *testing.T) {
	cases := []struct {
		name string
		sel  Selection
		ok   bool
	}{
		{"direct-baseharbor", Direct(), true},
		{"delegated-external", Selection{Mode: ModeDelegated, Provider: "gitops/argocd", Owner: OwnerExternal, Placement: PlacementExternal}, true},
		{"direct-external-owner-denied", Selection{Mode: ModeDirect, Provider: "direct", Owner: OwnerExternal}, false},
		{"delegated-baseharbor-owner-denied", Selection{Mode: ModeDelegated, Provider: "gitops/argocd", Owner: OwnerBaseHarbor}, false},
		{"delegated-must-not-use-direct-provider", Selection{Mode: ModeDelegated, Provider: "direct", Owner: OwnerExternal}, false},
		{"direct-must-use-direct-provider", Selection{Mode: ModeDirect, Provider: "gitops/argocd", Owner: OwnerBaseHarbor}, false},
		{"empty-owner-denied", Selection{Mode: ModeDelegated, Provider: "gitops/argocd", Owner: ""}, false},
		{"unknown-owner-denied", Selection{Mode: ModeDelegated, Provider: "gitops/argocd", Owner: "thirdparty"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sel.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// Provenance and immutable revision may be recorded on a valid selection
// without changing portable intent; they are carried but never resolve mode or owner.
func TestSelectionCarriesProvenanceWithoutResolvingOwnership(t *testing.T) {
	got := Selection{
		Mode:       ModeDelegated,
		Provider:   "gitops/argocd",
		Owner:      OwnerExternal,
		Placement:  PlacementExternal,
		Revision:   "abc123",
		Provenance: "git://example/.github/workflows/deploy.yaml@abc123",
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Normalize().Revision != "abc123" {
		t.Fatalf("revision not carried: %#v", got.Normalize())
	}
	if got.Normalize().Provenance != "git://example/.github/workflows/deploy.yaml@abc123" {
		t.Fatalf("provenance not carried: %#v", got.Normalize())
	}
}

// The typed selection contract survives a JSON round-trip: the JSON field
// names are part of the portable contract and must not drift.
func TestSelectionContractFixturesJSONRoundTrip(t *testing.T) {
	original := Selection{
		Mode:      ModeDelegated,
		Provider:  "gitops/argocd",
		Owner:     OwnerExternal,
		Placement: PlacementExternal,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Selection
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Fatalf("round-trip changed selection: %s -> %#v", string(data), decoded)
	}
	if string(data) != `{"mode":"delegated","provider":"gitops/argocd","owner":"external","placement":"external"}` {
		t.Fatalf("unexpected JSON shape: %s", string(data))
	}
}
