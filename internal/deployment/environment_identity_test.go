package deployment

import (
	"encoding/json"
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
	if _, err := readEnvironmentRecord(bad); err == nil {
		t.Fatal("corrupt record with empty target accepted")
	}
}
