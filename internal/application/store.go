package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var ErrExists = errors.New("application already exists")

type Store struct {
	Root string
}

func DefaultStore() Store { return Store{Root: filepath.Join(".baseharbor", "apps")} }

func (s Store) Create(m Manifest) (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return "", err
	}
	appDir := filepath.Join(s.Root, m.Name)
	if _, err := os.Stat(appDir); err == nil {
		return "", fmt.Errorf("%w: %s", ErrExists, m.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Mkdir(appDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(appDir, "baseharbor.yaml")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(m.YAML()), 0o600); err != nil {
		_ = os.RemoveAll(appDir)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.RemoveAll(appDir)
		return "", err
	}
	return path, nil
}

func (s Store) Load(name string) (Manifest, string, error) {
	if err := validateSlug("application name", name); err != nil {
		return Manifest{}, "", err
	}
	path := filepath.Join(s.Root, name, "baseharbor.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Manifest{}, path, fmt.Errorf("application %q not found", name)
		}
		return Manifest{}, path, err
	}
	m, err := ParseYAML(string(data))
	return m, path, err
}

func (s Store) List() ([]Manifest, error) {
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m, _, err := s.Load(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", entry.Name(), err)
		}
		items = append(items, m)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}
