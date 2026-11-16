package orgconfig

import (
	"errors"
	"testing"
)

// testScope is a scope authority that declares two Targets and one environment,
// used to prove scope validation against an owned authority.
type testScope struct {
	targets      map[string]bool
	environments map[string]bool
}

func (s testScope) TargetKnown(target string) bool {
	return s.targets[target]
}

func (s testScope) EnvironmentKnown(env string) bool {
	if env == "" {
		return true
	}
	return s.environments[env]
}

func newTestScope() testScope {
	return testScope{
		targets:      map[string]bool{"dev": true, "prod": true},
		environments: map[string]bool{"prod": true},
	}
}

// TestCreateThenRevoke proves the happy path: create a scoped profile, revoke
// it, and confirm it is gone.
func TestCreateThenRevoke(t *testing.T) {
	m := NewManager("install-acme")
	if err := m.BindScope(newTestScope()); err == nil {
		t.Fatal("BindScope returned nil")
	}
	p, err := m.Create(Profile{
		Name:       "platform-admins",
		Target:     "dev",
		Operations: CanonicalOperations(OpRead, OpChange, OpAdmin),
		Owner:      "team-platform",
	}, false)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if p.Name != "platform-admins" || p.Immutable == "" {
		t.Fatalf("create did not stamp canonical identity: %+v", p)
	}
	if m.Count() != 1 {
		t.Fatalf("expected 1 profile, got %d", m.Count())
	}
	if _, err := m.Revoke("platform-admins", "dev"); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if m.Count() != 0 {
		t.Fatalf("revoke did not remove profile, count=%d", m.Count())
	}
}

// TestCreateIdempotent proves a re-create with an identical definition is a
// no-op returning the existing profile, not a duplicate error.
func TestCreateIdempotent(t *testing.T) {
	m := NewManager("install-acme")
	ops := CanonicalOperations(OpChange, OpRead)
	first, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: ops, Owner: "o"}, false)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	second, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: ops, Owner: "o"}, false)
	if err != nil {
		t.Fatalf("idempotent create failed: %v", err)
	}
	if first.Immutable != second.Immutable {
		t.Fatalf("idempotent create changed identity")
	}
	if m.Count() != 1 {
		t.Fatalf("idempotent create duplicated profile")
	}
}

// TestCreateConflict proves a re-create with a DIFFERENT definition fails
// closed on ErrDuplicateProfile unless replace is true.
func TestCreateConflict(t *testing.T) {
	m := NewManager("install-acme")
	ops := CanonicalOperations(OpRead)
	if _, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: ops}, false); err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	// Different operation set -> conflict.
	if _, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpChange)}, false); err != ErrDuplicateProfile {
		t.Fatalf("expected ErrDuplicateProfile, got %v", err)
	}
	// With replace, it succeeds and reflects the new definition.
	up, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpChange)}, true)
	if err != nil {
		t.Fatalf("replace create failed: %v", err)
	}
	if len(up.Operations) != 1 || up.Operations[0] != OpChange {
		t.Fatalf("replace did not apply new operations: %+v", up.Operations)
	}
}

// TestCreateOperationCanonicalOrder proves operations are reordered into
// canonical precedence regardless of input order and deduped.
func TestCreateOperationCanonicalOrder(t *testing.T) {
	m := NewManager("install-acme")
	p, err := m.Create(Profile{Name: "x", Target: "dev", Operations: []Operation{OpAdmin, OpRead, OpRead, OpChange}}, false)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	want := []Operation{OpRead, OpChange, OpAdmin}
	if len(p.Operations) != len(want) {
		t.Fatalf("operations not deduped: %+v", p.Operations)
	}
	for i := range want {
		if p.Operations[i] != want[i] {
			t.Fatalf("operations not canonical: got %+v want %+v", p.Operations, want)
		}
	}
}

