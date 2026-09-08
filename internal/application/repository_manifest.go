package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const RepositoryManifestName = "baseharbor.yaml"

const stateGitIgnore = "*\n!.gitignore\n"

// FindRepositoryManifest returns the nearest baseharbor.yaml from start upward.
// This lets developers run baha from nested directories inside an application repository.
func FindRepositoryManifest(start string) (string, error) {
	if start == "" {
		start = "."
	}
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	for {
		candidate := filepath.Join(current, RepositoryManifestName)
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("%s is a directory", candidate)
			}
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect repository manifest: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("%s not found in the current directory or any parent", RepositoryManifestName)
}

func LoadManifestFile(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read application manifest %s: %w", path, err)
	}
	m, err := ParseYAML(string(data))
	if err != nil {
		return Manifest{}, fmt.Errorf("parse application manifest %s: %w", path, err)
	}
	return m, nil
}

// Sync stores a canonical internal copy of a repository manifest while preserving
// already materialized runtime state. The repository file remains authoritative.
func (s Store) Sync(m Manifest) (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	if err := ensureStateGitIgnore(s.Root); err != nil {
		return "", err
	}
	appDir := filepath.Join(s.Root, m.Name)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return "", fmt.Errorf("create application state directory: %w", err)
	}
	if err := os.Chmod(appDir, 0o700); err != nil {
		return "", fmt.Errorf("secure application state directory: %w", err)
	}
	path := filepath.Join(appDir, RepositoryManifestName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(m.YAML()), 0o600); err != nil {
		return "", fmt.Errorf("write synchronized application manifest: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("replace synchronized application manifest: %w", err)
	}
	return path, nil
}

func ensureStateGitIgnore(storeRoot string) error {
	stateDir := filepath.Dir(storeRoot)
	if filepath.Base(storeRoot) != "apps" {
		return nil
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fmt.Errorf("create BaseHarbor state directory: %w", err)
	}
	path := filepath.Join(stateDir, ".gitignore")
	if data, err := os.ReadFile(path); err == nil {
		if string(data) == stateGitIgnore {
			return nil
		}
		return fmt.Errorf("%s exists with unexpected content; refusing to overwrite Git safety rules", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect BaseHarbor state Git ignore: %w", err)
	}
	if err := os.WriteFile(path, []byte(stateGitIgnore), 0o644); err != nil {
		return fmt.Errorf("write BaseHarbor state Git ignore: %w", err)
	}
	return nil
}
