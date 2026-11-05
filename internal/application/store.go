package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrExists = errors.New("application already exists")

// isTransientBuildDir reports whether name is the in-progress build directory
// Create leaves behind before its atomic rename. Build directories are named
// "<app>.<pid>.tmp<nanos>", so a valid application slug (letters, digits and
// hyphens only, no dots) can never collide with one.
func isTransientBuildDir(name string) bool {
	if !strings.Contains(name, ".tmp") {
		return false
	}
	return validateSlug("application name", name) != nil
}

type Store struct {
	Root      string
	Namespace string
}

func DefaultStore() Store {
	if manifest, err := FindRepositoryManifest("."); err == nil {
		return Store{Root: filepath.Join(filepath.Dir(manifest), ".baseharbor", "apps")}
	}
	return Store{Root: filepath.Join(".baseharbor", "apps")}
}

func (s Store) Create(m Manifest) (string, error) {
	if m.ApplicationID == "" {
		return "", fmt.Errorf("application id is required")
	}
	if err := ValidateApplicationID(m.ApplicationID); err != nil {
		return "", err
	}
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
	// Build the app in a uniquely named sibling temp directory and atomically
	// rename it into place. The atomic directory rename guarantees List never
	// observes a half-built app (a directory without its manifest), which
	// would otherwise surface a spurious "not found" during concurrent first
	// registration. The per-try unique temp name keeps concurrent creators
	// from clobbering each other's in-progress builds, and normalizes the
	// Mkdir/rename EEXIST races to the same ErrExists sentinel callers retry on.
	tmpName := fmt.Sprintf("%s.%d.tmp%d", appDir, os.Getpid(), time.Now().UnixNano())
	if err := os.Mkdir(tmpName, 0o700); err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("%w: %s", ErrExists, m.Name)
		}
		return "", err
	}
	finalPath := filepath.Join(appDir, "baseharbor.yaml")
	buildPath := filepath.Join(tmpName, "baseharbor.yaml")
	if err := os.WriteFile(buildPath, []byte(m.YAML()), 0o600); err != nil {
		_ = os.RemoveAll(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, appDir); err != nil {
		_ = os.RemoveAll(tmpName)
		if os.IsExist(err) {
			return "", fmt.Errorf("%w: %s", ErrExists, m.Name)
		}
		return "", err
	}
	// Report the durable, renamed path (not the transient build path) so the
	// returned manifest location survives the atomic rename.
	return finalPath, nil
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
	m, err := ParseCanonicalYAML(string(data))
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
		// An in-progress Create builds the app under a uniquely named
		// "*.tmp<nanos>" directory and atomically renames it into place. Skip
		// these transient entries so List never surfaces a half-built app as an
		// application identity.
		if isTransientBuildDir(entry.Name()) {
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