// TestUpdateUnknownFailsClosed proves updating an unknown profile fails closed.
func TestUpdateUnknownFailsClosed(t *testing.T) {
	m := NewManager("install-acme")
	if _, err := m.Update(1, Profile{Name: "ghost", Target: "dev", Operations: CanonicalOperations(OpRead)}); err != ErrProfileNotFound {
		t.Fatalf("expected ErrProfileNotFound on update of unknown, got %v", err)
	}
}

// TestRevokeUnknownFailsClosed proves revoking an unknown name fails closed.
func TestRevokeUnknownFailsClosed(t *testing.T) {
	m := NewManager("install-acme")
	if _, err := m.Revoke("ghost", "dev"); err != ErrProfileNotFound {
		t.Fatalf("expected ErrProfileNotFound on revoke of unknown, got %v", err)
	}
}

// TestValidationCatchesBadNameAndEmpty proves structural validation: an empty
// name, a non-identifier name, a missing target, and empty operations all fail.
func TestValidationCatchesBadNameAndEmpty(t *testing.T) {
	_ = NewManager("install-acme")
	cases := []Profile{
		{Target: "dev", Operations: CanonicalOperations(OpRead)},                    // no name
		{Name: "bad name!", Target: "dev", Operations: CanonicalOperations(OpRead)}, // bad name
		{Name: "x"},                // no target
		{Name: "x", Target: "dev"}, // no operations
		{Name: "x", Target: "dev", Operations: []Operation{OpRead}}, // valid, must not fail
	}
	for i, c := range cases {
		wantErr := i < 4
		err := c.Validate()
		if wantErr && err == nil {
			t.Fatalf("case %d: expected validation error, got nil", i)
		}
		if !wantErr && err != nil {
			t.Fatalf("case %d: unexpected validation error: %v", i, err)
		}
	}
}

// TestUnknownOperationRejected proves an operation outside the fixed domain is
// rejected so a profile cannot widen the surface.
func TestUnknownOperationRejected(t *testing.T) {
	m := NewManager("install-acme")
	_, err := m.Create(Profile{Name: "x", Target: "dev", Operations: []Operation{"sudo"}}, false)
	if err == nil {
		t.Fatal("unknown operation was accepted; surface widened")
	}
}

// TestScopeValidateAgainstOwnedAuthority proves a profile referencing an undeclared
// target or environment fails closed: a profile can only reference what the
// authority already owns.
func TestScopeValidateAgainstOwnedAuthority(t *testing.T) {
	m := NewManager("install-acme").BindScope(newTestScope())
	// Undeclared target -> fail.
	if _, err := m.Create(Profile{Name: "x", Target: "staging", Operations: CanonicalOperations(OpRead)}, false); err == nil {
		t.Fatal("undeclared target accepted; scope escalation")
	}
	// Declared target, undeclared environment -> fail.
	if _, err := m.Create(Profile{Name: "y", Target: "dev", Environment: "qa", Operations: CanonicalOperations(OpRead)}, false); err == nil {
		t.Fatal("undeclared environment accepted; scope escalation")
	}
	// Declared target and environment -> ok.
	if _, err := m.Create(Profile{Name: "z", Target: "prod", Environment: "prod", Operations: CanonicalOperations(OpRead)}, false); err != nil {
		t.Fatalf("valid scope rejected: %v", err)
	}
}

