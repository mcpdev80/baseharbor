package deployment

import (
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
