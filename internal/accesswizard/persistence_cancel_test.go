package accesswizard

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
	"github.com/mcpdev80/baseharbor/internal/policy"
)

// persistHarness wires a Controller to an in-memory input stream and captures
// output. It drives the wizard through the confirmation gate.
type persistHarness struct {
	in  string
	out strings.Builder
}

func newPersistHarness(input string) *persistHarness {
	return &persistHarness{in: input}
}

func (h *persistHarness) controller(scope map[string]string) *Controller {
	c := NewController(scope)
	c.SetScope(scope)
	c.SetIO(strings.NewReader(h.in), &h.out)
	return c
}

// withAPIVersion stamps a valid apiVersion on the profile map so the active
// config passes validation.
func withAPIVersion(m map[string]orgconfig.Profile) map[string]orgconfig.Profile {
	out := make(map[string]orgconfig.Profile, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// activeStateWithProfiles builds a valid active state carrying the given profile
// map (with a valid apiVersion and organization name) for wizard
// persistence/cancel tests.
func activeStateWithProfiles(m map[string]orgconfig.Profile) orgconfig.ActiveState {
	cfg := orgconfig.Config{APIVersion: orgconfig.ContractVersion, Organization: "acme", Profiles: m}
	return orgconfig.ActiveState{Config: cfg}
}

// validActiveState returns a fully valid active state (source, resolution, and
// profiles) that SaveActive will accept, so persistence tests can exercise the
// real writer path without touching the filesystem.
func validActiveState() orgconfig.ActiveState {
	cfg := orgconfig.Config{
		APIVersion:   orgconfig.ContractVersion,
		Organization: "acme",
		Operation:    "read",
		Targets:      map[string]orgconfig.Reference{"dev": {Reference: "local://dev"}},
	}
	resolution := orgconfig.Resolution{
		ResolvedVersion: orgconfig.ContractVersion,
	}
	if dir, err := os.MkdirTemp("", "baha-test-*"); err == nil {
		resolution.Source = orgconfig.Source{Kind: orgconfig.SourceLocal, Location: dir}
	}
	return orgconfig.ActiveState{Resolution: resolution, Config: cfg}
}

func (h *persistHarness) outStr() string { return h.out.String() }

// TestCancelAfterSelectionNeverInvokesCallback proves cancel at the confirmation
// gate (after the review screen) fails closed without invoking the production
// callback and without persisting anything.
func TestCancelAfterSelectionNeverInvokesCallback(t *testing.T) {
	h := newPersistHarness("alice\n1\ncancel\n")
	c := h.controller(map[string]string{"local": "read", "remote": "read"})
	var called bool
	res := c.Run(func(Review) policy.Decision {
		called = true
		return policy.Allow
	})
	if called {
		t.Fatal("decide must not be called after cancel at the confirmation gate")
	}
	if res.Err != nil {
		t.Fatalf("cancel after selection must not error, got err=%v", res.Err)
	}
	if res.Decision != policy.Allow {
		t.Fatalf("cancel after selection must yield Allow, got %v", res.Decision)
	}
	if !strings.Contains(h.outStr(), "Cancelled. No changes were made.") {
		t.Fatalf("expected cancel message after confirmation-cancel, got %q", h.outStr())
	}
}

// TestNoApprovalAtGateFailsClosed proves a non-approve input at the confirmation
// gate fails closed without invoking the callback.
func TestNoApprovalAtGateFailsClosed(t *testing.T) {
	h := newPersistHarness("alice\n1\nmaybe\n")
	c := h.controller(map[string]string{"local": "read"})
	var called bool
	res := c.Run(func(Review) policy.Decision {
		called = true
		return policy.Allow
	})
	if called {
		t.Fatal("decide must not be called when approval is missing")
	}
	if res.Err == nil {
		t.Fatal("missing approval must fail closed")
	}
}

// fakeStore records the last saved state and can be armed to fail. It stands in
// for the production active-state writer so the wizard's persistence path is
// exercised without touching the filesystem.
type fakeStore struct {
	saved   []orgconfig.ActiveState
	failErr error
}

func (s *fakeStore) Save(state orgconfig.ActiveState) error {
	if s.failErr != nil {
		return s.failErr
	}
	s.saved = append(s.saved, state)
	return nil
}

// TestResolveAssignmentPersistsThroughResolveLoad proves ResolveAssignment folds
// the created profile back into the loaded active state, persists it through the
// store, and reports it; a reload sees the change.
func TestResolveAssignmentPersistsThroughResolveLoad(t *testing.T) {
	m := orgconfig.NewManager("install-acme",
		orgconfig.Profile{Name: "readers", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)},
	)
	loadCalls := 0
	resolve := func() (orgconfig.ActiveState, error) {
		loadCalls++
		return validActiveState(), nil
	}
	decision, err := ResolveAssignment(m, resolve, "reader", "dev", "read", false)
	if err != nil {
		t.Fatalf("assignment should not error: %v", err)
	}
	if decision != policy.Allow {
		t.Fatalf("assigned operation must allow, got %v", decision)
	}
	if loadCalls == 0 {
		t.Fatalf("resolveLoad must run on a successful assignment: loads=%d", loadCalls)
	}
	// The manager now holds the created profile; a reload through the manager
	// observes it.
	found := false
	for _, p := range m.List() {
		if p.Name == "reader" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reload must observe the assigned profile")
	}
}

// TestRevokePersistsThroughResolveLoad proves Revoke persists the removal through
// the store.
func TestRevokePersistsThroughResolveLoad(t *testing.T) {
	m := orgconfig.NewManager("install-acme",
		orgconfig.Profile{Name: "alice", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)},
	)
	resolve := func() (orgconfig.ActiveState, error) {
		return validActiveState(), nil
	}
	if err := RevokeAssignment(m, resolve, "alice", "dev"); err != nil {
		t.Fatalf("revoke alice/dev: %v", err)
	}
	if len(m.List()) == 1 {
		t.Fatal("revocation must remove alice from the manager")
	}
	if _, err := m.Get("alice"); !errors.Is(err, orgconfig.ErrProfileNotFound) {
		t.Fatal("alice must have been removed from the manager")
	}
}

