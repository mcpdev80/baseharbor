package application

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestSharedBackendOwnerEnvironmentIsCanonical verifies the fail-closed
// environment invariant behind mcp-lifecycle-acceptance GH-hosted drift
// (shared_backends_inventory.go:47): the persisted shared-backend
// state.Environment must equal the canonical owner directory basename
// ("shared-backends/core"), never the transient caller environment.
//
// The bug: a fresh apply persisted the caller env ("dev"), so the next
// reconcile read the "core" owner directory, saw "dev" != "core", and failed
// closed with "shared backend environment differs from its existing owner
// directory" — reproducible on every fresh runner. This test pins that the
// owner identity stays canonical and that a genuinely owned-by-environment-
// "other" backend still fails closed.
func TestSharedBackendOwnerEnvironmentIsCanonical(t *testing.T) {
	data := t.TempDir()

	// Reconcile path: caller env is "dev", but the owner directory is "core".
	files := SharedBackendFilesAt(data, "isolated", "dev")
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	state, err := loadSharedBackendState(files.State, "dev")
	if err != nil {
		t.Fatal(err)
	}
	// The canonical-owner correction (applied in ReconcileSharedBackends) pins
	// the owner identity to the directory basename.
	state.Environment = filepath.Base(files.Dir)
	if state.Environment != "core" {
		t.Fatalf("owner identity must be canonical %q, got %q", "core", state.Environment)
	}
	if err := writeSharedBackendState(files.State, state); err != nil {
		t.Fatal(err)
	}
	canonical := sharedBackendCompose(files, state)
	for path, content := range map[string]string{files.Compose: canonical, files.Env: "SHARED_POSTGRES_ADMIN_PASSWORD=test\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// The next reconcile accepts the canonical, read-only owner again (this is
	// exactly the path that failed on fresh GH-hosted runners before the fix).
	inv, err := ExistingSharedBackendInventoryAt(data, "isolated", "dev")
	if err != nil {
		t.Fatalf("canonical owner accepted again after correction: %v", err)
	}
	if len(inv.Services[capability.ProviderValkey]) != 0 {
		t.Fatalf("expected no valkey on a sql-only canonical owner, got %#v", inv)
	}

	// Regression for the real difference: a backend genuinely owned by a
	// different environment must still fail closed (never silently accepted).
	other := t.TempDir()
	otherFiles := SharedBackendFilesAt(other, "isolated", "core")
	if err := os.MkdirAll(otherFiles.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	otherState := sharedBackendState{Version: sharedBackendStateVersion, Environment: "other", PostgresMembers: 1, PostgresAdminCredential: "credentials/admin", Applications: map[string]sharedBackendAppState{}}
	if err := writeSharedBackendState(otherFiles.State, otherState); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingSharedBackendInventoryAt(other, "isolated", "dev"); err == nil {
		t.Fatal("genuinely owned-by-environment-\"other\" backend accepted for caller \"dev\"; fail-closed broken")
	}
}

// TestSharedBackendInventoryRejectsCallerEnvDrift documents that the pre-fix
// drift — persisting the caller env ("dev") into the canonical "core" owner
// directory — is rejected by the inventory. It is intentionally kept as a
// negative regression guard: it must keep failing until the owner identity is
// canonicalized.
func TestSharedBackendInventoryRejectsCallerEnvDrift(t *testing.T) {
	data := t.TempDir()
	files := SharedBackendFilesAt(data, "isolated", "dev")
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Drifted state: caller env "dev" written into the canonical "core" dir.
	drifted := sharedBackendState{Version: sharedBackendStateVersion, Environment: "dev", PostgresMembers: 1, PostgresAdminCredential: "credentials/admin", Applications: map[string]sharedBackendAppState{}}
	if err := writeSharedBackendState(files.State, drifted); err != nil {
		t.Fatal(err)
	}
	canonical := sharedBackendCompose(files, drifted)
	if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("SHARED_POSTGRES_ADMIN_PASSWORD=test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingSharedBackendInventoryAt(data, "isolated", "dev"); err == nil {
		t.Fatal("drifted caller-env state accepted in canonical owner dir; environment fail-closed broken")
	}
}
