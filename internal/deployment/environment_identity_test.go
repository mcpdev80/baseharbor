package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/environment"
)

// TestGetEnvironmentGeneratesOnceAndReuses proves the durable-identity
// guarantee: the first call allocates an ID, and every later call — including
// a brownfield reopen — returns that exact same ID. Renames and re-openings
// never recreate the identity.
func TestGetEnvironmentGeneratesOnceAndReuses(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	rec, err := GetEnvironment("target-a", "prod")
	if err != nil {
		t.Fatalf("first GetEnvironment: %v", err)
	}
	if err := environment.Validate(rec.Identity); err != nil {
		t.Fatalf("generated identity invalid: %v", err)
	}

	for i := 0; i < 50; i++ {
		again, err := GetEnvironment("target-a", "prod")
		if err != nil {
			t.Fatal(err)
		}
		if again.Identity != rec.Identity {
			t.Fatalf("identity changed on reopen: %q -> %q", rec.Identity, again.Identity)
		}
	}
}

// TestGetEnvironmentIsPerTargetIsolation confirms two Targets never share an
// environment identity, even for the same environment name.
func TestGetEnvironmentIsPerTargetIsolation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	dev, err := GetEnvironment("prod", "shared")
	if err != nil {
		t.Fatal(err)
	}
	staging, err := GetEnvironment("staging", "shared")
	if err != nil {
		t.Fatal(err)
	}
	if dev.Identity == staging.Identity {
		t.Fatalf("environment identity shared across targets: %q", dev.Identity)
	}
}

// TestGetEnvironmentReusesOnBrownfield proves that a second process (simulated
// by a fresh baseharbor process root but pre-existing state) reuses the exact
// ID written by the first, never regenerating it.
func TestGetEnvironmentReusesOnBrownfield(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	first, err := GetEnvironment("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a brand-new checkout/Core that opens the SAME state: the
	// directory already exists, so GetEnvironment must return the recorded ID.
	second, err := GetEnvironment("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if second.Identity != first.Identity {
		t.Fatalf("brownfield reopen regenerated identity: %q -> %q", first.Identity, second.Identity)
	}
}

// TestGetEnvironmentConcurrentRegistrationIsAtomic is the acceptance-critical
// test: two goroutines racing the first registration of the same
// (Target, environment) must both observe the SAME identity. A non-atomic
// implementation would write two different UUIDs and poison the environment
// permanently (state split, precedent 0427).
func TestGetEnvironmentConcurrentRegistrationIsAtomic(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	var wg sync.WaitGroup
	const workers = 32
	ids := make([]string, workers)
	var failed int
	var addMu sync.Mutex
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			rec, err := GetEnvironment("target-a", "hot")
			if err != nil {
				addMu.Lock()
				failed++
				addMu.Unlock()
				t.Errorf("worker %d failed: %v", idx, err)
				return
			}
			addMu.Lock()
			ids[idx] = rec.Identity
			addMu.Unlock()
		}(i)
	}
	wg.Wait()

	if failed != 0 {
		t.Fatalf("no worker should fail when all converge, got %d failures", failed)
	}
	// Exactly one distinct identity: every winner and every observer must
	// agree. Two distinct IDs would poison the environment permanently.
	distinct := map[string]bool{}
	for _, id := range ids {
		distinct[id] = true
	}
	if len(distinct) != 1 {
		keys := make([]string, 0, len(distinct))
		for k := range distinct {
			keys = append(keys, k)
		}
		t.Fatalf("concurrent registration produced divergent identities: %v", keys)
	}
	if err := environment.Validate(ids[0]); err != nil {
		t.Fatalf("converged identity invalid: %v", err)
	}
}

// TestGetEnvironmentStateDirOutsideDeploymentsNamespace confirms the identity
// file lives at targets/<t>/.baseharbor/environments/<env>/, NOT inside the
// deployment-registry namespace. This keeps the shared environment identity
// out of deployment scanning and locking.
func TestGetEnvironmentStateDirOutsideDeploymentsNamespace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	if _, err := GetEnvironment("target-a", "prod"); err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "prod", "environment.json")
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("environment identity not at expected home %q: %v", home, err)
	}
	// The deployment registry namespace must NOT contain the environment file.
	deployNs := filepath.Join(root, "targets", "target-a", "deployments")
	if _, err := os.Stat(deployNs); !os.IsNotExist(err) {
		t.Fatalf("deployment namespace should not exist for env-only state: %v", err)
	}
}

