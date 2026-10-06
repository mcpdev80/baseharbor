package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetSharedBackendDestroyRequiresReleasedConsumersAndCanonicalOwnership(t *testing.T) {
	data := t.TempDir()
	shared := SharedBackendFilesAt(data, "target-a", "dev")
	if err := os.MkdirAll(shared.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "dev", PostgresAdminCredential: "credentials/admin", PostgresMembers: 1, Applications: map[string]sharedBackendAppState{"demo/dev": {Application: "demo", Environment: "dev", SQL: map[string]sharedPostgresResource{"default": {Database: "owned"}}}}}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		t.Fatal(err)
	}
	if _, err := SharedBackendDestroyPlan(data, "target-a"); err == nil || !strings.Contains(err.Error(), "application-owned") {
		t.Fatal("active consumer allowed target teardown")
	}
	delete(state.Applications, "demo/dev")
	if err := writeSharedBackendState(shared.State, state); err != nil {
		t.Fatal(err)
	}
	plan, err := SharedBackendDestroyPlan(data, "target-a")
	if err != nil || len(plan) != 1 || plan[0].Project != shared.Project {
		t.Fatalf("released provider missing from target plan: %#v %v", plan, err)
	}
	if err := os.Remove(shared.State); err != nil {
		t.Fatal(err)
	}
	if _, err := SharedBackendDestroyPlan(data, "target-a"); err == nil {
		t.Fatal("missing protected state treated as no consumers")
	}
	foreignState := filepath.Join(t.TempDir(), "state.json")
	if err := writeSharedBackendState(foreignState, state); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignState, shared.State); err != nil {
		t.Fatal(err)
	}
	if _, err := SharedBackendDestroyPlan(data, "target-a"); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatal("symlinked ownership state accepted")
	}
	if err := os.Remove(shared.State); err != nil {
		t.Fatal(err)
	}
	if err := writeSharedBackendState(shared.State, state); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "foreign"), filepath.Join(filepath.Dir(shared.Dir), "foreign")); err != nil {
		t.Fatal(err)
	}
	if _, err := SharedBackendDestroyPlan(data, "target-a"); err == nil {
		t.Fatal("symlink accepted for teardown")
	}
}

func TestSharedBackendDestroyRefusesUnprotectedOrReplacedOwnershipState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "dev"}
	if err := writeSharedBackendState(path, state); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSharedBackendDestroyState(path, expected); err == nil {
		t.Fatal("public ownership state accepted")
	}
	replacement := filepath.Join(filepath.Dir(path), "replacement.json")
	if err := writeSharedBackendState(replacement, state); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, err := readSharedBackendDestroyState(path, expected); err == nil {
		t.Fatal("replaced ownership file accepted")
	}
}
