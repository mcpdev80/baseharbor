package deployment

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func configMutationFixture(t *testing.T) (Config, string, []byte) {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	cfg := Config{Version: ConfigVersion, Access: map[string]AccessDefinition{"local": {Provider: "local", Reference: "local"}}, Targets: map[string]TargetDefinition{"first": {Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "local"}}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, path, raw
}

func TestSaveExpectedPreservesConcurrentConfiguration(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	other, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	other.Targets["other"] = other.Targets["first"]
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.Targets, "first")
	if err := cfg.SaveExpected(raw); !errors.Is(err, ErrConfigurationChanged) {
		t.Fatalf("stale save: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, current) {
		t.Fatal("concurrent owner overwritten", err)
	}
}

func TestSaveExpectedBindsBytesDespiteRestoredTimestamp(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.ReplaceAll(raw, []byte("first"), []byte("other"))
	if bytes.Equal(raw, changed) {
		t.Fatal("fixture did not change")
	}
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveExpected(raw); !errors.Is(err, ErrConfigurationChanged) {
		t.Fatalf("changed equal-size owner accepted: %v", err)
	}
}

func TestSaveExpectedChangesOnlySelectedConfiguration(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	delete(cfg.Targets, "first")
	if err := cfg.SaveExpected(raw); err != nil {
		t.Fatal(err)
	}
	actual, err := LoadConfig()
	if err != nil || len(actual.Targets) != 0 || len(actual.Access) != 1 {
		t.Fatalf("unexpected configuration: %#v %v", actual, err)
	}
	info, err := os.Lstat(path)
	if err != nil || !bhruntime.ProtectedStateOwner(path, info) {
		t.Fatal("replacement owner unprotected", err)
	}
}

func TestSaveExpectedRejectsLinkedOwners(t *testing.T) {
	for _, link := range []string{"hard", "symbolic"} {
		t.Run(link, func(t *testing.T) {
			cfg, path, raw := configMutationFixture(t)
			foreign := filepath.Join(t.TempDir(), "foreign")
			if link == "hard" {
				if err := os.Link(path, foreign); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(path, foreign); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, path); err != nil {
					t.Fatal(err)
				}
			}
			if err := cfg.SaveExpected(raw); err == nil {
				t.Fatal("linked owner accepted")
			}
			after, err := os.ReadFile(foreign)
			if err != nil || !bytes.Equal(after, raw) {
				t.Fatal("foreign owner changed", err)
			}
		})
	}
}

func TestSaveDoesNotFollowLegacyTemporaryPath(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	foreign := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(foreign, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, path+".tmp"); err != nil {
		t.Fatal(err)
	}
	delete(cfg.Targets, "first")
	if err := cfg.SaveExpected(raw); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(foreign)
	if err != nil || string(after) != "preserve" {
		t.Fatal("legacy temporary path followed", err)
	}
}

func TestSaveExpectedSerializesWithExistingStateLock(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	unlock, err := coreinstallation.AcquireStateFileLock(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveExpected(raw); err == nil {
		unlock()
		t.Fatal("held lock bypassed")
	}
	unlock()
	if err := cfg.SaveExpected(raw); err != nil {
		t.Fatal(err)
	}
}

func TestSaveExpectedDistinguishesEmptyAndAbsentOwner(t *testing.T) {
	cfg, path, _ := configMutationFixture(t)
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveExpected(nil); !errors.Is(err, ErrConfigurationChanged) {
		t.Fatalf("existing empty owner replaced as absent: %v", err)
	}
}

func TestConfigReplaceRevalidatesImmediatelyBeforeRename(t *testing.T) {
	cfg, path, raw := configMutationFixture(t)
	if err := cfg.saveAt(path, func() error { return ErrConfigurationChanged }); !errors.Is(err, ErrConfigurationChanged) {
		t.Fatalf("revalidation failure: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("failed revalidation replaced owner", err)
	}
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if err != nil || len(entries) != 0 {
		t.Fatal("temporary state leaked", entries, err)
	}
}

func TestMutationSnapshotUsesNormalParserWithoutWriting(t *testing.T) {
	_, path, raw := configMutationFixture(t)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, expected, err := LoadConfigForMutation()
	if err != nil || !bytes.Equal(raw, expected) || cfg.Targets["first"].Runtime.Provider != "docker" {
		t.Fatalf("mutation snapshot: %#v %v", cfg, err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || before.ModTime() != after.ModTime() {
		t.Fatal("snapshot mutated configuration", err)
	}
	if err := os.WriteFile(path, []byte("version: 99\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadConfigForMutation(); err == nil {
		t.Fatal("mutation snapshot bypassed normal version validation")
	}
}

func TestMutationSnapshotAbsentOwnerDoesNotInitialize(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	cfg, expected, err := LoadConfigForMutation()
	if err != nil || expected != nil || cfg.Version != ConfigVersion || len(cfg.Targets) != 0 {
		t.Fatalf("absent snapshot: %#v %v", cfg, err)
	}
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("read allocated state: %v", err)
	}
}

// TestSaveStateRouteFirstWriteNotRejectedAsGhost guards the 0428-110 r2 fix: a
// legitimate first write to the BASEHARBOR_STATE_DIR config route must NOT be
// rejected when a sibling <state>/config.yaml sits one level up. That sibling
// is a coincidence on the state route, never an XDG double-append, so the ghost
// guard must be XDG-scoped (configGhostRoot), not a generic grandparent check.
func TestSaveStateRouteFirstWriteNotRejectedAsGhost(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	// Legitimate sibling: <state>/config.yaml present, <state>/config/config.yaml absent.
	if err := os.WriteFile(filepath.Join(state, "config.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Version: ConfigVersion, Access: map[string]AccessDefinition{"local": {Provider: "local", Reference: "local"}}, Targets: map[string]TargetDefinition{"first": {Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "local"}}}}
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	// Absent owner: LoadConfigForMutation must return a clean nil snapshot.
	if _, expected, err := LoadConfigForMutation(); err != nil || expected != nil {
		t.Fatalf("absent owner must snapshot empty, got %+v expected=%v err=%v", expected, nil, err)
	}
	// First supported mutation: must write only to the configured state route.
	if err := cfg.SaveExpected(nil); err != nil {
		t.Fatalf("state-route first write must not be rejected as ghost: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state config.yaml not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "config", "config.yaml")); err != nil {
		t.Fatalf("write did not land in the configured config root: %v", err)
	}
	// No stray temp/config files were created outside the configured location.
	if entries, _ := filepath.Glob(filepath.Join(state, ".config-*")); len(entries) != 0 {
		t.Fatalf("temp config leaked into state dir: %v", entries)
	}
	if entries, _ := filepath.Glob(filepath.Join(state, "config", ".config-*")); len(entries) != 0 {
		t.Fatalf("temp config leaked into config root: %v", entries)
	}
}

// TestSaveRouteXDGGhostStillRejected preserves the XDG double-append guard on
// the save route: on the XDG route, an absent <xdg>/baseharbor/config.yaml with
// a config.yaml one level up is the genuine ghost and must still be rejected.
func TestSaveRouteXDGGhostStillRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config", "baseharbor")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", root)
	cfg := Config{Version: ConfigVersion, Access: map[string]AccessDefinition{"local": {Provider: "local", Reference: "local"}}, Targets: map[string]TargetDefinition{"first": {Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "local"}}}}
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("fixture: nested config.yaml must be absent, got %v", statErr)
	}
	if err := cfg.Save(); !errors.Is(err, ErrConfigMissing) {
		t.Fatalf("XDG ghost must still be rejected on save route, got: %v", err)
	}
}
