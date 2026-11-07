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
	if err := RevokeAssignment(manager, nil, "alice", "dev"); err != nil {
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
			if err := RevokeAssignment(manager, nil, tc.name, tc.target); err == nil {
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
	if err := RevokeAssignment(nil, nil, "alice", "dev"); !errors.Is(err, ErrDenied) {
		t.Fatalf("missing authority should fail closed: %v", err)
	}
}
