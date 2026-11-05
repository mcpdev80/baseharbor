package policy

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// TestResolveAssignsAllowProvesAssignmentAdapter proves a profile that assigns
// an operation resolves to an Allow Finding; the adapter confers no right, it
// only reports membership in the assigned set and contributes it to the single
// policy authority.
func TestResolveAssignsAllowProvesAssignmentAdapter(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	ops := orgconfig.CanonicalOperations(orgconfig.OpRead, orgconfig.OpChange)
	if _, err := m.Create(orgconfig.Profile{Name: "readers", Target: "dev", Operations: ops}, false); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	a := NewAdapter(m)

	got, err := a.Resolve("readers", "install-acme", orgconfig.OpRead)
	if err != nil {
		t.Fatalf("assigned operation should not error: %v", err)
	}
	if got.Decision != Allow {
		t.Fatalf("assigned operation should allow, got %v", got.Decision)
	}
	// The single production authority aggregates the finding to Allow.
	if ResolveFinding(nil, got) != Allow {
		t.Fatalf("ResolveFinding should be Allow for an assigned operation")
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
	got, err := a.Resolve("readers", "install-acme", orgconfig.OpAdmin)
	if err != nil {
		t.Fatalf("resolve should not error for a known profile: %v", err)
	}
	if got.Decision != Deny {
		t.Fatalf("unassigned operation must deny, got %v", got.Decision)
	}
	if ResolveFinding(nil, got) != Deny {
		t.Fatalf("aggregate must deny an unassigned operation")
	}
}

// TestResolveUnknownProfileFailsClosed proves resolving an unknown profile fails
// closed rather than defaulting to allow.
func TestResolveUnknownProfileFailsClosed(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	a := NewAdapter(m)
	if _, err := a.Resolve("ghost", "install-acme", orgconfig.OpRead); err == nil {
		t.Fatal("unknown profile should fail closed, got nil error")
	}
}

// TestResolveEmptyManagerFailsClosed proves an adapter over a nil manager fails
// closed rather than allowing.
func TestResolveEmptyManagerFailsClosed(t *testing.T) {
	a := NewAdapter(nil)
	if _, err := a.Resolve("any", "install-acme", orgconfig.OpRead); err == nil {
		t.Fatal("nil manager must fail closed, got nil error")
	}
}

// TestResolveForeignTenantFailsClosed proves a call from a foreign installation
// identity cannot resolve another installation's profile: the adapter binds to
// the one authority that owns the bound Manager.
func TestResolveForeignTenantFailsClosed(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	if _, err := m.Create(orgconfig.Profile{Name: "readers", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)}, false); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	a := NewAdapter(m)
	if _, err := a.Resolve("readers", "install-other", orgconfig.OpRead); err == nil {
		t.Fatal("foreign tenant must fail closed, got nil error")
	}
}

// TestResolveRevokedViaAdapterFailsClosed proves a revoked profile fails closed
// through the SAME adapter: a caller must not be able to exercise a right by
// re-resolving a profile that was removed.
func TestResolveRevokedViaAdapterFailsClosed(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	if _, err := m.Create(orgconfig.Profile{Name: "readers", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)}, false); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	a := NewAdapter(m)
	// Assigned before revoke.
	if got, err := a.Resolve("readers", "install-acme", orgconfig.OpRead); err != nil || got.Decision != Allow {
		t.Fatalf("pre-revoke assign should allow: got %v err %v", got.Decision, err)
	}
	// After revoke, the same adapter resolves the removed name as failed closed.
	if _, err := m.Revoke("readers"); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if _, err := a.Resolve("readers", "install-acme", orgconfig.OpRead); err == nil {
		t.Fatal("revoked profile must deny through the same adapter, got nil error")
	}
}

// TestResolveAggregatePreservesMandatoryDeny proves the adapter's Allow Finding
// does not widen any other mandatory deny: a foreign-tenant Deny finding folds
// into the aggregate as Deny even alongside an assignment Allow.
func TestResolveAggregatePreservesMandatoryDeny(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	if _, err := m.Create(orgconfig.Profile{Name: "readers", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)}, false); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	a := NewAdapter(m)
	allowFinding, err := a.Resolve("readers", "install-acme", orgconfig.OpRead)
	if err != nil || allowFinding.Decision != Allow {
		t.Fatalf("expected Allow finding, got %v err %v", allowFinding.Decision, err)
	}
	// A foreign tenant Deny on the same call must force the aggregate to Deny.
	foreign := Finding{Code: "access-profile", Subject: "readers", Scope: "profile", Decision: Deny, Source: SourceDefault, Message: "foreign tenant"}
	if got := ResolveFinding([]Finding{allowFinding}, foreign); got != Deny {
		t.Fatalf("aggregate must be Deny when a mandatory foreign-tenant Deny is present, got %v", got)
	}
}
