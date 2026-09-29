package development

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
)

func TestProfileCatalogScopesAndComposition(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", config)

	repoRoot := t.TempDir()
	builtins := map[string]StackProfile{
		"go": BuiltinProfile(goadapter.AdapterID, "go"),
	}

	user := StackProfile{
		APIVersion: StackProfileAPIVersion,
		Kind:       StackProfileKind,
		Metadata:   ProfileMetadata{Name: "team-api"},
		Extends:    []string{"go"},
	}
	if _, err := SaveProfile(user, ProfileScopeUser, repoRoot); err != nil {
		t.Fatal(err)
	}

	repository := StackProfile{
		APIVersion: StackProfileAPIVersion,
		Kind:       StackProfileKind,
		Metadata:   ProfileMetadata{Name: "repo-api"},
		Extends:    []string{"team-api"},
	}
	if _, err := SaveProfile(repository, ProfileScopeRepository, repoRoot); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProfileCatalog(repoRoot, builtins)
	if err != nil {
		t.Fatal(err)
	}
	if catalog["go"].Scope != ProfileScopeBuiltin {
		t.Fatalf("go scope = %q", catalog["go"].Scope)
	}
	if catalog["team-api"].Scope != ProfileScopeUser {
		t.Fatalf("team-api scope = %q", catalog["team-api"].Scope)
	}
	if catalog["repo-api"].Scope != ProfileScopeRepository {
		t.Fatalf("repo-api scope = %q", catalog["repo-api"].Scope)
	}

	resolved, err := ResolveStackProfile("repo-api", ProfileMap(catalog))
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Profile.Components) != 1 || resolved.Profile.Components[0].Adapter != goadapter.AdapterID {
		t.Fatalf("unexpected resolved profile: %#v", resolved.Profile)
	}
}

func TestSaveProfileNeverOverwritesExistingProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	profile := BuiltinProfile(goadapter.AdapterID, "custom-go")
	if _, err := SaveProfile(profile, ProfileScopeUser, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveProfile(profile, ProfileScopeUser, t.TempDir()); err == nil {
		t.Fatal("second SaveProfile unexpectedly overwrote existing profile")
	}
}

func TestRepositoryProfileDoesNotLeakIntoUserScope(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", config)
	repoRoot := t.TempDir()

	profile := BuiltinProfile(goadapter.AdapterID, "repo-only")
	path, err := SaveProfile(profile, ProfileScopeRepository, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != RepositoryProfileRoot(repoRoot) {
		t.Fatalf("repository profile path = %s", path)
	}
	userRoot, err := UserProfileRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(userRoot, "repo-only.yaml")); !os.IsNotExist(err) {
		t.Fatalf("repository profile leaked into user scope: %v", err)
	}
}