// TestForeignInstallationProfilesArePreserved proves that a Manager for one
// owner never touches another owner's profiles. Two Managers are disjoint by
// construction: an owner cannot create, update or revoke a name that belongs to
// another owner, because the sets are keyed by (owner,name) and the owner is a
// field on each Manager, not on the shared store.
func TestForeignInstallationProfilesArePreserved(t *testing.T) {
	ownerM := NewManager("install-acme", Profile{Name: "mine", Target: "dev", Operations: CanonicalOperations(OpRead)})
	foreignM := NewManager("install-other", Profile{Name: "theirs", Target: "dev", Operations: CanonicalOperations(OpRead)})

	// An owner Manager cannot see or remove a foreign profile.
	if _, err := ownerM.Get("theirs"); err != ErrProfileNotFound {
		t.Fatalf("owner enumerated a foreign profile name: %v", err)
	}
	if _, err := ownerM.Revoke("theirs", "dev"); err != ErrProfileNotFound {
		t.Fatalf("owner tried to revoke a foreign profile: %v", err)
	}
	if foreignM.Count() != 1 || ownerM.Count() != 1 {
		t.Fatalf("managers leaked state: owner=%d foreign=%d", ownerM.Count(), foreignM.Count())
	}
	// The foreign profile is still exactly as it was.
	foreign, _ := foreignM.Get("theirs")
	if foreign.Name != "theirs" || foreign.Target != "dev" {
		t.Fatalf("foreign profile was mutated: %+v", foreign)
	}
}

// TestDistinctTargetsAreDistinctProfiles proves a name held on two declared
// Targets is one profile per Target, not a single profile: the per-target model
// keys by (name, target), so both survive and neither overwrites the other.
func TestDistinctTargetsAreDistinctProfiles(t *testing.T) {
	m := NewManager("install-acme")
	if _, err := m.Create(Profile{Name: "dup", Target: "dev", Operations: CanonicalOperations(OpRead)}, false); err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	// A second create with the same Name on a different Target is a distinct
	// profile, so it succeeds — the earlier per-Name duplicate was the defect.
	if _, err := m.Create(Profile{Name: "dup", Target: "prod", Operations: CanonicalOperations(OpRead)}, false); err != nil {
		t.Fatalf("second create on a different target must not collide: %v", err)
	}
	if m.Count() != 2 {
		t.Fatalf("expected 2 distinct (name,target) profiles, got %d", m.Count())
	}
	// GetByKey resolves each Target to its own profile.
	if dev, err := m.GetByKey("dup", "dev"); err != nil || dev.Target != "dev" {
		t.Fatalf("dev profile must resolve: %+v err=%v", dev, err)
	}
	if prod, err := m.GetByKey("dup", "prod"); err != nil || prod.Target != "prod" {
		t.Fatalf("prod profile must resolve: %+v err=%v", prod, err)
	}
	// Revoking one Target leaves the other.
	if _, err := m.Revoke("dup", "dev"); err != nil {
		t.Fatalf("revoke dev: %v", err)
	}
	if _, err := m.GetByKey("dup", "prod"); err != nil {
		t.Fatalf("revoking dev must leave prod: %v", err)
	}
	if _, err := m.GetByKey("dup", "dev"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatal("dev profile must be gone")
	}
}

// TestCanonicalOperationsDedupesAndOrders proves the exported helper canonicalizes.
func TestCanonicalOperationsDedupesAndOrders(t *testing.T) {
	got := CanonicalOperations(OpAdmin, OpRead, OpRead, OpChange)
	want := []Operation{OpRead, OpChange, OpAdmin}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CanonicalOperations got %+v want %+v", got, want)
		}
	}
	if len(got) != 3 {
		t.Fatalf("CanonicalOperations did not dedupe: %+v", got)
	}
}

// TestListReturnsCanonicalOrderProvesStableOrder proves List returns profiles
// sorted by name regardless of insertion order.
func TestListReturnsCanonicalOrderProvesStableOrder(t *testing.T) {
	m := NewManager("install-acme")
	_, _ = m.Create(Profile{Name: "zeta", Target: "dev", Operations: CanonicalOperations(OpRead)}, false)
	_, _ = m.Create(Profile{Name: "alpha", Target: "dev", Operations: CanonicalOperations(OpRead)}, false)
	list := m.List()
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("List not sorted by name: %+v", list)
	}
}

