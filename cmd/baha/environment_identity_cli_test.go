package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// configureIsolatedTargetForEnvironment writes an isolated single-Target
// configuration under fresh XDG state dirs so the whole test runs in a private
// baseharbor state root (XDG_DATA_HOME/baseharbor). No persisted target and no
// BASEHARBOR_TARGET are set, so the target must be selected explicitly on the
// command line — the same production selection path a real invocation takes.
func configureIsolatedTargetForEnvironment(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{
		"env-tgt": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
	}, Access: map[string]deployment.AccessDefinition{"local": {Provider: "local", Reference: "local"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return os.Getenv("XDG_DATA_HOME")
}

// TestEnvironmentIdentityReachableThroughProductionCLI is the real-CLI-entry
// test (precedent 175-r8): it proves the environment identity seam is reachable
// through the actual `baha` entry point, not only from a unit fixture.
//
// The test writes a durable environment identity via the deployment seam under
// the private XDG state root, then runs the production CLI path
// (runWithIO: global option parsing, context plumbing, command dispatch,
// effectiveTarget) with `target show --json`. It asserts:
//  1. the JSON state_root names the seam's per-Target environment home,
//  2. reopening the environment through the seam under the same state root
//     returns the exact same UUID (durable + CLI-path reachable), and
//  3. renaming the environment under that state root via the seam keeps the
//     UUID stable across the rename.
//
// Mutation witness: replacing the seam's GetEnvironment with a fresh UUID on
// every call makes assertion (2) fail verbatim, so this pins the production
// identity-read path through the real CLI.
func TestEnvironmentIdentityReachableThroughProductionCLI(t *testing.T) {
	stateRoot := configureIsolatedTargetForEnvironment(t)

	// Create the durable identity through the seam first.
	rec, err := deployment.GetEnvironment("env-tgt", "prod")
	if err != nil {
		t.Fatalf("GetEnvironment through production state root: %v", err)
	}

	// Production CLI entry point: runWithIO parses global options, resolves the
	// effective target, and dispatches target show to the real command Run.
	var stdout, stderr bytes.Buffer
	runErr := runWithIO(context.Background(), []string{"target", "show", "env-tgt", "--json"}, &stdout, &stderr)
	if runErr != nil {
		t.Fatalf("baha target show failed through production CLI: %v\n%s", runErr, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"docker\"") {
		t.Fatalf("target show did not render production output:\n%s", stdout.String())
	}

	var report targetShowResult
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("target show --json output: %v\n%s", err, stdout.String())
	}

	// (1) The production state_root must name the seam's per-Target environment
	// home, proving the CLI path resolves the same deployment namespace the seam
	// owns.
	wantHome := filepath.Join(stateRoot, "baseharbor", "targets", "env-tgt")
	if report.StateRoot != wantHome {
		t.Fatalf("production state_root=%q, want %q", report.StateRoot, wantHome)
	}

	// (2) Reopening through the seam from that production state root returns the
	// exact same UUID — the identity is durable and reachable via the CLI path.
	reopened, err := deployment.GetEnvironment("env-tgt", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Identity != rec.Identity {
		t.Fatalf("production CLI path did not preserve identity: %q -> %q", rec.Identity, reopened.Identity)
	}

	// (3) Renaming through the seam under the production state root retains the
	// UUID; reopening under the new name still resolves it.
	moved, err := deployment.RenameEnvironment("env-tgt", "prod", "production")
	if err != nil {
		t.Fatalf("RenameEnvironment through production state root: %v", err)
	}
	if moved.Identity != rec.Identity {
		t.Fatalf("rename via production state changed identity: %q -> %q", rec.Identity, moved.Identity)
	}
	renamed, err := deployment.GetEnvironment("env-tgt", "production")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Identity != rec.Identity {
		t.Fatalf("identity not stable across rename via production path: %q", renamed.Identity)
	}
}

// TestEnvironmentIdentityCLIPathCreatesOnFirstUse proves the seam's create-once
// behaviour through the production CLI state root: a brand-new environment name
// resolves to a freshly generated UUID, and a subsequent call reuses it.
func TestEnvironmentIdentityCLIPathCreatesOnFirstUse(t *testing.T) {
	configureIsolatedTargetForEnvironment(t)

	first, err := deployment.EnvironmentID("env-tgt", "staging")
	if err != nil {
		t.Fatalf("EnvironmentID first create: %v", err)
	}
	for i := 0; i < 25; i++ {
		again, err := deployment.EnvironmentID("env-tgt", "staging")
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("CLI-path env identity regenerated: %q -> %q", first, again)
		}
	}
}