// TestGetEnvironmentRejectsInvalidTargetOrName confirms the seam fails closed
// on invalid input before any directory is created.
func TestGetEnvironmentRejectsInvalidTargetOrName(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	if _, err := GetEnvironment("bad/name", "prod"); err == nil {
		t.Fatal("invalid target accepted")
	}
	if _, err := GetEnvironment("target-a", "bad/name"); err == nil {
		t.Fatal("invalid environment name accepted")
	}
}

func TestEnvironmentPathNamesRejectTraversalBeforeAccess(t *testing.T) {
	t.Run("get rejects existing traversed record", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", root)
		outsidePath := writeTraversedEnvironmentRecord(t, root)
		before, err := os.ReadFile(outsidePath)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := GetEnvironment("target-a", "../outside"); err == nil {
			t.Fatal("GetEnvironment accepted a traversing environment name")
		}
		after, err := os.ReadFile(outsidePath)
		if err != nil {
			t.Fatalf("traversed record was removed: %v", err)
		}
		if string(after) != string(before) {
			t.Fatal("traversed record was modified")
		}
	})

	t.Run("rename rejects traversed old name before reading it", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", root)
		outsidePath := writeTraversedEnvironmentRecord(t, root)
		before, err := os.ReadFile(outsidePath)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := RenameEnvironment("target-a", "../outside", "safe"); err == nil {
			t.Fatal("RenameEnvironment accepted a traversing old name")
		}
		after, err := os.ReadFile(outsidePath)
		if err != nil {
			t.Fatalf("traversed source record was removed: %v", err)
		}
		if string(after) != string(before) {
			t.Fatal("traversed source record was modified")
		}
	})

	t.Run("rename rejects traversed new name before creating it", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("BASEHARBOR_STATE_DIR", root)
		original, err := GetEnvironment("target-a", "prod")
		if err != nil {
			t.Fatal(err)
		}
		outsidePath := filepath.Join(root, "targets", "target-a", ".baseharbor", "outside", environmentRecordFile)
		outsideDir := filepath.Dir(outsidePath)

		if _, err := RenameEnvironment("target-a", "prod", "../outside"); err == nil {
			t.Fatal("RenameEnvironment accepted a traversing new name")
		}
		if _, err := os.Stat(outsideDir); !os.IsNotExist(err) {
			t.Fatalf("traversed destination directory was created: %v", err)
		}
		if _, err := os.Stat(outsidePath); !os.IsNotExist(err) {
			t.Fatalf("traversed destination was created: %v", err)
		}
		after, err := GetEnvironment("target-a", "prod")
		if err != nil {
			t.Fatal(err)
		}
		if after.Identity != original.Identity {
			t.Fatalf("source identity changed after rejected rename: %q != %q", after.Identity, original.Identity)
		}
	})
}

func writeTraversedEnvironmentRecord(t *testing.T, root string) string {
	t.Helper()
	identity, err := environment.NewID()
	if err != nil {
		t.Fatal(err)
	}
	record := EnvironmentRecord{Target: "target-a", Name: "../outside", Identity: identity}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "targets", "target-a", ".baseharbor", "outside", environmentRecordFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestEnvironmentRecordSerializesNameAndIdentitySeparately proves the two core
// contract fields — Name (mutable selector) and Identity (durable UUID) — are
// written to their own distinct json fields. A previous regression wrote the
// generated id into the name field; this test decodes the on-disk record and
// asserts Target, Name, and Identity all carry the correct, distinct values.
func TestEnvironmentRecordSerializesNameAndIdentitySeparately(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	rec, err := GetEnvironment("target-a", "production")
	if err != nil {
		t.Fatal(err)
	}
	// Decode the raw on-disk record to prove the fields are not aliased.
	home := filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "production")
	raw, err := os.ReadFile(filepath.Join(home, "environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded EnvironmentRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode on-disk record: %v", err)
	}
	if decoded.Target != "target-a" {
		t.Fatalf("stored target=%q, want target-a", decoded.Target)
	}
	if decoded.Name != "production" {
		t.Fatalf("stored name=%q, want production (must NOT be the id)", decoded.Name)
	}
	if decoded.Identity == "" {
		t.Fatal("stored identity is empty")
	}
	if decoded.Name == decoded.Identity {
		t.Fatalf("name and identity must be distinct fields, both=%q", decoded.Name)
	}
	if decoded.Identity != rec.Identity {
		t.Fatalf("decoded identity mismatch: %q vs %q", decoded.Identity, rec.Identity)
	}
	if decoded.Name != "production" || decoded.Identity == "production" {
		t.Fatalf("name/identity aliased on disk: %+v", decoded)
	}
}