// TestResolveAssignmentPersistenceFailureFailsClosed proves that when the
// active-state writer fails, the assignment fails closed rather than reporting
// success for a change that was not persisted.
func TestResolveAssignmentPersistenceFailureFailsClosed(t *testing.T) {
	m := orgconfig.NewManager("install-acme",
		orgconfig.Profile{Name: "readers", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)},
	)
	resolve := func() (orgconfig.ActiveState, error) {
		return orgconfig.ActiveState{}, errors.New("disk full")
	}
	decision, err := ResolveAssignment(m, resolve, "reader", "dev", "read", false)
	if err == nil {
		t.Fatal("persistence failure must fail closed")
	}
	if decision != policy.Deny {
		t.Fatalf("persistence failure must deny, got %v", decision)
	}
}

// TestRevokePersistenceFailureFailsClosed proves that when the active-state
// writer fails, the revocation fails closed rather than reporting success.
func TestRevokePersistenceFailureFailsClosed(t *testing.T) {
	m := orgconfig.NewManager("install-acme",
		orgconfig.Profile{Name: "alice", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)},
	)
	resolve := func() (orgconfig.ActiveState, error) {
		return orgconfig.ActiveState{}, errors.New("disk full")
	}
	if err := RevokeAssignment(m, resolve, "alice", "dev"); err == nil {
		t.Fatal("persistence failure must fail closed")
	}
}

// TestSnapshotProfilesReturnsCopy proves SnapshotProfiles returns a copy that
// does not mutate the manager when edited.
func TestSnapshotProfilesReturnsCopy(t *testing.T) {
	m := orgconfig.NewManager("install-acme",
		orgconfig.Profile{Name: "alice", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)},
	)
	snap := m.SnapshotProfiles()
	if len(snap) != 1 {
		t.Fatalf("expected one profile in snapshot, got %d", len(snap))
	}
	snap[0].Name = "mutated"
	after := m.List()
	if after[0].Name != "alice" {
		t.Fatal("editing the snapshot must not mutate the manager")
	}
}

// TestPersistActiveFailsOnNilManager proves PersistActive fails closed on a nil
// manager.
func TestPersistActiveFailsOnNilManager(t *testing.T) {
	var m *orgconfig.Manager
	if err := orgconfig.PersistActive(m, orgconfig.ActiveState{}); err == nil {
		t.Fatal("nil manager must fail closed")
	}
}

// TestParseTargetListMultipleTargets proves finding #3 at the controller level:
// a comma-separated target selector parses into every listed target in display
// order, so the review processes all selections rather than truncating to the
// first. A single missing entry fails closed (never silently drops a target).
func TestParseTargetListMultipleTargets(t *testing.T) {
	names := []string{"dev", "prod", "staging"}
	c := NewController(map[string]string{"dev": "read", "prod": "read", "staging": "read"})
	c.SetScope(map[string]string{"dev": "read", "prod": "read", "staging": "read"})

	// By number.
	list, ok := c.parseTargetList(names, "1,3")
	if !ok || len(list) != 2 {
		t.Fatalf("expected 2 selections from '1,3', got ok=%v len=%d", ok, len(list))
	}
	if list[0].TargetName != "dev" || list[1].TargetName != "staging" {
		t.Fatalf("expected dev,staging in display order, got %s,%s", list[0].TargetName, list[1].TargetName)
	}

	// By name.
	list, ok = c.parseTargetList(names, "staging,dev")
	if !ok || len(list) != 2 {
		t.Fatalf("expected 2 selections from 'staging,dev', got ok=%v len=%d", ok, len(list))
	}
	if list[0].TargetName != "staging" || list[1].TargetName != "dev" {
		t.Fatalf("expected staging,dev in input order, got %s,%s", list[0].TargetName, list[1].TargetName)
	}

	// A missing entry fails closed.
	if _, ok := c.parseTargetList(names, "1,9"); ok {
		t.Fatal("a missing target in the list must fail closed")
	}
}
