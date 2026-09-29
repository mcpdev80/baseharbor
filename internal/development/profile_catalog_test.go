package development_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
)

func TestProfileCatalogScopesAndComposition(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", config)

	repoRoot := t.TempDir()
	builtins := map[string]development.StackProfile{
		"go": development.BuiltinProfile(goadapter.AdapterID, "go"),
	}

	user := development.StackProfile{
		APIVersion: development.StackProfileAPIVersion,
		Kind:       development.StackProfileKind,
		Metadata:   development.ProfileMetadata{Name: "team-api"},
		Extends:    []string{"go"},
	}
	if _, err := development.SaveProfile(user, development.ProfileScopeUser, repoRoot); err != nil {
		t.Fatal(err)
	}

	repository := development.StackProfile{
		APIVersion: development.StackProfileAPIVersion,
		Kind:       development.StackProfileKind,
		Metadata:   development.ProfileMetadata{Name: "repo-api"},
		Extends:    []string{"team-api"},
	}
	if _, err := development.SaveProfile(repository, development.ProfileScopeRepository, repoRoot); err != nil {
		t.Fatal(err)
	}

	catalog, err := development.LoadProfileCatalog(repoRoot, builtins)
	if err != nil {
		t.Fatal(err)
	}
	if catalog["go"].Scope != development.ProfileScopeBuiltin {
		t.Fatalf("go scope = %q", catalog["go"].Scope)
	}
	if catalog["team-api"].Scope != development.ProfileScopeUser {
		t.Fatalf("team-api scope = %q", catalog["team-api"].Scope)
	}
	if catalog["repo-api"].Scope != development.ProfileScopeRepository {
		t.Fatalf("repo-api scope = %q", catalog["repo-api"].Scope)
	}

	resolved, err := development.ResolveStackProfile("repo-api", development.ProfileMap(catalog))
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Profile.Components) != 1 || resolved.Profile.Components[0].Adapter != goadapter.AdapterID {
		t.Fatalf("unexpected resolved profile: %#v", resolved.Profile)
	}
}

func TestSaveProfileNeverOverwritesExistingProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	profile := development.BuiltinProfile(goadapter.AdapterID, "custom-go")
	if _, err := development.SaveProfile(profile, development.ProfileScopeUser, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := development.SaveProfile(profile, development.ProfileScopeUser, t.TempDir()); err == nil {
		t.Fatal("second development.SaveProfile unexpectedly overwrote existing profile")
	}
}

func TestRepositoryProfileDoesNotLeakIntoUserScope(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", config)
	repoRoot := t.TempDir()

	profile := development.BuiltinProfile(goadapter.AdapterID, "repo-only")
	path, err := development.SaveProfile(profile, development.ProfileScopeRepository, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != development.RepositoryProfileRoot(repoRoot) {
		t.Fatalf("repository profile path = %s", path)
	}
	userRoot, err := development.UserProfileRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(userRoot, "repo-only.yaml")); !os.IsNotExist(err) {
		t.Fatalf("repository profile leaked into user scope: %v", err)
	}
}

func TestDerivedProfileCanPersistPlacementOnInheritedComponent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	repoRoot := t.TempDir()

	builtins := map[string]development.StackProfile{
		"go": development.BuiltinProfile(goadapter.AdapterID, "go"),
	}
	derived := development.StackProfile{
		APIVersion: development.StackProfileAPIVersion,
		Kind:       development.StackProfileKind,
		Metadata:   development.ProfileMetadata{Name: "team-api"},
		Extends:    []string{"go"},
		Capabilities: []development.CapabilityPreference{
			{Capability: capability.SQL, Components: []string{"app"}},
		},
	}
	if _, err := development.SaveProfile(derived, development.ProfileScopeUser, repoRoot); err != nil {
		t.Fatal(err)
	}
	catalog, err := development.LoadProfileCatalog(repoRoot, builtins)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := development.ResolveStackProfile("team-api", development.ProfileMap(catalog))
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Profile.Components) != 1 || resolved.Profile.Components[0].ID != "app" {
		t.Fatalf("unexpected inherited components: %#v", resolved.Profile.Components)
	}
	if len(resolved.Profile.Capabilities) != 1 || len(resolved.Profile.Capabilities[0].Components) != 1 || resolved.Profile.Capabilities[0].Components[0] != "app" {
		t.Fatalf("unexpected inherited placement: %#v", resolved.Profile.Capabilities)
	}
}

func TestProfileCatalogRejectsCrossScopeNameCollision(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	repoRoot := t.TempDir()
	profile := development.BuiltinProfile(goadapter.AdapterID, "go")
	if _, err := development.SaveProfile(profile, development.ProfileScopeUser, repoRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := development.LoadProfileCatalog(repoRoot, map[string]development.StackProfile{"go": profile}); err == nil {
		t.Fatal("cross-scope profile name collision unexpectedly accepted")
	}
}