// TestRenameRetainsIdentityIsThePersistedContract proves the acceptance rule
// "uid bleibt, name ist adresierbar": renaming persists a metadata update that
// keeps the exact UUID, and reopening under the NEW name returns the same ID.
// The on-disk record now carries the NEW name, not the old one.
func TestRenameRetainsIdentityIsThePersistedContract(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	orig, err := GetEnvironment("target-a", "prod")
	if err != nil {
		t.Fatal(err)
	}

	moved, err := RenameEnvironment("target-a", "prod", "production")
	if err != nil {
		t.Fatalf("RenameEnvironment: %v", err)
	}
	if moved.Identity != orig.Identity {
		t.Fatalf("rename changed the uid: %q -> %q", orig.Identity, moved.Identity)
	}
	if moved.Name != "production" {
		t.Fatalf("new name not applied: %q", moved.Name)
	}

	// Reopen under the new name: same UUID (stable across rename).
	reopened, err := GetEnvironment("target-a", "production")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Identity != orig.Identity {
		t.Fatalf("identity not stable across rename: %q -> %q", orig.Identity, reopened.Identity)
	}

	// The old identity record is gone; the identity lives at exactly one path.
	oldRecord := filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "prod", "environment.json")
	if _, statErr := os.Stat(oldRecord); !os.IsNotExist(statErr) {
		t.Fatalf("old env record still exists after rename (identity duplicated): %v", statErr)
	}
	newPath := filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "production")
	if _, statErr := os.Stat(newPath); statErr != nil {
		t.Fatalf("new env path missing after rename: %v", statErr)
	}

	// The on-disk record now names the new environment, not the old one.
	raw, err := os.ReadFile(filepath.Join(newPath, "environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded EnvironmentRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode after rename: %v", err)
	}
	if decoded.Name != "production" {
		t.Fatalf("renamed record still carries old name %q", decoded.Name)
	}
	if decoded.Identity != orig.Identity {
		t.Fatalf("renamed record changed identity: %q", decoded.Identity)
	}
}

func TestRenamePreservesEnvironmentAssociations(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	const target = "target-a"
	const oldName = "prod"
	const newName = "production"
	const appID = "app-rename-preserve"
	if _, err := RegisterEnvironmentAssociation(target, oldName, AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}

	moved, err := RenameEnvironment(target, oldName, newName)
	if err != nil {
		t.Fatalf("RenameEnvironment: %v", err)
	}
	if len(moved.Associations) != 1 || moved.Associations[0] != (AssociationBinding{SubjectKind: AssociationKindApp, SubjectID: appID}) {
		t.Fatalf("rename result lost environment associations: %+v", moved.Associations)
	}
	associations, err := ListEnvironmentAssociations(target, newName)
	if err != nil {
		t.Fatal(err)
	}
	if len(associations) != 1 || associations[0].SubjectID != appID {
		t.Fatalf("renamed environment associations = %+v, want app %q", associations, appID)
	}
}

