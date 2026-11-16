package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigFailsLoudOnUnparseableConfig covers BF-05: a config file that
// exists at the path LoadConfig() resolves but cannot be parsed must fail loud
// with a clear error, never fall through to the implicit empty default. The
// user must see that their configuration was not read.
func TestLoadConfigFailsLoudOnUnparseableConfig(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	cfgPath := filepath.Join(state, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("version: 1\n\tBAD INDENT {{{\n")
	if err := os.WriteFile(cfgPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err == nil {
		t.Fatalf("unparseable config must fail loud, got empty config %+v", cfg)
	}
}

// TestLoadConfigMissingUnderPopulatedRootFailsLoud covers the second half of
// BF-05: when the resolved config path is absent but the config root already
// carries BaseHarbor state (or a config.yaml one level up from the resolved
// path, the XDG double-append that made the H-D28 repro fall silently
// through), LoadConfig must surface ErrConfigMissing rather than silently
// returning the implicit default.
func TestLoadConfigMissingUnderPopulatedRootFailsLoud(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	cfgPath := filepath.Join(state, "config", "config.yaml")
	// The config directory exists and carries deployment-target state (an
	// active-target marker), but the authoritative config.yaml is missing.
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfgPath), "active-target"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if !errors.Is(err, ErrConfigMissing) {
		t.Fatalf("missing config under populated root must return ErrConfigMissing, got %+v (err=%v)", cfg, err)
	}
}

// TestLoadConfigMissingUnderEmptyRootSucceeds guards the regression that the
// BF-05 fix must NOT break: a genuinely empty config root (fresh install, no
// config.yaml yet) stays a clean no-config with the implicit default.
func TestLoadConfigMissingUnderEmptyRootSucceeds(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	// State dir exists but no config/ subdir at all.
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("absent config root must not error, got %v", err)
	}
	if len(cfg.Targets) != 0 || len(cfg.Access) != 0 {
		t.Fatalf("absent config root must yield the empty default, got %+v", cfg)
	}
}

// TestLoadConfigEmptyFileSucceeds guards: a deliberately empty config.yaml
// carries no user intent and must stay a clean no-config (not a parse error
// on version 0), so the daemon never fails on an emptied config.
func TestLoadConfigEmptyFileSucceeds(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	cfgPath := filepath.Join(state, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("empty config file must not error, got %v", err)
	}
	if len(cfg.Targets) != 0 {
		t.Fatalf("empty config file must yield the empty default, got %+v", cfg)
	}
}

// TestLoadConfigValidStillLoads is the happy-path guard: a well-formed config
// under a populated root still loads normally after the BF-05 changes.
func TestLoadConfigValidStillLoads(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	cfg := Config{Version: ConfigVersion, Access: map[string]AccessDefinition{"local": {Provider: "local", Reference: "local"}}, Targets: map[string]TargetDefinition{"first": {Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "local"}}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("valid config must load, got %v", err)
	}
	if len(loaded.Targets) != 1 {
		t.Fatalf("valid config did not load its target: %+v", loaded.Targets)
	}
}

// TestDirExistsAndHasBaseharborState exercises the detection helper directly for
// the two directions the fix depends on: a populated root and a config.yaml one
// level up (the XDG double-append), plus the clean empty root.
func TestDirExistsAndHasBaseharborState(t *testing.T) {
	// Populated root: a deployment-marker (active-target) next to the missing
	// config.yaml.
	populated := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(populated, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(populated, "active-target"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !dirExistsAndHasBaseharborState(populated) {
		t.Fatal("populated root must be detected")
	}

	// An org-config subtree (active.json / organization/) must NOT, by itself,
	// mark the deployment config root as populated — that is a separate system
	// and flagging it would regress the org-config scoping tests.
	orgOnly := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(filepath.Join(orgOnly, "organization"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orgOnly, "organization", "active.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dirExistsAndHasBaseharborState(orgOnly) {
		t.Fatal("org-config-only root must not be detected as populated")
	}

	// config.yaml one level up from the resolved path (XDG double-append):
	// LoadConfig receives filepath.Dir(resolved) = upLevel/baseharbor, and the
	// user's config.yaml sits in upLevel, one level above that.
	upLevel := filepath.Join(t.TempDir(), ".config", "baseharbor")
	if err := os.MkdirAll(upLevel, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(upLevel, "config.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !dirExistsAndHasBaseharborState(filepath.Join(upLevel, "baseharbor")) {
		t.Fatal("config.yaml one level up must be detected")
	}

	// Clean empty root.
	empty := filepath.Join(t.TempDir(), "config")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if dirExistsAndHasBaseharborState(empty) {
		t.Fatal("empty root must not be detected as populated")
	}

	// Absent root.
	if dirExistsAndHasBaseharborState(filepath.Join(t.TempDir(), "nope")) {
		t.Fatal("absent root must not be detected as populated")
	}
}
