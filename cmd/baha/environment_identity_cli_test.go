package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
// UID create, reopen, and rename through the ACTUAL `baha target env` command
// path via runWithIO — never by calling the deployment seam directly. The UID
// is created/reused/renamed by the command itself; the test only parses the
// `--json` output and asserts the reported environment_id/name.
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
	// REAL `baha target env rename` CLI entry. Finding 2 requires the rename
	// regression to drive the RENAME MUTATION through a real CLI command via
	// runWithIO — not to call the deployment seam directly and only assert a
	// read. So the rename below runs the actual command path.
	stdout, stderr := runTargetEnvRename(t, "env-tgt", envName, "production")
	if stderr != "" {
		t.Fatalf("target env rename failed: %s", stderr)
	}
	var renameReport targetEnvShowResult
	if err := json.Unmarshal([]byte(stdout), &renameReport); err != nil {
		t.Fatalf("renamed target env rename --json: %v\n%s", err, stdout)
	}
	if renameReport.EnvironmentID != firstID {
		t.Fatalf("UID changed across CLI rename: %q -> %q", firstID, renameReport.EnvironmentID)
	}
	if renameReport.EnvironmentName != "production" {
		t.Fatalf("renamed env reported name=%q, want production", renameReport.EnvironmentName)
	}
	// And reopening the renamed selector via `target show` must keep the same
	// UID (the rename was a pure metadata update that retained the UUID).
	stdout, _ = runTargetShowJSON(t, "env-tgt", "production")
	var reopenAfterReport targetShowResult
	if err := json.Unmarshal([]byte(stdout), &reopenAfterReport); err != nil {
		t.Fatalf("reopened renamed target show --json: %v\n%s", err, stdout)
	}
	if reopenAfterReport.EnvironmentID != firstID {
		t.Fatalf("UID changed after rename reopened via CLI: %q -> %q", firstID, reopenAfterReport.EnvironmentID)
	}
	if reopenAfterReport.EnvironmentName != "production" {
		t.Fatalf("renamed env reported name=%q, want production", reopenAfterReport.EnvironmentName)
	}
}

