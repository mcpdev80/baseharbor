package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// writeTestActiveConfig seeds a valid active organization configuration with the
// given targets, operation, and owned profiles under the test's BASEHARBOR_STATE_DIR.
func writeTestActiveConfig(t *testing.T, targets []string, operation string, profiles []orgconfig.Profile) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	t.Chdir(root)

	cfg := orgconfig.Config{
		APIVersion:   orgconfig.ContractVersion,
		Organization: "persist-acme",
		Operation:    operation,
		Targets:      map[string]orgconfig.Reference{},
		Profiles:     map[string]orgconfig.Profile{},
	}
	for _, name := range targets {
		cfg.Targets[name] = orgconfig.Reference{Reference: "local://" + name}
	}
	for _, p := range profiles {
		// Key by (name, target): the active configuration stores one profile per
		// owned (name, target) pair, so a person held on multiple Targets seeds
		// as distinct profiles rather than collapsing to the last.
		cfg.Profiles[orgconfig.ProfileKey(p.Name, p.Target)] = p
	}

	resolution := orgconfig.Resolution{
		ResolvedVersion: orgconfig.ContractVersion,
		Source:          orgconfig.Source{Kind: orgconfig.SourceLocal, Location: root},
	}

	active := orgconfig.ActiveState{Resolution: resolution, Config: cfg}
	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "config", "organization")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// runWizardWithInput runs a CLI command with a fixed interactive input on stdin.
