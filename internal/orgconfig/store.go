package orgconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ActiveState struct {
	Resolution Resolution `json:"resolution"`
	Config     Config     `json:"config"`
}

func configRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); root != "" {
		return filepath.Join(root, "baseharbor", "organization"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("cannot determine BaseHarbor organization config directory")
	}
	return filepath.Join(home, ".config", "baseharbor", "organization"), nil
}

func cacheRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); root != "" {
		return filepath.Join(root, "baseharbor", "organization"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("cannot determine BaseHarbor organization cache directory")
	}
	return filepath.Join(home, ".cache", "baseharbor", "organization"), nil
}

func ActivePath() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "active.json"), nil
}

func LoadActive() (ActiveState, error) {
	path, err := ActivePath()
	if err != nil {
		return ActiveState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ActiveState{}, fmt.Errorf("organization configuration is not configured; run 'baha config organization set'")
		}
		return ActiveState{}, fmt.Errorf("read active organization configuration: %w", err)
	}
	var state ActiveState
	if err := json.Unmarshal(data, &state); err != nil {
		return ActiveState{}, fmt.Errorf("decode active organization configuration: %w", err)
	}
	if err := state.Config.Validate(); err != nil {
		return ActiveState{}, fmt.Errorf("validate active organization configuration: %w", err)
	}
	if err := state.Resolution.Validate(); err != nil {
		return ActiveState{}, fmt.Errorf("validate active organization resolution: %w", err)
	}
	return state, nil
}

func SaveActive(state ActiveState) error {
	if err := state.Config.Validate(); err != nil {
		return err
	}
	if err := state.Resolution.Validate(); err != nil {
		return err
	}
	path, err := ActivePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create organization config directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode active organization configuration: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write active organization configuration: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("secure active organization configuration: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace active organization configuration: %w", err)
	}
	return nil
}

func writeCache(resolution Resolution, content []byte) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	key := strings.TrimPrefix(strings.TrimSpace(resolution.ResolvedDigest), "sha256:")
	if key == "" {
		key = strings.TrimSpace(resolution.ResolvedRevision)
	}
	if key == "" {
		return "", errors.New("organization resolution has no immutable cache key")
	}
	if len(key) > 64 {
		key = key[:64]
	}
	dir := filepath.Join(root, key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create organization cache: %w", err)
	}
	path := filepath.Join(dir, "organization.yaml")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", fmt.Errorf("write organization cache: %w", err)
	}
	return path, nil
}
