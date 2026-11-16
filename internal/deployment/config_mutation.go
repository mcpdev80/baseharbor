package deployment

import (
	"bytes"
	"errors"
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
