package delivery

import "testing"

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
