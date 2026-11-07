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
		cfg.Profiles[p.Name] = p
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

func ownedProfile(t *testing.T, name string) (orgconfig.Profile, bool) {
	t.Helper()
	state, err := orgconfig.LoadActive()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := state.Config.Profiles[name]
	return p, ok
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
	if _, ok := ownedProfile(t, "alice"); ok {
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
	if _, ok := ownedProfile(t, "alice"); !ok {
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
	if _, ok := ownedProfile(t, "bob"); ok {
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
	if _, ok := ownedProfile(t, "dave"); ok {
		t.Fatal("cancel must not persist the assignment")
	}
	if _, ok := ownedProfile(t, "carol"); !ok {
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
	if _, ok := ownedProfile(t, "dave"); !ok {
		t.Fatal("dave profile must be persisted on the selected target")
	}
	if got := ownedProfileCount(t); got != 2 {
		t.Fatalf("expected 2 profiles (seed+dave), got %d", got)
	}
}
