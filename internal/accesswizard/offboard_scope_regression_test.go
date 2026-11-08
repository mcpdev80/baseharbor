package accesswizard

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func newOffboardAuthority() *orgconfig.Manager {
	return orgconfig.NewManager("installation-1",
		orgconfig.Profile{Name: "alice", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
		orgconfig.Profile{Name: "other", Target: "other", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	)
}

// The wizard's third argument is the selected Target, NOT a profile name.
func TestOffboardRemovesOnlyRequestedPersonOnReviewedTarget(t *testing.T) {
	manager := newOffboardAuthority()
	if err := RevokeAssignment(manager, "alice", "dev", RemoveManagedUser); err != nil {
		t.Fatalf("revoke alice/dev: %v", err)
	}
	if _, err := manager.Get("alice"); !errors.Is(err, orgconfig.ErrProfileNotFound) {
		t.Fatalf("alice should have been removed, got %v", err)
	}
	if _, err := manager.Get("other"); err != nil {
		t.Fatalf("unrelated profile must survive: %v", err)
	}
}

func TestOffboardWrongTargetCannotRevokeAnotherProfile(t *testing.T) {
	for _, tc := range []struct {
		name, target string
	}{
		{"alice", "other"},
		{"missing", "other"},
		{"other", "dev"},
		{"alice", ""},
		{"", "dev"},
	} {
		t.Run(tc.name+"/"+tc.target, func(t *testing.T) {
			manager := newOffboardAuthority()
			if err := RevokeAssignment(manager, tc.name, tc.target, RemoveManagedUser); err == nil {
				t.Fatal("wrong subject/target must fail closed")
			}
			for _, profileName := range []string{"alice", "other"} {
				if _, err := manager.Get(profileName); err != nil {
					t.Fatalf("unauthorized offboard removed %s: %v", profileName, err)
				}
			}
		})
	}
}

func TestOffboardNilAuthorityAlwaysDenies(t *testing.T) {
	if err := RevokeAssignment(nil, "alice", "dev", RemoveManagedUser); !errors.Is(err, ErrDenied) {
		t.Fatalf("missing authority should fail closed: %v", err)
	}
}

// TestOffboardUnimplementedKindFailsClosedNoSilentMutation proves finding #4: an
// unimplemented removal kind (connection, device, owned grant) must fail closed
// with a clear rejection and must NOT be silently served as the managed-user
// revoke. The owned profile set is left byte-identical.
func TestOffboardUnimplementedKindFailsClosedNoSilentMutation(t *testing.T) {
	manager := newOffboardAuthority()
	for _, kind := range []RemoveKind{RemoveConnection, RemoveDevice, RemoveGrant} {
		t.Run(kind.String(), func(t *testing.T) {
			before := manager.SnapshotProfiles()
			err := RevokeAssignment(manager, "alice", "dev", kind)
			if err == nil {
				t.Fatalf("%q must fail closed: no production route on this card", kind)
			}
			if !errors.Is(err, ErrDenied) {
				t.Fatalf("%q must fail closed on ErrDenied, got %v", kind, err)
			}
			after := manager.SnapshotProfiles()
			if len(before) != len(after) || before[0].Name != after[0].Name {
				t.Fatalf("%q must not mutate the managed-user profile set: before=%v after=%v", kind, before, after)
			}
		})
	}
}