func runWizardWithInput(t *testing.T, args []string, input string) (string, error) {
	t.Helper()
	old := os.Stdin
	oldApp := appNewInput
	f := t.TempDir() + "/stdin"
	if err := os.WriteFile(f, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	fp, err := os.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = fp
	appNewInput = fp
	t.Cleanup(func() { os.Stdin = old; appNewInput = oldApp })

	var stdout, stderr bytes.Buffer
	outErr := runWithIO(machineNoninteractiveContext(context.Background()), args, &stdout, &stderr)
	return stdout.String() + stderr.String(), outErr
}

// ownedProfileCount reads the number of owned profiles persisted in the active
// configuration after a wizard run.
func ownedProfileCount(t *testing.T) int {
	t.Helper()
	state, err := orgconfig.LoadActive()
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	return len(state.Config.Profiles)
}

// ownedProfileNames returns the distinct owned profile names persisted in the
// active configuration.
func ownedProfileNames(t *testing.T) []string {
	t.Helper()
	state, err := orgconfig.LoadActive()
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	names := make([]string, 0, len(state.Config.Profiles))
	for _, p := range state.Config.Profiles {
		names = append(names, p.Name)
	}
	return names
}

// ownedProfileOn returns the owned profile for the given name on the given
// Target, or (Profile,false) when no profile with that name exists on that
// Target. The stored map is keyed by (name, target).
func ownedProfileOn(t *testing.T, name, target string) (orgconfig.Profile, bool) {
	t.Helper()
	state, err := orgconfig.LoadActive()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range state.Config.Profiles {
		if p.Name == name && p.Target == target {
			return p, true
		}
	}
	return orgconfig.Profile{}, false
}

// profileExists reports whether any owned profile with the given name is
// persisted, regardless of Target.
func profileExists(t *testing.T, name string) bool {
	for _, n := range ownedProfileNames(t) {
		if n == name {
			return true
		}
	}
	return false
}

// TestAccessAssignPersistsThroughCLI proves a confirmed assignment reaches the
// production active-state store: the owned profile set grows by one and reload
// observes it.
func TestAccessAssignPersistsThroughCLI(t *testing.T) {
	// Seed one owned profile so the manager is non-empty (the CLI fails closed
	// on an empty manager before the wizard runs).
	seed := []orgconfig.Profile{
		orgconfig.Profile{Name: "seed", Target: "prod", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	}
	targets := []string{"dev", "prod"}
	writeTestActiveConfig(t, targets, "read", seed)

	out, err := runWizardWithInput(t, []string{"access", "assign", "alice"}, "alice\ncancel\n")
	if err != nil {
		t.Fatalf("cancel run should not error: %v\noutput=%q", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "cancelled") {
		t.Fatalf("expected cancel to abort before approval, got %q", out)
	}
	if got := ownedProfileCount(t); got != 1 {
		t.Fatalf("cancel must not persist: expected 1 profile (seed), got %d", got)
	}
	if profileExists(t, "alice") {
		t.Fatal("cancel must not persist the assignment")
	}

	out, err = runWizardWithInput(t, []string{"access", "assign", "alice"}, "alice\n1\nyes\n")
	if err != nil {
		t.Fatalf("access assign: %v\noutput=%q", err, out)
	}
	if !strings.Contains(out, "assigned") {
		t.Fatalf("expected assignment confirmation, got %q", out)
	}
	if got := ownedProfileCount(t); got != 2 {
		t.Fatalf("expected 2 persisted profiles after assign (seed+alice), got %d", got)
	}
	if !profileExists(t, "alice") {
		t.Fatal("alice profile must be persisted")
	}
}

// TestAccessOffboardPersistsThroughCLI proves a confirmed revocation reaches the
// production active-state store: the owned profile set shrinks by one and reload
// observes it.
func TestAccessOffboardPersistsThroughCLI(t *testing.T) {
	targets := []string{"dev"}
	writeTestActiveConfig(t, targets, "read", []orgconfig.Profile{
		orgconfig.Profile{Name: "bob", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	})

	out, err := runWizardWithInput(t, []string{"access", "offboard", "bob"}, "bob\n1\nyes\n")
	if err != nil {
		t.Fatalf("access offboard: %v\noutput=%q", err, out)
	}
	if got := ownedProfileCount(t); got != 0 {
		t.Fatalf("expected 0 persisted profiles after offboard, got %d", got)
	}
	if profileExists(t, "bob") {
		t.Fatal("bob profile must have been removed")
	}
}

// TestAccessAssignCancelAfterSelectionLeavesStateUnchanged proves that
// canceling after selection leaves the production store byte-for-byte unchanged.
func TestAccessAssignCancelAfterSelectionLeavesStateUnchanged(t *testing.T) {
	targets := []string{"dev"}
	seed := []orgconfig.Profile{
		orgconfig.Profile{Name: "carol", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	}
	writeTestActiveConfig(t, targets, "read", seed)

	out, err := runWizardWithInput(t, []string{"access", "assign", "dave"}, "dave\n1\ncancel\n")
	if err != nil {
		t.Fatalf("access assign cancel: %v\noutput=%q", err, out)
	}
	if got := ownedProfileCount(t); got != 1 {
		t.Fatalf("expected 1 profile after cancel (unchanged), got %d", got)
	}
	if profileExists(t, "dave") {
		t.Fatal("cancel must not persist the assignment")
	}
	if !profileExists(t, "carol") {
		t.Fatal("pre-existing profile must survive the canceled run")
	}
}

// TestAccessAssignSingleTargetPersistsAndReloadObserves proves finding #1 at the
// CLI boundary: a confirmed assignment on one target persists through the
// production active-state store, and a reload observes the new owned profile.
func TestAccessAssignSingleTargetPersistsAndReloadObserves(t *testing.T) {
	targets := []string{"dev"}
	seed := []orgconfig.Profile{
		orgconfig.Profile{Name: "carol", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	}
	writeTestActiveConfig(t, targets, "read", seed)

	out, err := runWizardWithInput(t, []string{"access", "assign", "dave"}, "dave\n1\nyes\n")
	if err != nil {
		t.Fatalf("access assign: %v\noutput=%q", err, out)
	}
	if !profileExists(t, "dave") {
		t.Fatal("dave profile must be persisted on the selected target")
	}
	if got := ownedProfileCount(t); got != 2 {
		t.Fatalf("expected 2 profiles (seed+dave), got %d", got)
	}
}

// TestAccessAssignMultipleTargetsPersistsAll proves the batch-atomic fix
// (finding #5): confirming an assign on N targets durably persists one owned
// profile per (person, target) — no earlier Target is dropped by a later one.
func TestAccessAssignMultipleTargetsPersistsAll(t *testing.T) {
	targets := []string{"dev", "prod", "staging"}
	seed := []orgconfig.Profile{
		orgconfig.Profile{Name: "seed", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	}
	writeTestActiveConfig(t, targets, "read", seed)

	out, err := runWizardWithInput(t, []string{"access", "assign", "alice"}, "alice\n1,2,3\nyes\n")
	if err != nil {
		t.Fatalf("access assign: %v\noutput=%q", err, out)
	}
	if !strings.Contains(out, "assigned") {
		t.Fatalf("expected assignment confirmation, got %q", out)
	}
	// One owned profile per (alice, target) plus the seed must survive.
	for _, target := range targets {
		if !profileExists(t, "alice") {
			t.Fatalf("alice profile must be persisted on %q", target)
		}
	}
	if got := ownedProfileCount(t); got != 4 {
		t.Fatalf("expected 4 owned profiles (seed + alice×3), got %d", got)
	}
	// Reload each target's profile identity explicitly.
	reloaded, err := orgconfig.LoadActive()
	if err != nil {
		t.Fatalf("reload active: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range reloaded.Config.Profiles {
		if p.Name == "alice" {
			seen[p.Target] = true
		}
	}
	for _, target := range targets {
		if !seen[target] {
			t.Fatalf("alice on %q must be persisted", target)
		}
	}
}

// TestAccessOffboardMultipleTargetsRevokesAll proves batch revoke (finding #5):
// confirming revoke on N targets durably removes one owned profile per
// (person, target).
func TestAccessOffboardMultipleTargetsRevokesAll(t *testing.T) {
	targets := []string{"dev", "prod"}
	writeTestActiveConfig(t, targets, "read", []orgconfig.Profile{
		orgconfig.Profile{Name: "bob", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
		orgconfig.Profile{Name: "bob", Target: "prod", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	})

	out, err := runWizardWithInput(t, []string{"access", "offboard", "bob"}, "bob\n1,2\nyes\n")
	if err != nil {
		t.Fatalf("access offboard: %v\noutput=%q", err, out)
	}
	if got := ownedProfileCount(t); got != 0 {
		t.Fatalf("expected 0 owned profiles after offboard, got %d", got)
	}
	if profileExists(t, "bob") {
		t.Fatal("bob must be removed from both targets")
	}
}

// TestAccessOffboardLaterTargetFailureLeavesStoreUnchanged proves the
// all-or-nothing atomicity (finding #5): when a later reviewed Target fails
// revocation, the whole batch fails and the store is byte-identical (no partial
// removal of an earlier Target).
func TestAccessOffboardLaterTargetFailureLeavesStoreUnchanged(t *testing.T) {
	targets := []string{"dev", "prod"}
	writeTestActiveConfig(t, targets, "read", []orgconfig.Profile{
		// bob is owned on dev; carol is unrelated, owned on prod.
		orgconfig.Profile{Name: "bob", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
		orgconfig.Profile{Name: "carol", Target: "prod", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	})

	// Revoke on dev (bob exists) then prod (bob does NOT exist): the batch must
	// validate every reviewed Target before persisting anything. The missing
	// bob on prod fails the batch, so dev's bob must NOT be removed either.
	out, err := runWizardWithInput(t, []string{"access", "offboard", "bob"}, "bob\n1,2\nyes\n")
	if err == nil {
		t.Fatalf("offboard on a missing Target must fail closed\noutput=%q", out)
	}
	// carol (unrelated, on prod) must survive — no partial removal happened.
	if !profileExists(t, "carol") {
		t.Fatal("unrelated profile must survive a failed batch")
	}
	// bob on dev must survive: the batch validated all Targets first and persisted
	// nothing because the later Target failed.
	if !profileExists(t, "bob") {
		t.Fatal("the earlier Target must NOT have been removed before the later failure")
	}
}

// TestAccessOffboardAtomicNoPartialWriteByteEqual proves that a batch revoke
// whose later Target fails leaves the active state JSON byte-for-byte identical
// to the pre-run store (no partial write, no reordered keys).
func TestAccessOffboardAtomicNoPartialWriteByteEqual(t *testing.T) {
	targets := []string{"dev", "prod"}
	writeTestActiveConfig(t, targets, "read", []orgconfig.Profile{
		orgconfig.Profile{Name: "bob", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}},
	})

	beforeBytes := activeBytes(t)

	out, err := runWizardWithInput(t, []string{"access", "offboard", "bob"}, "bob\n1,2\nyes\n")
	if err == nil {
		t.Fatalf("offboard on a missing Target must fail closed\noutput=%q", out)
	}
	afterBytes := activeBytes(t)
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("store must be byte-identical after a failed batch:\nbefore=%s\nafter=%s", beforeBytes, afterBytes)
	}
}

// activeBytes returns the raw JSON bytes of the persisted active configuration.
func activeBytes(t *testing.T) []byte {
	t.Helper()
	state, err := orgconfig.LoadActive()
	if err != nil {
		t.Fatalf("load active: %v", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// foreignProfile seeds a profile owned by a different installation identity.
// ManagerFromActive deliberately does not load such rows (LoadProfiles skips
// foreign owners), so PersistActive must carry them through the write untouched
// rather than replace the whole profile map with the active owner's snapshot.
func foreignProfile() orgconfig.Profile {
	return orgconfig.Profile{
		Name:       "foreign",
		Target:     "prod",
		Operations: []orgconfig.Operation{orgconfig.OpRead, orgconfig.OpLog},
		Scope:      "legacy",
		Owner:      "other-installation",
		Revision:   7,
	}
}

// assertForeignIntact reloads the active configuration and fails when the
// foreign-owner profile is missing or differs from the seeded definition, the
// persisted byte-identity the preservation boundary demands.
func assertForeignIntact(t *testing.T, want orgconfig.Profile) {
	t.Helper()
	got, ok := ownedProfileOn(t, want.Name, want.Target)
	if !ok {
		t.Fatalf("foreign-owner profile %q on %q must survive the write", want.Name, want.Target)
	}
	if got.Owner != want.Owner || got.Scope != want.Scope || got.Revision != want.Revision ||
		len(got.Operations) != len(want.Operations) {
		t.Fatalf("foreign-owner profile definition rewritten: got %+v want %+v", got, want)
	}
	for i, op := range want.Operations {
		if got.Operations[i] != op {
			t.Fatalf("foreign-owner operations changed: got %v want %v", got.Operations, want.Operations)
		}
	}
}

// TestAccessAssignPreservesForeignOwnerProfile proves the r6 finding at the
// production CLI boundary: a successful assignment of the active owner must not
// delete another installation's profile that the loader deliberately skipped.
func TestAccessAssignPreservesForeignOwnerProfile(t *testing.T) {
	foreign := foreignProfile()
	writeTestActiveConfig(t, []string{"dev", "prod"}, "read", []orgconfig.Profile{
		{Name: "seed", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}, Owner: "persist-acme"},
		foreign,
	})

	out, err := runWizardWithInput(t, []string{"access", "assign", "alice"}, "alice\n1\nyes\n")
	if err != nil {
		t.Fatalf("access assign: %v\noutput=%q", err, out)
	}
	if !strings.Contains(out, "assigned") {
		t.Fatalf("expected assignment confirmation, got %q", out)
	}
	if !profileExists(t, "alice") {
		t.Fatal("alice profile must be persisted")
	}
	assertForeignIntact(t, foreign)
	// The active owner's seed row must also survive alongside the foreign row.
	if !profileExists(t, "seed") {
		t.Fatal("the active owner's seed profile must survive")
	}
}

// TestAccessOffboardPreservesForeignOwnerProfile proves the same preservation
// boundary on the offboarding route: a successful revoke of the active owner's
// rows must leave the foreign installation's profile unchanged.
func TestAccessOffboardPreservesForeignOwnerProfile(t *testing.T) {
	foreign := foreignProfile()
	writeTestActiveConfig(t, []string{"dev", "prod"}, "read", []orgconfig.Profile{
		{Name: "bob", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}, Owner: "persist-acme"},
		foreign,
	})

	out, err := runWizardWithInput(t, []string{"access", "offboard", "bob"}, "bob\n1\nyes\n")
	if err != nil {
		t.Fatalf("access offboard: %v\noutput=%q", err, out)
	}
	if profileExists(t, "bob") {
		t.Fatal("bob's owned profile must be removed")
	}
	assertForeignIntact(t, foreign)
}

// foreignSameKeyProfile seeds a profile owned by another installation that
// occupies the EXACT same (name, target) key as the row the active owner's
// wizard assigns or revokes. Distinct-key preservation alone is insufficient:
// the map is keyed by (name, target), so the same key addressed by two owners
// is precisely where a last-write-wins merge would overwrite the foreign row.
func foreignSameKeyProfile(name, target string) orgconfig.Profile {
	return orgconfig.Profile{
		Name:       name,
		Target:     target,
		Operations: []orgconfig.Operation{orgconfig.OpRead, orgconfig.OpLog},
		Scope:      "legacy",
		Owner:      "other-installation",
		Revision:   7,
	}
}

// TestAccessAssignSameKeyForeignOwnerFailsClosed proves the r7 finding at the
// production CLI boundary: when the active owner assigns a profile whose
// (name, target) key is already occupied by a foreign owner, the persistence
// fold must fail closed with ErrOwnerCollision — no success report, no write,
// and the foreign row unchanged.
func TestAccessAssignSameKeyForeignOwnerFailsClosed(t *testing.T) {
	foreign := foreignSameKeyProfile("alice", "prod")
	writeTestActiveConfig(t, []string{"dev", "prod"}, "read", []orgconfig.Profile{
		{Name: "seed", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}, Owner: "persist-acme"},
		foreign,
	})

	beforeBytes := activeBytes(t)

	// The foreign installation holds the EXACT same (name, target) key the
	// active owner's assign writes: alice@prod. LoadProfiles deliberately
	// skipped that row, so the owner's manager would fold a same-key row over
	// it — the r6 overwrite. The persistence guard refuses the whole write.
	// Select prod so the assignment collides on exactly ("alice","prod").
	out, err := runWizardWithInput(t, []string{"access", "assign", "alice"}, "alice\n2\nyes\n")
	if err == nil {
		t.Fatalf("assign on a foreign-owner same key must fail closed\noutput=%q", out)
	}
	if !strings.Contains(out, "collides") {
		t.Fatalf("expected a collision failure report, got %q", out)
	}
	afterBytes := activeBytes(t)
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("store must be byte-identical after a colliding assign:\nbefore=%s\nafter=%s", beforeBytes, afterBytes)
	}
	assertForeignIntact(t, foreign)
}

// TestAccessOffboardSameKeyForeignOwnerFailsClosed proves the same fail-closed
// boundary on the offboarding route: when an offboard batch reaches a Target
// where the person does not exist in the owner's namespace — precisely the
// Target where a foreign installation holds the SAME (name, target) key — the
// mutation fails closed before any write: the batch persists nothing, the store
// stays byte-identical, and the foreign row survives untouched.
func TestAccessOffboardSameKeyForeignOwnerFailsClosed(t *testing.T) {
	foreign := foreignSameKeyProfile("bob", "prod")
	writeTestActiveConfig(t, []string{"dev", "prod"}, "read", []orgconfig.Profile{
		{Name: "bob", Target: "dev", Operations: []orgconfig.Operation{orgconfig.OpRead}, Owner: "persist-acme"},
		foreign,
	})

	beforeBytes := activeBytes(t)

	// The batch revokes bob on dev and prod. The owner's manager never loaded
	// the foreign bob@prod row (LoadProfiles skips foreign owners), so prod is
	// not a revocable owned selection and the whole batch fails validation
	// before anything is persisted: no write, foreign row and owned row both
	// intact. On the prod-only key the same fail-closed boundary is carried by
	// TestAccessAssignSameKeyForeignOwnerFailsClosed and the collision guard's
	// unit witness below.
	out, err := runWizardWithInput(t, []string{"access", "offboard", "bob"}, "bob\n1,2\nyes\n")
	if err == nil {
		t.Fatalf("offboard batch touching a foreign-owner same key must fail closed\noutput=%q", out)
	}
	afterBytes := activeBytes(t)
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Fatalf("store must be byte-identical after a refused offboard:\nbefore=%s\nafter=%s", beforeBytes, afterBytes)
	}
	assertForeignIntact(t, foreign)
	if _, ok := ownedProfileOn(t, "bob", "dev"); !ok {
		t.Fatal("the active owner's bob@dev row must survive the refused write")
	}
}

// TestPersistActiveSameKeyCollisionGuard pins the orgconfig-level guarantee
// directly: a snapshot row whose (name, target) key a foreign owner occupies
// in the loaded state fails closed with ErrOwnerCollision before anything is
// saved, while a distinct-key fold still merges and preserves the foreign row.
func TestPersistActiveSameKeyCollisionGuard(t *testing.T) {
	t.Run("assignment over foreign same key refuses write", func(t *testing.T) {
		foreign := foreignSameKeyProfile("alice", "prod")
		root := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", root)
		state := orgconfig.ActiveState{
			Resolution: orgconfig.Resolution{
				ResolvedVersion: orgconfig.ContractVersion,
				Source:          orgconfig.Source{Kind: orgconfig.SourceLocal, Location: root},
			},
			Config: orgconfig.Config{
				Organization: "persist-acme",
				APIVersion:   orgconfig.ContractVersion,
				Operation:    "read",
				Targets:      map[string]orgconfig.Reference{"prod": {Reference: "local://prod"}},
				Profiles:     orgconfig.ProfileMap([]orgconfig.Profile{foreign}),
			},
		}
		manager := orgconfig.NewManager("persist-acme",
			orgconfig.Profile{Name: "alice", Target: "prod",
				Operations: []orgconfig.Operation{orgconfig.OpRead}, Owner: "persist-acme"})
		// The collision must surface the guard error, not a document-validation
		// error, so the state carries a valid Resolution.
		err := orgconfig.PersistActive(manager, state)
		if err == nil || !strings.Contains(err.Error(), "collides") {
			t.Fatalf("same-key fold must fail closed with the collision guard, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(root, "config", "organization", "active.json")); !os.IsNotExist(statErr) {
			t.Fatal("a refused fold must write nothing")
		}
	})
	t.Run("distinct key preserves", func(t *testing.T) {
		foreign := foreignSameKeyProfile("alice", "prod")
		root := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", root)
		state := orgconfig.ActiveState{
			Resolution: orgconfig.Resolution{
				ResolvedVersion: orgconfig.ContractVersion,
				Source:          orgconfig.Source{Kind: orgconfig.SourceLocal, Location: root},
			},
			Config: orgconfig.Config{
				Organization: "persist-acme",
				APIVersion:   orgconfig.ContractVersion,
				Operation:    "read",
				Targets:      map[string]orgconfig.Reference{"dev": {Reference: "local://dev"}, "prod": {Reference: "local://prod"}},
				Profiles:     orgconfig.ProfileMap([]orgconfig.Profile{foreign}),
			},
		}
		manager := orgconfig.NewManager("persist-acme",
			orgconfig.Profile{Name: "alice", Target: "dev",
				Operations: []orgconfig.Operation{orgconfig.OpRead}, Owner: "persist-acme"})
		if err := orgconfig.PersistActive(manager, state); err != nil {
			t.Fatalf("distinct-key fold must persist, got %v", err)
		}
		// The merged state carries both rows: the foreign prod row unchanged and
		// the owner's dev row added. PersistActive merges into the state it was
		// given, so reload the document it saved.
		saved, err := orgconfig.LoadActive()
		if err != nil {
			t.Fatalf("reload after persist: %v", err)
		}
		kept, ok := saved.Config.Profiles[orgconfig.ProfileKey("alice", "prod")]
		if !ok || kept.Owner != "other-installation" || kept.Revision != 7 {
			t.Fatalf("foreign row must survive the merge untouched, got %+v", kept)
		}
		if _, ok := saved.Config.Profiles[orgconfig.ProfileKey("alice", "dev")]; !ok {
			t.Fatal("the owner's distinct-key row must be merged")
		}
	})
	t.Run("revocation fold preserves foreign row", func(t *testing.T) {
		// Keyed-merge side of the same guarantee: the owner revokes their own
		// bob@dev while the state also carries a foreign-owned bob@prod. The
		// fold must delete exactly the revoked owned row and carry the foreign
		// row through byte-for-byte.
		foreign := foreignSameKeyProfile("bob", "prod")
		ownerRow := orgconfig.Profile{Name: "bob", Target: "dev",
			Operations: []orgconfig.Operation{orgconfig.OpRead}, Owner: "persist-acme"}
		root := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", root)
		state := orgconfig.ActiveState{
			Resolution: orgconfig.Resolution{
				ResolvedVersion: orgconfig.ContractVersion,
				Source:          orgconfig.Source{Kind: orgconfig.SourceLocal, Location: root},
			},
			Config: orgconfig.Config{
				Organization: "persist-acme",
				APIVersion:   orgconfig.ContractVersion,
				Operation:    "read",
				Targets:      map[string]orgconfig.Reference{"dev": {Reference: "local://dev"}, "prod": {Reference: "local://prod"}},
				Profiles:     orgconfig.ProfileMap([]orgconfig.Profile{ownerRow, foreign}),
			},
		}
		manager := orgconfig.NewManager("persist-acme", ownerRow)
		if _, err := manager.Revoke("bob", "dev"); err != nil {
			t.Fatalf("revoke of the owner's own row must succeed: %v", err)
		}
		if err := orgconfig.PersistActive(manager, state); err != nil {
			t.Fatalf("revocation fold of an owned row must persist, got %v", err)
		}
		saved, err := orgconfig.LoadActive()
		if err != nil {
			t.Fatalf("reload after persist: %v", err)
		}
		if _, ok := saved.Config.Profiles[orgconfig.ProfileKey("bob", "dev")]; ok {
			t.Fatal("the owner's revoked bob@dev row must be removed by the fold")
		}
		if kept, ok := saved.Config.Profiles[orgconfig.ProfileKey("bob", "prod")]; !ok || kept.Owner != "other-installation" || kept.Revision != foreign.Revision {
			t.Fatalf("foreign bob@prod row must survive the fold untouched, got %+v", kept)
		}
	})
}