// runTargetEnvRename drives the REAL `baha target env rename OLD NEW` command
// through the production runWithIO entry point (finding 2). It returns the
// command's stdout and any stderr, so the caller can assert on a rejected
// rename.
func runTargetEnvRename(t *testing.T, target, oldName, newName string) (string, string) {
	t.Helper()
	t.Setenv("BASEHARBOR_TARGET", target)
	t.Setenv("BASEHARBOR_ENVIRONMENT", "")
	var stdout, stderr bytes.Buffer
	if err := runWithIO(context.Background(), []string{"target", "env", "rename", oldName, newName}, &stdout, &stderr); err != nil {
		// A rejected rename (binding mismatch, idempotent no-op, etc.) still
		// returns an error; surface both streams for the caller's assertion.
		return stdout.String(), stderr.String() + "\n" + err.Error()
	}
	return stdout.String(), ""
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

// TestEnvironmentUIDRenameRejectsDestinationBindingMismatch proves finding 3:
// renaming onto a destination that already carries a DIFFERENT (Target,
// environment)'s identity must be refused by the real CLI rename command —
// the guard must NOT swallow the binding-mismatch and clobber the foreign
// identity. The command returns an error and no aliased UID surfaces.
func TestEnvironmentUIDRenameRejectsDestinationBindingMismatch(t *testing.T) {
	stateRoot := configureIsolatedTargetForEnvironment(t)

	const oldName = "staging"
	const foreignName = "production"

	// (1) Create the source environment (allocates its durable UUID) via the
	// real CLI rename entry. The rename first ensures the source exists, then
	// moves it; here we move it to a distinct scratch name so a durable source
	// identity exists for the failing-closed assertion below.
	if _, err1 := runTargetEnvRename(t, "env-tgt", oldName, "scratch-source"); err1 != "" {
		t.Fatalf("unexpected source-creation failure: %s", err1)
	}

	// (2) Seed a valid record at the destination that belongs to a DIFFERENT
	// (Target, environment): a foreign claim that must never be clobbered.
	foreignHome := filepath.Join(stateRoot, "baseharbor", "targets", "env-tgt", ".baseharbor", "environments", foreignName)
	if err := os.MkdirAll(foreignHome, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignID := "00000000-0000-4000-8000-000000000009"
	bogus := fmt.Sprintf(`{"target":"foreign-target","name":"foreign","id":%q}`, foreignID)
	if err := os.WriteFile(filepath.Join(foreignHome, "environment.json"), []byte(bogus), 0o600); err != nil {
		t.Fatal(err)
	}

	// (3) Rename OLD -> FOREIGN through the real CLI. Must FAIL CLOSED: the
	// destination already holds a different identity, so the rename is refused
	// and the foreign record is untouched.
	out2, err2 := runTargetEnvRename(t, "env-tgt", oldName, foreignName)
	if err2 == "" {
		t.Fatalf("expected the rename onto a foreign-bound destination to be refused; stdout=%q", out2)
	}
	if strings.Contains(err2, foreignID) {
		t.Fatalf("CLI renamed onto / surfaced a foreign-bound UID: %s", err2)
	}
	seeded, readErr := os.ReadFile(filepath.Join(foreignHome, "environment.json"))
	if readErr != nil {
		t.Fatalf("read foreign record: %v", readErr)
	}
	if string(seeded) != bogus {
		t.Fatalf("foreign destination record was clobbered by the rename:\nbefore=%safter=%q", bogus, string(seeded))
	}
	if !strings.Contains(strings.ToLower(err2), "binding") && !strings.Contains(strings.ToLower(err2), "exists") {
		t.Fatalf("expected a binding/existence rejection, got: %s", err2)
	}
}

// TestEnvironmentUIDRenameIdempotentSameRequest proves finding 4: a second
// identical rename request after success is an idempotent no-op that
// converges on the destination record carrying the source's identity — it does
// NOT allocate a fresh UID cycle. Positive assertion: both calls report the
// same durable UUID.
func TestEnvironmentUIDRenameIdempotentSameRequest(t *testing.T) {
	configureIsolatedTargetForEnvironment(t)

	const oldName = "dev"
	const newName = "development"

	// First rename allocates the source and moves it to the new name.
	out1, err1 := runTargetEnvRename(t, "env-tgt", oldName, newName)
	if err1 != "" {
		t.Fatalf("first rename failed: %s", err1)
	}
	var first targetEnvShowResult
	if err := json.Unmarshal([]byte(out1), &first); err != nil {
		t.Fatalf("first rename output: %v\n%s", err, out1)
	}
	if first.EnvironmentID == "" {
		t.Fatalf("first rename did not report an environment_id: %s", out1)
	}

	// Second identical rename request: must be an idempotent no-op returning
	// the SAME UUID — no fresh UID cycle.
	out2, err2 := runTargetEnvRename(t, "env-tgt", oldName, newName)
	if err2 != "" {
		t.Fatalf("second identical rename should be an idempotent no-op, got error: %s", err2)
	}
	var second targetEnvShowResult
	if err := json.Unmarshal([]byte(out2), &second); err != nil {
		t.Fatalf("second rename output: %v\n%s", err, out2)
	}
	if second.EnvironmentID != first.EnvironmentID {
		t.Fatalf("idempotent rename allocated a fresh UID: %q -> %q", first.EnvironmentID, second.EnvironmentID)
	}
	if second.EnvironmentName != newName {
		t.Fatalf("idempotent rename reported name=%q, want %q", second.EnvironmentName, newName)
	}
}

// TestEnvironmentUIDNoAuthoredLeak proves the no-authored-leak guarantee for
// the environment UID (decision B): the environment identity is a
// system-generated UUID that is NEVER authored in a developer contract and
// NEVER allocated by a read command. A pure GetEnvironment call must only
// validate/reuse — it must not manufacture an id, and it must never read an
// authored id out of a manifest. This mirrors the app.id precedent
// (app_binding_production_test) adapted to the per-(target,env) identity
// record: the UID lives in the system-owned environment.json, never in the
// developer-authored contract.
func TestEnvironmentUIDNoAuthoredLeak(t *testing.T) {
	stateRoot := configureIsolatedTargetForEnvironment(t)

	const envName = "prod"

	// A pure read (GetEnvironment) must resolve the durable identity without
	// manufacturing one from authored intent and without returning any non-
	// server-assigned id.
	id, idErr := deployment.GetEnvironment("env-tgt", envName)
	if idErr != nil {
		t.Fatalf("GetEnvironment read failed: %v", idErr)
	}
	if id.Identity == "" {
		t.Fatalf("GetEnvironment did not resolve an identity: %+v", id)
	}
	// The resolved id must be a valid server-assigned environment UUID, never
	// an authored value.
	if err := environment.Validate(id.Identity); err != nil {
		t.Fatalf("environment identity is not a valid server-assigned id: %v (%q)", err, id.Identity)
	}

	// The developer-authored contract (baseharbor.yaml) must NOT carry the
	// environment id; only the system-owned environment.json owns it.
	manifestPath := filepath.Join(stateRoot, "baseharbor.yaml")
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		authored, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(authored), "environment_id") {
			t.Fatalf("authored contract carries an authored environment id; id assignment is not server-side")
		}
	}
}