func TestRenameRetryCleansSourceAfterInterruptedInstall(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	const target = "target-a"
	const oldName = "prod"
	const newName = "production"
	const appID = "app-rename-retry"
	if _, err := RegisterEnvironmentAssociation(target, oldName, AssociationKindApp, appID); err != nil {
		t.Fatal(err)
	}
	source, err := GetEnvironment(target, oldName)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a crash after installing the destination record but before
	// removing the source. The old implementation also omitted Associations
	// from that installed destination, so retry must reconcile both records.
	newHome, newPath, err := environmentStateRoot(target, newName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newHome, 0o700); err != nil {
		t.Fatal(err)
	}
	interrupted := EnvironmentRecord{
		Target:   target,
		Name:     newName,
		Identity: source.Identity,
		Associations: []AssociationBinding{{
			SubjectKind: AssociationKindNode,
			SubjectID:   "node-after-crash",
		}},
	}
	blob, err := json.MarshalIndent(interrupted, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, blob, 0o600); err != nil {
		t.Fatal(err)
	}

	retried, err := RenameEnvironment(target, oldName, newName)
	if err != nil {
		t.Fatalf("retry RenameEnvironment after interrupted install: %v", err)
	}
	if len(retried.Associations) != 2 {
		t.Fatalf("retry lost source associations: %+v", retried.Associations)
	}
	_, oldPath, err := environmentStateRoot(target, oldName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retry left stale source identity at %q: %v", oldPath, err)
	}
	associations, err := ListEnvironmentAssociations(target, newName)
	if err != nil {
		t.Fatal(err)
	}
	if len(associations) != 2 {
		t.Fatalf("retry destination associations = %+v, want source and post-crash memberships", associations)
	}
	got := map[string]AssociationSubjectKind{}
	for _, association := range associations {
		got[association.SubjectID] = association.SubjectKind
	}
	if got[appID] != AssociationKindApp || got["node-after-crash"] != AssociationKindNode {
		t.Fatalf("retry destination memberships lost source or destination data: %+v", associations)
	}
}

// TestRenameReopeningByNameIsIdempotent proves that reopening an already-renamed
// environment returns the same UUID without allocating a new one or duplicating
// state.
func TestRenameReopeningByNameIsIdempotent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	first, _ := GetEnvironment("target-a", "dev")
	if _, err := RenameEnvironment("target-a", "dev", "staging"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		rec, err := GetEnvironment("target-a", "staging")
		if err != nil {
			t.Fatal(err)
		}
		if rec.Identity != first.Identity {
			t.Fatalf("reopen renamed env regenerated id: %q -> %q", first.Identity, rec.Identity)
		}
	}
}

// TestRenameAcrossTargetsKeepsIsolation proves renaming within one target does
// not collide with (and cannot adopt) an identity that already exists on a
// different target.
func TestRenameAcrossTargetsKeepsIsolation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	// Create an identity named "shared" on both targets first.
	targetA, _ := GetEnvironment("target-a", "shared")
	targetB, _ := GetEnvironment("target-b", "shared")
	if targetA.Identity == targetB.Identity {
		t.Fatal("cross-target identity collision at setup")
	}

	// Rename on target-a only; target-b's identity is untouched.
	if _, err := RenameEnvironment("target-a", "shared", "shared-renamed"); err != nil {
		t.Fatal(err)
	}
	stillB, err := GetEnvironment("target-b", "shared")
	if err != nil {
		t.Fatal(err)
	}
	if stillB.Identity != targetB.Identity {
		t.Fatalf("rename on target-a leaked into target-b: %q", stillB.Identity)
	}
	renamedA, err := GetEnvironment("target-a", "shared-renamed")
	if err != nil {
		t.Fatal(err)
	}
	if renamedA.Identity != targetA.Identity {
		t.Fatalf("rename on target-a lost its identity: %q", renamedA.Identity)
	}
}

