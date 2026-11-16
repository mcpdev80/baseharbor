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
