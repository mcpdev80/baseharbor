package main

import (
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

	// Installation A: one registration, activated through the persistable
	// selection, below its own state root.
	rootA := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", rootA)
	writeTestTargetRegistrations(t, "alpha")
	if err := writePersistedTarget("alpha"); err != nil {
		t.Fatalf("writePersistedTarget under A: %v", err)
	}
	selectionA := filepath.Join(rootA, "config", "active-target")
	if _, err := os.Stat(selectionA); err != nil {
		t.Fatalf("A's activation must persist below A: %v", err)
	}
	// Installation A also carries an organization selection naming its own
	// target at the ambient user location: what `baha config organization
	// set` writes when the developer's normal installation activates the
	// organization. This is the state a dedicated state root must never
	// consult.
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	writeTestOrganizationSelection(t, "", "alpha")
	t.Setenv("BASEHARBOR_STATE_DIR", rootA)

	// Installation B: a dedicated state root with two registrations and no
	// activation of its own.
	rootB := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", rootB)
	writeTestTargetRegistrations(t, "beta", "gamma")

	// A root-local organization state for B is what `baha config
	// organization set` leaves below an activated installation: it must not
	// be visible while no state root is selected. The ambient user
	// organization state of installation A (above) is the stale state the
	// hunt left behind; with a dedicated root, organization state resolves
	// exclusively below that root.
	writeTestOrganizationSelection(t, rootB, "beta")

	// Direct proof of the isolation seam: under the state root the
	// organization lookup must resolve B's OWN state location, not the
	// ambient user path. Before the fix the organization selection lookup
	// followed an environment-only chain that let the ambient file answer
	// even while a dedicated root was active — exactly the h-d28 leakage.
	if path, err := orgconfig.ActivePath(); err != nil {
		t.Fatalf("ActivePath under B: %v", err)
	} else if !strings.HasPrefix(path, rootB+string(os.PathSeparator)) {
		t.Fatalf("organization state resolves to %q, outside B's state root %q", path, rootB)
	}

	// B must observe none of A's selection: neither through the persisted
	// file nor through the resolver.
	if selected, err := readPersistedTarget(); err != nil || selected != "" {
		t.Fatalf("persisted selection from installation A leaked into B: %q (%v)", selected, err)
	}
	var err error
	target, err := effectiveTarget(t.Context())
	if err == nil {
		t.Fatalf("a foreign organization default naming a target B never registered resolved under B: %q", target.Name)
	}
	if containsAllForTest(err.Error(), "alpha") {
		t.Fatalf("resolver reported installation A's target in B's failure: %v", err)
	}

	// Contract part 3 — a root-local selection resolves root-local: B's own
	// activation names beta and the resolver answers with B's registration
	// even while the ambient organization state of installation A names
	// alpha. Activating inside B writes below B only; A's selection file
	// stays byte-identical.
	if err := writePersistedTarget("beta"); err != nil {
		t.Fatalf("writePersistedTarget under B: %v", err)
	}
	target, err = effectiveTarget(t.Context())
	if err != nil {
		t.Fatalf("effectiveTarget under B: %v", err)
	}
	if target.Name != "beta" {
		t.Fatalf("effective target under B is %q, want the root-local selection beta", target.Name)
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
func writeTestOrganizationSelection(t *testing.T, root, targetName string) {
	t.Helper()
	previous, hadPrevious := os.LookupEnv("BASEHARBOR_STATE_DIR")
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	defer func() {
		if hadPrevious {
			os.Setenv("BASEHARBOR_STATE_DIR", previous)
		} else {
			os.Unsetenv("BASEHARBOR_STATE_DIR")
		}
	}()
	// Mirror what `baha config organization set --source local` records:
	// the resolution names the state location, which for an organization
	// directory below the active root is the root-local organization state.
	dir := root
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("ambient home: %v", err)
		}
		dir = filepath.Join(home, ".config", "baseharbor", "organization")
	}
	if err := orgconfig.SaveActive(orgconfig.ActiveState{
		Resolution: orgconfig.Resolution{ResolvedVersion: orgconfig.ContractVersion,
			Source: orgconfig.Source{Kind: orgconfig.SourceLocal, Location: dir}},
		Config: orgconfig.Config{
			APIVersion:   orgconfig.ContractVersion,
			Organization: "bf01-test",
			Defaults:     orgconfig.EnvironmentDefaults{Target: targetName},
		},
	}); err != nil {
		t.Fatalf("saving organization selection below %q: %v", root, err)
	}
}