// TestReadEnvironmentRecordFailsClosedOnCorruptBinding proves a persisted
// record with an invalid/empty binding never surfaces as valid, closing the
// cross-target aliasing path.
func TestReadEnvironmentRecordFailsClosedOnCorruptBinding(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	home := filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "prod")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	// Empty target binding.
	bad := filepath.Join(home, "environment.json")
	if err := os.WriteFile(bad, []byte("{\"target\":\"\",\"name\":\"prod\",\"id\":\"00000000-0000-4000-8000-000000000000\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvironmentRecord(bad, "target-a", "prod"); err == nil {
		t.Fatal("corrupt record with empty target accepted")
	}

	// Valid mismatched binding: a syntactically valid record whose stored
	// (target, name) do not match the path it was read from. This is the exact
	// cross-aliasing path the review flagged — a valid record copied under a
	// different (Target, environment) must fail closed on the bind-check, not
	// return its UUID.
	mismatch := filepath.Join(home, "environment.json")
	if err := os.WriteFile(mismatch, []byte("{\"target\":\"other-target\",\"name\":\"other\",\"id\":\"00000000-0000-4000-8000-000000000000\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readEnvironmentRecord(mismatch, "target-a", "prod")
	if err == nil {
		t.Fatal("record with valid-but-mismatched binding accepted")
	}
	if !errors.Is(err, ErrEnvironmentBindingMismatch) {
		t.Fatalf("expected ErrEnvironmentBindingMismatch, got %v", err)
	}
}

// TestRenameRefusesExistingDestinationUID proves a rename must never overwrite
// the identity of a DIFFERENT environment that already exists at the
// destination name. This is the collision-risk the review flagged: a naive
// os.Rename would clobber the destination record's UUID.
func TestRenameRefusesExistingDestinationUID(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	// Two distinct environments, each with its own UUID.
	first, err := GetEnvironment("target-a", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	second, err := GetEnvironment("target-a", "beta")
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity == second.Identity {
		t.Fatal("setup: two environments share a UUID")
	}

	// Renaming alpha onto beta must refuse (beta already holds a different UID)
	// and must leave beta's record byte-for-byte intact.
	_, err = RenameEnvironment("target-a", "alpha", "beta")
	if err == nil {
		t.Fatal("rename onto a different-UID destination was allowed")
	}

	// beta is unchanged; alpha is unchanged; no overwrite occurred.
	stillBeta, err := GetEnvironment("target-a", "beta")
	if err != nil {
		t.Fatal(err)
	}
	if stillBeta.Identity != second.Identity {
		t.Fatalf("beta identity changed after refused rename: %q -> %q", second.Identity, stillBeta.Identity)
	}
	stillAlpha, err := GetEnvironment("target-a", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if stillAlpha.Identity != first.Identity {
		t.Fatalf("alpha identity changed: %q -> %q", first.Identity, stillAlpha.Identity)
	}
}

// TestRenameConvergesUnderContention proves two concurrent renames of the same
// (target, src)→dst pair converge on a single stable identity: exactly one wins
// the registry lock, the other observes the winner's record and returns the
// same UUID. This is the rename-side analogue of the concurrent-registration
// convergence already proven for first creation.
func TestRenameConvergesUnderContention(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	src, err := GetEnvironment("target-a", "converge-src")
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	ids := make([]string, goroutines)
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			rec, rErr := RenameEnvironment("target-a", "converge-src", "converge-dst")
			ids[i] = rec.Identity
			errs[i] = rErr
		}(i)
	}
	wg.Wait()
	for i := 0; i < goroutines; i++ {
		if errs[i] != nil {
			t.Fatalf("concurrent rename %d failed: %v", i, errs[i])
		}
		if ids[i] != src.Identity {
			t.Fatalf("concurrent rename %d diverged: %q != %q", i, ids[i], src.Identity)
		}
	}
	// Exactly one record remains — at dst, none at the old name.
	if _, err := GetEnvironment("target-a", "converge-dst"); err != nil {
		t.Fatalf("converged dst missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "converge-src", "environment.json")); !os.IsNotExist(err) {
		t.Fatalf("old name not removed after rename: %v", err)
	}
}

// TestLegacyStateReuseViaGetEnvironment proves a pre-existing legacy environment
// record (written in the canonical shape by an earlier checkout) is reused
// unchanged by GetEnvironment — the UID is regenerated neither by a reopen nor
// by a rename. This is the migration/reuse the card requires.
func TestLegacyStateReuseViaGetEnvironment(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	home := filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", "legacy")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyID := "00000000-0000-4000-8000-000000000001"
	legacy := fmt.Sprintf("{\"target\":\"target-a\",\"name\":\"legacy\",\"id\":\"%s\"}\n", legacyID)
	if err := os.WriteFile(filepath.Join(home, "environment.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	rec, err := GetEnvironment("target-a", "legacy")
	if err != nil {
		t.Fatalf("GetEnvironment on legacy state: %v", err)
	}
	if rec.Identity != legacyID {
		t.Fatalf("legacy ID regenerated: got %q want %q", rec.Identity, legacyID)
	}
	// Renaming the reused legacy env keeps the historical UUID.
	moved, err := RenameEnvironment("target-a", "legacy", "legacy-v2")
	if err != nil {
		t.Fatalf("rename legacy: %v", err)
	}
	if moved.Identity != legacyID {
		t.Fatalf("rename changed legacy identity: %q -> %q", legacyID, moved.Identity)
	}
}

// TestRenameDoesNotClobberConcurrentDestinationCreator is the deterministic
// regression for the review #1 finding. The pre-lock destination guard in
// RenameEnvironment only saw newPath as absent; two concurrent renames of
// DIFFERENT sources can each clear that guard while dst is still absent, then
// one wins the lock, writes its identity into dst, releases the lock, and the
// other wins the lock next. Its source is still present, so it reaches the
// install with os.Rename(tmpName, newPath) and — without the fix — overwrites
// the identity the other rename just wrote (a silent UID overwrite: two writers
// both succeeded, the destination's identity flipped). The fix re-validates dst
// under the lock on the source-present path and refuses rather than replacing a
// foreign identity.
//
// Two pools of renames move two different sources onto the same still-absent
// destination. The destination must settle on exactly ONE identity: every
// successful rename after the first must observe the SAME dst identity that a
// prior successful rename observed. A flip in the observed dst identity between
// two successful renames is the clobber.
func TestRenameDoesNotClobberConcurrentDestinationCreator(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)

	srcA, err := GetEnvironment("target-a", "race-a-src")
	if err != nil {
		t.Fatal(err)
	}
	srcB, err := GetEnvironment("target-a", "race-b-src")
	if err != nil {
		t.Fatal(err)
	}
	if srcA.Identity == srcB.Identity {
		t.Fatal("setup: two sources share an identity")
	}
	const dst = "race-dst"

	var wg sync.WaitGroup
	const workers = 16
	// observed[i] is the dst identity the i-th successful rename saw.
	observed := make([]string, workers)
	// A mutex-protected view of the last observed dst identity, to detect a
	// flip between concurrent successful renames (the clobber).
	var mu sync.Mutex
	lastSeen := map[string]bool{}
	successCount := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "race-a-src"
			if i%2 == 1 {
				name = "race-b-src"
			}
			for j := 0; j < 60; j++ {
				rec, rErr := RenameEnvironment("target-a", name, dst)
				if rErr != nil {
					continue
				}
				mu.Lock()
				lastSeen[rec.Identity] = true
				successCount++
				observed[i] = rec.Identity
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	// If the fix works, exactly one identity ever exists at dst: the first
	// writer claims it, every later writer either converges onto it or refuses.
	if len(lastSeen) != 1 {
		keys := make([]string, 0, len(lastSeen))
		for k := range lastSeen {
			keys = append(keys, k)
		}
		t.Fatalf("dst identity flipped across concurrent renames: %v — a rename clobbered another's freshly written identity", keys)
	}
	if successCount == 0 {
		t.Fatal("no rename succeeded; expected at least one winner at dst")
	}

	// Exactly one identity remains at dst, stable and valid, never a source ID.
	final, err := GetEnvironment("target-a", dst)
	if err != nil {
		t.Fatalf("final read of destination: %v", err)
	}
	if err := environment.Validate(final.Identity); err != nil {
		t.Fatalf("destination identity invalid: %v", err)
	}
	if final.Identity == srcA.Identity && final.Identity == srcB.Identity {
		t.Fatal("dst identity cannot equal both sources")
	}
	reopen, err := GetEnvironment("target-a", dst)
	if err != nil {
		t.Fatalf("reopen destination: %v", err)
	}
	if reopen.Identity != final.Identity {
		t.Fatalf("dst identity not stable: %q -> %q", final.Identity, reopen.Identity)
	}
	if _, statErr := os.Stat(filepath.Join(root, "targets", "target-a", ".baseharbor", "environments", dst, "environment.json")); statErr != nil {
		t.Fatalf("destination record missing: %v", statErr)
	}
}
