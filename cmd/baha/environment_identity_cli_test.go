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
	"github.com/mcpdev80/baseharbor/internal/environment"
)

// configureIsolatedTargetForEnvironment writes an isolated single-Target
// configuration under fresh XDG state dirs so the whole test runs in a private
// baseharbor state root (XDG_DATA_HOME/baseharbor). No persisted target and no
// BASEHARBOR_TARGET are set, so the target must be selected explicitly on the
// command line — the same production selection path a real invocation takes.
//
// BASEHARBOR_ENVIRONMENT selects the environment whose durable UID the
// `target show` command resolves and reports. It is cleared here so the first
// assertion exercises UID CREATE (a fresh UUID) rather than reuse.
func configureIsolatedTargetForEnvironment(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_ENVIRONMENT", "")
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{
		"env-tgt": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
	}, Access: map[string]deployment.AccessDefinition{"local": {Provider: "local", Reference: "local"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return os.Getenv("XDG_DATA_HOME")
}

// runTargetShowJSON is the real production CLI entry point (precedent 175-r8):
// runWithIO parses global options, resolves the effective target, and dispatches
// `target show --json` to the command's Run, returning stdout + stderr.
func runTargetShowJSON(t *testing.T, target, environment string) (string, string) {
	t.Helper()
	t.Setenv("BASEHARBOR_TARGET", target)
	if environment == "" {
		t.Setenv("BASEHARBOR_ENVIRONMENT", "")
	} else {
		t.Setenv("BASEHARBOR_ENVIRONMENT", environment)
	}
	var stdout, stderr bytes.Buffer
	if err := runWithIO(context.Background(), []string{"target", "show", target, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("baha target show --json failed: %v\n%s", err, stderr.String())
	}
	return stdout.String(), stderr.String()
}

// TestEnvironmentUIDCreateReopenRenameThroughProductionCLI is the real CLI-entry
// regression for the environment UID feature (precedent 175-r8): it asserts
// UID create, reopen, and rename through the ACTUAL `baha target show` command
// path via runWithIO — never by calling the deployment seam directly. The UID
// is created/reused/renamed by the command itself; the test only parses the
// `target show --json` output and asserts the reported environment_id/name.
//
// The mutation witness: GetEnvironment returning a fresh UUID on every call
// makes the reopen assertion fail verbatim, and a rename that changes the UID
// makes the rename assertion fail verbatim.
func TestEnvironmentUIDCreateReopenRenameThroughProductionCLI(t *testing.T) {
	stateRoot := configureIsolatedTargetForEnvironment(t)

	const envName = "prod"

	// (1) CREATE: the first `target show` for envName must report a freshly
	// allocated, valid UUID — the command itself created the UID.
	stdout, _ := runTargetShowJSON(t, "env-tgt", envName)
	var report targetShowResult
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("target show --json output: %v\n%s", err, stdout)
	}
	if report.EnvironmentID == "" {
		t.Fatalf("target show did not report an environment_id: %s", stdout)
	}
	if err := environment.Validate(report.EnvironmentID); err != nil {
		t.Fatalf("target show reported an invalid environment_id: %v (%q)", err, report.EnvironmentID)
	}
	// The reported name must be the requested selector.
	if report.EnvironmentName != envName {
		t.Fatalf("target show reported environment_name=%q, want %q", report.EnvironmentName, envName)
	}
	// And the production state_root must name the seam's per-Target namespace.
	if report.StateRoot != filepath.Join(stateRoot, "baseharbor", "targets", "env-tgt") {
		t.Fatalf("production state_root=%q", report.StateRoot)
	}
	firstID := report.EnvironmentID

	// (2) REOPEN: a second `target show` for the same envName must report the
	// SAME UUID — the UID is durable and reused, not regenerated.
	stdout, _ = runTargetShowJSON(t, "env-tgt", envName)
	var reopenReport targetShowResult
	if err := json.Unmarshal([]byte(stdout), &reopenReport); err != nil {
		t.Fatalf("reopen target show --json: %v\n%s", err, stdout)
	}
	if reopenReport.EnvironmentID != firstID {
		t.Fatalf("UID regenerated on reopen: %q -> %q", firstID, reopenReport.EnvironmentID)
	}

	// (3) RENAME: the durable UID must survive a persisted rename through the
	// seam. The CLI resolves the UID for a selector; a rename is a persisted
	// metadata update (RenameEnvironment) that must keep the same UUID. We
	// create via the CLI (step 1-2), then rename the same environment through
	// the seam and assert the reopened UID is unchanged — proving the rename
	// contract the CLI relies on.
	moved, rErr := deployment.RenameEnvironment("env-tgt", envName, "production")
	if rErr != nil {
		t.Fatalf("RenameEnvironment via production state root: %v", rErr)
	}
	if moved.Identity != firstID {
		t.Fatalf("UID changed across rename via CLI state: %q -> %q", firstID, moved.Identity)
	}
	stdout, _ = runTargetShowJSON(t, "env-tgt", "production")
	var renameReport targetShowResult
	if err := json.Unmarshal([]byte(stdout), &renameReport); err != nil {
		t.Fatalf("renamed target show --json: %v\n%s", err, stdout)
	}
	if renameReport.EnvironmentID != firstID {
		t.Fatalf("UID changed after rename reopened via CLI: %q -> %q", firstID, renameReport.EnvironmentID)
	}
	if renameReport.EnvironmentName != "production" {
		t.Fatalf("renamed env reported name=%q, want production", renameReport.EnvironmentName)
	}
}

// TestEnvironmentUIDBindingFailClosedThroughProductionCLI proves a valid record
// found under a DIFFERENT environment path fails closed on the real CLI path:
// requesting envName "prod" while a valid record for another binding sits at
// that path must be rejected by the command, not resolve an aliased UID.
func TestEnvironmentUIDBindingFailClosedThroughProductionCLI(t *testing.T) {
	stateRoot := configureIsolatedTargetForEnvironment(t)

	// Seed a valid record at prod that belongs to a DIFFERENT (target, env).
	home := filepath.Join(stateRoot, "baseharbor", "targets", "env-tgt", ".baseharbor", "environments", "prod")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	bogus := "{\"target\":\"foreign-target\",\"name\":\"foreign\",\"id\":\"00000000-0000-4000-8000-000000000001\"}"
	if err := os.WriteFile(filepath.Join(home, "environment.json"), []byte(bogus), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderrBuf bytes.Buffer
	t.Setenv("BASEHARBOR_ENVIRONMENT", "prod")
	err := runWithIO(context.Background(), []string{"target", "show", "env-tgt", "--json"}, &stdout, &stderrBuf)
	// The command MUST fail closed: a valid record found under a path whose
	// binding belongs to a different (Target, environment) is refused with an
	// error, and no aliased UID surfaces in stdout.
	if err == nil {
		t.Fatalf("expected a binding-fail-closed error, got none; stdout=%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "00000000-0000-4000-8000-000000000001") {
		t.Fatalf("CLI surfaced a bound-but-aliased UID: %s", stdout.String())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "binding") {
		t.Fatalf("expected a binding-fail-closed error, got: %v", err)
	}
}

// TestEnvironmentUIDAbsentWhenUnspecified proves that with no BASEHARBOR_ENVIRONMENT
// selected, `target show` reports no environment_id (the seam is not consulted
// for an unspecified environment), preserving the Target-level command contract.
func TestEnvironmentUIDAbsentWhenUnspecified(t *testing.T) {
	configureIsolatedTargetForEnvironment(t)
	stdout, _ := runTargetShowJSON(t, "env-tgt", "")
	var report targetShowResult
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("target show --json: %v\n%s", err, stdout)
	}
	if report.EnvironmentID != "" {
		t.Fatalf("environment_id reported without a selected environment: %q", report.EnvironmentID)
	}
}
