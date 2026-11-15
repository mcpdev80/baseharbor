package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

// TestStateDirIsolatesTargetSelection is the BF-01 (h-d28) regression: the
// target-selection state a command consults must move with
// BASEHARBOR_STATE_DIR, so switching state roots can never resolve — or act
// on — a selection or registration of a different installation. The contract
// (docs/explanation/targets.md): the override takes precedence over XDG data
// and configuration paths, so an isolated installation cannot discover or
// rewrite the normal user's Target selection.
//
// The h-d28 hunt observed the opposite: with BASEHARBOR_STATE_DIR set, a
// command still consulted organization selection state that did not live
// below the state root, so an "isolated" root inherited a foreign activation
// and listed registrations of another installation's Target.
func TestStateDirIsolatesTargetSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	// Installation A: one registration plus the implicit `local` fallback,
	// so an unattributed organization default cannot hide behind the
	// single-registration shortcut, activated below its own state root.
	rootA := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", rootA)
	writeTestTargetRegistrations(t, "alpha", "spare-a")
	if err := writePersistedTarget("alpha"); err != nil {
		t.Fatalf("writePersistedTarget under A: %v", err)
	}
	selectionA := filepath.Join(rootA, "config", "active-target")
	if _, err := os.Stat(selectionA); err != nil {
		t.Fatalf("A's activation must persist below A: %v", err)
	}

	// Installation B: a dedicated state root with its own registrations and
	// no activation.
	rootB := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", rootB)
	writeTestTargetRegistrations(t, "beta", "spare-b")

	// Contract part 1 — with a state root the persisted selection lives
	// strictly below that root.
	if path, err := targetSelectionPath(); err != nil {
		t.Fatalf("targetSelectionPath under a state root: %v", err)
	} else if want := filepath.Join(rootB, "config", "active-target"); path != want {
		t.Fatalf("state-root selection path %q, want %q", path, want)
	}
	// Contract part 2 — the organization state of installation A, recorded
	// the way an ambient call writes it (source location outside every
	// dedicated root), must be invisible below B: the state-root loader
	// resolves organization state below the selected root only.
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	writeTestOrganizationSelection(t, "", "alpha")
	if _, ok, err := orgconfig.LoadActiveOptional(); err != nil || !ok {
		t.Fatalf("the root-less lookup must find the ambient organization state: found=%v (%v)", ok, err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", rootB)
	if path, err := orgconfig.ActivePath(); err != nil {
		t.Fatalf("ActivePath under B: %v", err)
	} else if !strings.HasPrefix(path, rootB+string(os.PathSeparator)) {
		t.Fatalf("organization state path %q does not follow the state root %q", path, rootB)
	}

	// Contract part 3 — a dedicated state root owns the selection: with no
	// B-local activation the resolver answers from B's own registrations
	// only. Installation A's ambient organization default must not resolve
	// (or act on) A's registrations from below B, and it must not even be
	// named in the failure. On the unfixed code the ambient state
	// answered here and the switch read A's registrations.
	if target, err := effectiveTarget(t.Context()); err != nil {
		if containsAllForTest(err.Error(), "alpha") {
			t.Fatalf("resolver reported installation A's target under B: %v", err)
		}
		if !containsAllForTest(err.Error(), "beta", "spare-b") {
			t.Fatalf("resolver did not fail closed on B's own registrations: %v", err)
		}
	} else if target.Name == "alpha" || target.Name == "local" {
		t.Fatalf("A's ambient organization default resolved against B's state as %q; want fail closed", target.Name)
	}

	// Contract part 4 — the isolation holds for the organization consult
	// itself: below a dedicated root, organization state with a source
	// outside that root (A's ambient state) must not answer.
	if _, ok, err := orgconfig.LoadActiveOptional(); err != nil || ok {
		t.Fatalf("installation A's organization state answered under B: found=%v (%v)", ok, err)
	}

	// Contract part 5 — the root-local organization state completes the
	// root-local selection chain: B's own organization default answers
	// under B — declared, as a real root-local organization state is, and
	// naming B's registration — where the root holds no activation. An
	// ambient default never wins here; with an activation present, the
	// activation stays authoritative.
	if err := clearPersistedTarget(); err != nil {
		t.Fatalf("clearPersistedTarget under B: %v", err)
	}
	writeTestOrganizationSelection(t, rootB, "beta")
	target, err := effectiveTarget(t.Context())
	if err != nil {
		t.Fatalf("effectiveTarget with the root-local organization selection: %v", err)
	}
	if target.Name != "beta" {
		t.Fatalf("effective target under B is %q, want the root-local organization selection beta", target.Name)
	}
	if err := writePersistedTarget("beta"); err != nil {
		t.Fatalf("writePersistedTarget under B: %v", err)
	}

	// Contract part 6 — a selection naming a registration that does not
	// exist below the current root must fail closed, never resolve
	// ambiently.
	if err := clearPersistedTarget(); err != nil {
		t.Fatalf("clearPersistedTarget under B: %v", err)
	}
	if err := writePersistedTarget("alpha"); err != nil {
		t.Fatalf("writePersistedTarget(alpha) under B: %v", err)
	}
	if _, err := effectiveTarget(t.Context()); err == nil {
		t.Fatal("installation A's activated target resolved against B's state; want fail closed")
	}

	// Contract part 7 — activation state moves with the root: activating
	// inside B persists below B only, and A's activation stays exactly as
	// A wrote it.
	if err := clearPersistedTarget(); err != nil {
		t.Fatalf("clearPersistedTarget under B: %v", err)
	}
	if err := writePersistedTarget("beta"); err != nil {
		t.Fatalf("writePersistedTarget(beta) under B: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "config", "active-target")); err != nil {
		t.Fatalf("B's activation did not persist below B: %v", err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	if selected, err := readPersistedTarget(); err != nil || selected != "" {
		t.Fatalf("B's activation leaked into the root-less user state: %q (%v)", selected, err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", rootA)
	if selected, err := readPersistedTarget(); err != nil || selected != "alpha" {
		t.Fatalf("B's activation changed A's selection to %q (%v)", selected, err)
	}
}

// writeTestTargetRegistrations stores registrations for the named targets
// below the currently active state root.
func writeTestTargetRegistrations(t *testing.T, names ...string) {
	t.Helper()
	cfg := deployment.Config{
		Version: deployment.ConfigVersion,
		Access: map[string]deployment.AccessDefinition{
			"local-docker": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{},
	}
	for _, name := range names {
		cfg.Targets[name] = deployment.TargetDefinition{
			Runtime: deployment.RuntimeDefinition{Provider: "docker"},
			Access:  deployment.TargetAccess{Reference: "local-docker"},
			Scope:   "local",
		}
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("saving registrations %v: %v", names, err)
	}
}

func containsAllForTest(hay string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(hay, needle) {
			return false
		}
	}
	return true
}

// writeTestOrganizationSelection persists an organization default target at
// the organization state location: below <root>/config/organization for a
// state root, below the ambient user configuration for root "".
func writeTestOrganizationSelection(t *testing.T, root, targetName string, declared ...string) {
	t.Helper()
	// Persist the organization state of the installation rooted at `root`
	// (the empty root stands for the ambient user installation) directly at
	// that installation's organization state location, independently of
	// what the environment currently selects — exactly where the CLI
	// records it.
	dir := organizationStateSourceRoot(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	refs := map[string]orgconfig.Reference{}
	for _, name := range append([]string{targetName}, declared...) {
		refs[name] = orgconfig.Reference{Reference: "local://" + name}
	}
	state := orgconfig.ActiveState{
		Resolution: orgconfig.Resolution{ResolvedVersion: orgconfig.ContractVersion,
			Source: orgconfig.Source{Kind: orgconfig.SourceLocal, Location: dir}},
		Config: orgconfig.Config{
			APIVersion:   orgconfig.ContractVersion,
			Organization: "bf01-test",
			Defaults:     orgconfig.EnvironmentDefaults{Target: targetName},
			Targets:      refs,
		},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("saving organization selection below %q: %v", root, err)
	}
}

// organizationStateSourceRoot names the state location an organization
// selection refers to: the installation root whose state it is (the empty
// root stands for the ambient user installation).
func organizationStateSourceRoot(root string) string {
	if root != "" {
		return filepath.Join(root, "config", "organization")
	}
	return ambientUserOrganizationRoot()
}

// ambientUserOrganizationRoot is the organization state location of the
// root-less user installation.
func ambientUserOrganizationRoot() string {
	if root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); root != "" {
		return filepath.Join(root, "baseharbor", "organization")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "baseharbor", "organization")
}
