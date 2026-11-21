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
// BF-05 fix must NOT break: a genuinely absent config file under a clean,
// empty config root (fresh install, no config.yaml yet) stays a clean no-config
// with the implicit default.
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

// TestLoadConfigExistingEmptyFileFailsLoud is the round-2 guard (Luna batch17,
// Object 2): an EXISTING but empty/whitespace-only config.yaml is a malformed
// configuration, not a clean absent-config default. The user pointed BaseHarbor
// at this path on purpose, so it must surface an actionable error from the
// ErrConfigMissing family rather than silently falling through to the implicit
// defaults.
func TestLoadConfigExistingEmptyFileFailsLoud(t *testing.T) {
	for _, name := range []string{"empty", "whitespace"} {
		t.Run(name, func(t *testing.T) {
			state := t.TempDir()
			t.Setenv("BASEHARBOR_STATE_DIR", state)
			cfgPath := filepath.Join(state, "config", "config.yaml")
			if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
				t.Fatal(err)
			}
			var content []byte
			if name == "empty" {
				content = []byte{}
			} else {
				content = []byte("   \n\t \n")
			}
			if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig()
			if err == nil {
				t.Fatalf("%s config must fail loud, got empty config %+v", name, cfg)
			}
			if !errors.Is(err, ErrConfigEmpty) {
				t.Fatalf("%s config must return ErrConfigEmpty, got %+v (err=%v)", name, cfg, err)
			}
		})
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

// TestLoadConfigEndToEndXDGPath exercises LoadConfig over the real XDG config
// home with the four cases that matter for BF-05, resolving the path exactly as
// the production ConfigPath()/LoadConfig() do (XDG_CONFIG_HOME/<app>/config.yaml):
//
//   - exists + empty        -> fail loud (ErrConfigEmpty), not the implicit default
//   - missing               -> clean default (genuinely empty root)
//   - corrupt               -> fail loud (unparseable)
//   - valid                 -> loads the configured target
//
// This is the end-to-end witness Luna batch17 requested: the helper predicate
// alone never ran LoadConfig against the XDG double-append setup.
func TestLoadConfigEndToEndXDGPath(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), ".config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	// ConfigPath() resolves to XDG_CONFIG_HOME/baseharbor/config.yaml.
	cfgPath := filepath.Join(xdg, "baseharbor", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}

	// Case 1: exists + empty.
	if err := os.WriteFile(cfgPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); !errors.Is(err, ErrConfigEmpty) {
		t.Fatalf("empty XDG config must return ErrConfigEmpty, got %v", err)
	}

	// Case 2: missing.
	if err := os.Remove(cfgPath); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("missing XDG config under empty root must not error, got %v", err)
	}
	if len(cfg.Targets) != 0 || len(cfg.Access) != 0 {
		t.Fatalf("missing XDG config must yield the empty default, got %+v", cfg)
	}

	// Case 3: corrupt.
	corrupt := []byte("version: 1\n\tBAD INDENT {{{\n")
	if err := os.WriteFile(cfgPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err == nil {
		t.Fatal("corrupt XDG config must fail loud")
	}

	// Case 4: valid.
	valid := []byte("version: 1\naccess:\n  local:\n    provider: local\n    reference: local\ntargets:\n  first:\n    runtime:\n      provider: docker\n    access:\n      reference: local\n")
	if err := os.WriteFile(cfgPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("valid XDG config must load, got %v", err)
	}
	if len(loaded.Targets) != 1 {
		t.Fatalf("valid XDG config did not load its target: %+v", loaded.Targets)
	}
}

// TestLoadConfigDoubleAppendResolvedPathMissingFailsLoud covers the original
// BF-05 reproduction end-to-end at the production LoadConfig(): the XDG
// double-append where XDG_CONFIG_HOME itself already ends in /baseharbor. Here
// ConfigPath() resolves to <root>/baseharbor/baseharbor/config.yaml (a nested,
// absent path) while the user's intended config.yaml sits exactly one level up
// at <root>/baseharbor/config.yaml. LoadConfig must surface ErrConfigMissing
// (the nested root is populated by the sibling file, so the misconfiguration is
// loud) rather than silently returning the implicit default. This is the case
// Luna batch17 found missing: it exercises the real ConfigPath()/LoadConfig()
// resolution, not only the helper predicate.
func TestLoadConfigDoubleAppendResolvedPathMissingFailsLoud(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".config", "baseharbor")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	// The intended config.yaml at the XDG root.
	if err := os.WriteFile(filepath.Join(root, "config.yaml"),
		[]byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", root)
	// ConfigPath() resolves <root>/baseharbor/baseharbor/config.yaml, which is
	// absent, but the sibling one level up marks the root populated.
	if _, err := LoadConfig(); !errors.Is(err, ErrConfigMissing) {
		t.Fatalf("double-append missing config must return ErrConfigMissing, got %v", err)
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
