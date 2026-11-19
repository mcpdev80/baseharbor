package deployment

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrConfigurationChanged = errors.New("Target configuration changed; reload and review the current operation before retrying")

// LoadConfigForMutation captures the protected existing owner bytes and uses
// the normal configuration parser. Pair it with SaveExpected; a separate read
// followed by Save cannot acknowledge concurrent Target changes.
func LoadConfigForMutation() (Config, []byte, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, nil, err
	}
	if err := configurationParents(filepath.Dir(path), true); err != nil {
		return Config{}, nil, err
	}
	if _, err := os.Lstat(filepath.Dir(path)); os.IsNotExist(err) {
		return Config{Version: ConfigVersion, Access: map[string]AccessDefinition{}, Targets: map[string]TargetDefinition{}}, nil, nil
	}
	raw, err := readConfigurationOwner(path)
	if err != nil {
		return Config{}, nil, err
	}
	if raw == nil {
		return Config{Version: ConfigVersion, Access: map[string]AccessDefinition{}, Targets: map[string]TargetDefinition{}}, nil, nil
	}
	cfg, err := decodeConfig(raw)
	return cfg, raw, err
}

// SaveExpected replaces the existing owner only if the exact acknowledged
// bytes are still current. No revision or competing Target state is persisted.
// All normal saves use the same existing lifecycle-lock primitive.
func (c Config) SaveExpected(expected []byte) error {
	return c.saveWithExpected(expected, true)
}

func (c Config) saveWithExpected(expected []byte, compare bool) error {
	return c.saveWithPreparation(expected, compare, nil)
}

func (c Config) saveWithPreparation(expected []byte, compare bool, prepare func() error) error {
	if c.Version == 0 {
		c.Version = ConfigVersion
	}
	if err := c.Validate(); err != nil {
		return err
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	root := filepath.Dir(path)
	if err := configurationParents(root, true); err != nil {
		return err
	}
	// The XDG double-append ghost must be rejected BEFORE anything is created
	// on disk (0428-110). The ghost is the XDG-layout where the root does not
	// exist yet and a config.yaml sits in the grandparent: that file is the
	// user's real BaseHarbor root, and creating a nested root below it would
	// silently shadow it. configGhostRoot is route-specific: it only reports
	// true on the XDG route (the resolved root is literally a "baseharbor"
	// child), so a sibling config.yaml above a legitimate state route
	// (<state>/config) stays a clean first write and is never mistaken for a
	// double-append. If the root already exists, BaseHarbor created it on a
	// legitimate earlier run and nothing here can shadow a ghost.
	if _, statErr := os.Lstat(root); os.IsNotExist(statErr) && configGhostRoot(root) {
		return configRootMisconfiguration(path, true)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || !bhruntime.ProtectedStateOwner(root, info) {
		return errors.New("Target configuration mutation requires its protected owner directory")
	}
	if _, err := readConfigurationOwner(filepath.Join(root, ".installation.lock")); err != nil {
		return err
	}
	unlock, err := coreinstallation.AcquireStateFileLock(root)
	if err != nil {
		return err
	}
	defer unlock()
	actual, err := readConfigurationOwner(path)
	if err != nil {
		return err
	}
	if actual == nil {
		// An existing owner this route cannot read (e.g. corrupt content under
		// a non-protected mode) is never a clean absent config: it must fail
		// loud before anything is written (0428-110). Ghost layout is already
		// rejected above.
		if existing, statErr := os.Lstat(path); statErr == nil && existing.Mode().IsRegular() {
			return configRootMisconfiguration(path, false)
		}
	}
	if compare && ((actual == nil) != (expected == nil) || !bytes.Equal(actual, expected)) {
		return ErrConfigurationChanged
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	return c.saveAt(path, func() error {
		current, err := readConfigurationOwner(path)
		if err != nil {
			return err
		}
		if (current == nil) != (actual == nil) || !bytes.Equal(current, actual) {
			return ErrConfigurationChanged
		}
		return nil
	})
}

// Protect mutation against linked/foreign owner substitution as well as stale
// content. An absent owner is valid only for an initial save (expected=nil).
func readConfigurationOwner(path string) ([]byte, error) {
	failure := errors.New("Target configuration mutation requires its protected existing owner")
	if configurationParents(filepath.Dir(path), false) != nil {
		return nil, failure
	}
	before, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !before.Mode().IsRegular() || before.Size() > 8<<20 || !bhruntime.ProtectedStateOwner(path, before) {
		return nil, failure
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, failure
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, failure
	}
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	after, statErr := os.Lstat(path)
	if err != nil || statErr != nil || len(data) > 8<<20 || !os.SameFile(before, after) || !after.Mode().IsRegular() || !bhruntime.ProtectedStateOwner(path, after) {
		return nil, failure
	}
	if data == nil {
		data = []byte{}
	}
	return data, nil
}

func configurationParents(root string, allowMissing bool) error {
	for parent := root; parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if os.IsNotExist(err) && allowMissing {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Target configuration mutation requires an unlinked owner path")
		}
	}
	return nil
}

// configGhostRoot reports the XDG double-append layout for a resolved config
// directory (the user's real BaseHarbor root is dir's parent, which the nested
// resolution silently ignores). It applies ONLY on the XDG route: the resolved
// directory is literally a "baseharbor" child, and a sibling config.yaml one
// level up is the file the user's XDG_CONFIG_HOME already targets. On the
// BASEHARBOR_STATE_DIR route the resolved directory is "config" (not
// "baseharbor"), so a sibling <state>/config.yaml is a coincidence and never a
// ghost. This makes the guard route-specific so a legitimate first write to the
// state route is not rejected (0428-110 r2).
func configGhostRoot(resolvedDir string) bool {
	if filepath.Base(resolvedDir) != "baseharbor" {
		return false
	}
	if _, err := os.Lstat(filepath.Join(resolvedDir, ".installation.lock")); err == nil {
		return false
	}
	parent := filepath.Dir(resolvedDir)
	if parent == resolvedDir {
		return false
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() == "config.yaml" {
			return true
		}
	}
	return false
}

// configRootMisconfiguration names, in one actionable message, which root holds
// the user's real BaseHarbor state when a mutation resolves a config root
// without a readable config.yaml. The conservative contract (0428-110) is to
// reject and explain, never to silently dedupe or merge configurations.
func configRootMisconfiguration(path string, ghost bool) error {
	if ghost {
		return fmt.Errorf("%w: %s. The config root resolves into an extra directory level (XDG double-append): BaseHarbor state also sits one level up at %s. BaseHarbor will not merge or deduplicate two config roots; unset the duplicated path component (for example export XDG_CONFIG_HOME=%s) or remove the ghost root, then retry", ErrConfigMissing, path, filepath.Join(filepath.Dir(path), "..", "config.yaml"), filepath.Dir(filepath.Dir(path)))
	}
	return fmt.Errorf("%w: %s", ErrConfigMissing, path)
}
