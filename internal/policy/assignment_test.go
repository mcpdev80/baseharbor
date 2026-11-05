package policy

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// TestResolveAssignsAllowProvesAssignmentAdapter proves a profile that assigns
// an operation resolves to Allow; the adapter confers no right, it only reports
// membership in the assigned set.
func TestResolveAssignsAllowProvesAssignmentAdapter(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	ops := orgconfig.CanonicalOperations(orgconfig.OpRead, orgconfig.OpChange)
	if _, err := m.Create(orgconfig.Profile{Name: "readers", Target: "dev", Operations: ops}, false); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	a := NewAdapter(m)

	got, err := a.Resolve("readers", orgconfig.OpRead)
	if err != nil || got != Allow {
		t.Fatalf("assigned operation should allow: got %v err %v", got, err)
	}
}

// TestResolveOutsideAssignmentDeniesProvesNoSilentWiden proves an operation the
// profile does not assign resolves to Deny (not Warn), so a missing assignment
// cannot silently widen access.
func TestResolveOutsideAssignmentDeniesProvesNoSilentWiden(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	if _, err := m.Create(orgconfig.Profile{Name: "readers", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)}, false); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	a := NewAdapter(m)
	got, err := a.Resolve("readers", orgconfig.OpAdmin)
	if err != nil {
		t.Fatalf("resolve should not error for a known profile: %v", err)
	}
	if got != Deny {
		t.Fatalf("unassigned operation must deny, got %v", got)
	}
}

// TestResolveUnknownProfileFailsClosed proves resolving an unknown profile fails
// closed rather than defaulting to allow.
func TestResolveUnknownProfileFailsClosed(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	a := NewAdapter(m)
	if _, err := a.Resolve("ghost", orgconfig.OpRead); err == nil {
		t.Fatal("unknown profile should fail closed, got nil error")
	}
}

// TestResolveEmptyManagerFailsClosed proves an adapter over a nil manager fails
// closed rather than allowing.
func TestResolveEmptyManagerFailsClosed(t *testing.T) {
	a := NewAdapter(nil)
	got, err := a.Resolve("any", orgconfig.OpRead)
	if err == nil || got != Deny {
		t.Fatalf("nil manager must deny, got %v err %v", got, err)
	}
}