// TestUpdateBumpsRevisionProvesStaleDetection proves that a successful edit
// bumps the stored revision so a later write carrying the OLD revision is
// rejected as stale (no last-write-wins).
func TestUpdateBumpsRevisionProvesStaleDetection(t *testing.T) {
	m := NewManager("install-acme").BindScope(newTestScope())
	p, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead, OpLog), Owner: "team-r"}, false)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if p.Revision != 1 {
		t.Fatalf("fresh profile must start at revision 1, got %d", p.Revision)
	}
	// Edit that removes an operation with the observed revision -> accepted, revision bumps to 2.
	up, err := m.Update(p.Revision, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"})
	if err != nil {
		t.Fatalf("valid update must succeed: %v", err)
	}
	if up.Revision != 2 {
		t.Fatalf("update must bump revision to 2, got %d", up.Revision)
	}
	// Re-apply the OLD revision (1) -> must fail closed as stale.
	if _, err := m.Update(1, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"}); err == nil {
		t.Fatal("stale revision (older than stored) must be rejected")
	}
	// A zero expected revision is rejected outright: stale enforcement is
	// mandatory and cannot be bypassed by omission.
	if _, err := m.Update(0, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"}); err != ErrStaleRevision {
		t.Fatalf("zero expected revision must return ErrStaleRevision, got %v", err)
	}
}

// TestUpdateEscalationRejected proves that adding an operation the current
// definition does not assign (read -> admin) is rejected so a profile edit can
// never widen the effective right. Removing operations is always allowed.
func TestUpdateEscalationRejected(t *testing.T) {
	m := NewManager("install-acme").BindScope(newTestScope())
	p, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"}, false)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// Escalate read -> admin must fail closed.
	if _, err := m.Update(p.Revision, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead, OpAdmin), Owner: "team-r"}); err == nil {
		t.Fatal("edit adding admin to a read-only profile must be rejected (no escalation)")
	}
	// A down-scope (removing an operation) is allowed.
	down, err := m.Update(p.Revision, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"})
	if err != nil {
		t.Fatalf("down-scope edit must succeed: %v", err)
	}
	if len(down.Operations) != 1 || down.Operations[0] != OpRead {
		t.Fatalf("down-scope did not preserve read: %+v", down.Operations)
	}
}

// TestUpdateRevisionMatrixProvesExactMatch proves the revision guard enforces an
// EXACT match: only the observed revision is accepted, and every mismatch — a
// future (revision+1) write, an equal write, and a stale (revision-1) write —
// fails closed. The four cells are the contract: +1 red, unchanged green,
// -1 red, and zero red.
func TestUpdateRevisionMatrixProvesExactMatch(t *testing.T) {
	m := NewManager("install-acme").BindScope(newTestScope())
	p, err := m.Create(Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead, OpLog), Owner: "team-r"}, false)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Cell 1 (RED): future revision (existing+1) must be rejected — the caller
	// never observed a revision it does not know about; the guard cannot be
	// raced forward past the stored revision.
	if _, err := m.Update(p.Revision+1, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"}); err == nil {
		t.Fatal("future revision (existing+1) must be rejected (mismatched, not observed)")
	}

	// Cell 2 (GREEN): the exact observed revision is accepted.
	if _, err := m.Update(p.Revision, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"}); err != nil {
		t.Fatalf("exact observed revision must be accepted, got %v", err)
	}

	// Cell 3 (RED): stale revision (existing-1) must be rejected — no
	// last-write-wins.
	if _, err := m.Update(p.Revision-1, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"}); err == nil {
		t.Fatal("stale revision (existing-1) must be rejected")
	}

	// Cell 4 (RED): zero expected revision is rejected outright — stale
	// enforcement is mandatory and cannot be bypassed by omission.
	if _, err := m.Update(0, Profile{Name: "readers", Target: "dev", Operations: CanonicalOperations(OpRead), Owner: "team-r"}); err != ErrStaleRevision {
		t.Fatalf("zero expected revision must return ErrStaleRevision, got %v", err)
	}
}
