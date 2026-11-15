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

// worldWritableCheckOptOutEnv, when set to "skip", disables the world-writable
// /tmp guard in tests that intentionally use a world-writable t.TempDir() as
// XDG_CONFIG_HOME. Production never sets it, so the guard stays enforced.
const worldWritableCheckOptOutEnv = "BASEHARBOR_SKIP_WORLD_WRITABLE_CHECK"

// configRoot returns the canonical user-state directory that holds the persisted
// organization (target) selection: <user-config-dir>/baseharbor/organization.
//
// The persisted selection is authoritative about which registry reads resolve
// against, so it must never live under a world-writable path (for example a
// sticky-bit /tmp directory reachable via an attacker-controlled
// XDG_CONFIG_HOME/BASEHARBOR_STATE_DIR). os.UserConfigDir() honours
// XDG_CONFIG_HOME when it points at a private user-owned location, but falls
// back to the user's real config directory (XDG_CONFIG_HOME unset -> ~/.config,
// macOS ~/Library/Preferences, Windows %APPDATA%); it never resolves to /tmp.
func configRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return filepath.Join(root, "config", "organization"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return "", errors.New("cannot determine BaseHarbor organization config directory")
	}
	return filepath.Join(base, "baseharbor", "organization"), nil
}

func cacheRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return filepath.Join(root, "cache", "organization"), nil
	}
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
	state, ok, err := LoadActiveOptional()
	if err != nil {
		return ActiveState{}, err
	}
	if !ok {
		return ActiveState{}, fmt.Errorf("%w; run 'baha config organization set'", ErrNotConfigured)
	}
	return state, nil
}

func LoadActiveOptional() (ActiveState, bool, error) {
	path, err := ActivePath()
	if err != nil {
		return ActiveState{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ActiveState{}, false, nil
		}
		return ActiveState{}, false, fmt.Errorf("read active organization configuration: %w", err)
	}
	var state ActiveState
	if err := json.Unmarshal(data, &state); err != nil {
		return ActiveState{}, false, fmt.Errorf("decode active organization configuration: %w", err)
	}
	if err := state.Config.Validate(); err != nil {
		return ActiveState{}, false, fmt.Errorf("validate active organization configuration: %w", err)
	}
	if err := state.Resolution.Validate(); err != nil {
		return ActiveState{}, false, fmt.Errorf("validate active organization resolution: %w", err)
	}
	return state, true, nil
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
	if err := migrateFromWorldWritable(path); err != nil {
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

// migrateFromWorldWritable enforces the world-writable /tmp prohibition for the
// persisted organization (target) selection. The active state determines which
// registry reads resolve against, so it must never sit in a path a local user
// can pre-create under the sticky-bit world-writable /tmp.
//
// Behaviour:
//   - If the resolved path is NOT under a world-writable directory, nothing
//     happens (the normal case for os.UserConfigDir()).
//   - If the resolved path IS under a world-writable directory:
//   - an existing selection file there is adopted once into the canonical
//     user-state location, then left in place as a read-only tombstone —
//     future SaveActive calls reject further writes so the stale /tmp copy
//     can never be updated again;
//   - a missing selection file there is an immediate rejection, because
//     accepting it would persist the vulnerable state.
func migrateFromWorldWritable(path string) error {
	if strings.TrimSpace(os.Getenv(worldWritableCheckOptOutEnv)) == "skip" {
		return nil
	}
	if isWorldFrozenTombstone(filepath.Dir(path)) {
		return fmt.Errorf("organization selection state at %q was frozen after a world-writable /tmp migration; rewrite it in the user-state directory", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// A missing selection is only a problem under a world-writable dir:
			// an attacker could pre-create it. For a private location a missing
			// file is the normal "not configured yet" case and must not be
			// rejected here (SaveActive writes the file afterwards).
			if isWorldWritableDir(filepath.Dir(path)) {
				return fmt.Errorf("organization selection state resolves to a world-writable path and must be written to the user-state directory first: %w", err)
			}
			return nil
		}
		return fmt.Errorf("inspect organization selection state: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("organization selection state at %q is not a regular file", path)
	}

	dir := filepath.Dir(path)
	if !isWorldWritableDir(dir) {
		return nil
	}

	// World-writable: migrate once and turn the old path into a read-only tombstone.
	newPath, err := ActivePath()
	if err != nil {
		return err
	}
	if filepath.Clean(newPath) == filepath.Clean(path) {
		// The canonical location itself is the world-writable path. We cannot
		// write safely there: freeze the existing selection read-only (if any)
		// and refuse further writes until the caller points XDG_CONFIG_HOME at a
		// private directory.
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			if err := os.Chmod(path, 0o400); err != nil {
				return fmt.Errorf("honor stale world-writable organization selection: %w", err)
			}
			tomb := filepath.Join(filepath.Dir(path), "active.world-writable.tombstone")
			_ = os.WriteFile(tomb, []byte("# World-writable organization selection path frozen by migration.\n"+
				"This file exists to prevent a second, identical migration.\n"+
				"Original location: "+path+"\n"), 0o600)
			return fmt.Errorf("organization selection state resides in a world-writable directory %q; freeze it by pointing XDG_CONFIG_HOME at a private user directory", filepath.Dir(path))
		}
		return fmt.Errorf("organization selection state resolves to a world-writable path and must be written to a private user directory first")
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o700); err != nil {
		return fmt.Errorf("create organization config directory: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read active organization configuration from world-writable path: %w", err)
	}
	tmp := newPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write active organization configuration: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("secure active organization configuration: %w", err)
	}
	if err := os.Rename(tmp, newPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace active organization configuration: %w", err)
	}

	// Tombstone: freeze the stale /tmp copy so no further write lands there.
	if err := os.Chmod(path, 0o400); err != nil {
		return fmt.Errorf("honor stale world-writable organization selection: %w", err)
	}
	tomb := filepath.Join(filepath.Dir(path), "active.world-writable.tombstone")
	_ = os.WriteFile(tomb, []byte("# World-writable organization selection path frozen by migration.\n"+
		"This file exists to prevent a second, identical migration.\n"+
		"Original location: "+path+"\n"), 0o600)
	return nil
}

// isWorldFrozenTombstone reports whether dir carries the tombstone marker that
// migrateFromWorldWritable writes after adopting a stale world-writable selection.
// Its presence means the selection has already been migrated and the old path
// must never be written again.
func isWorldFrozenTombstone(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "active.world-writable.tombstone"))
	return err == nil
}

// isWorldWritableDir reports whether dir is a directory that any local user may
// write into: a world-writable directory that is not sticky, or a sticky-bit
// world-writable directory (the classic /tmp pattern). Sticky-bit world-writable
// dirs still allow any user to create a file at a known path before the
// application does, so both classes are treated as unsafe for the selection.
func isWorldWritableDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		// A missing directory still cannot be trusted under a world-writable
		// parent: the caller may pre-create it before SaveActive writes. Fall
		// back to inspecting the parent so a world-writable XDG_CONFIG_HOME is
		// rejected even before the org subdir exists.
		return isWorldWritableDir(filepath.Dir(dir))
	}
	mode := info.Mode()
	if !mode.IsDir() || mode&0o002 == 0 {
		return false
	}
	return true
}

func writeCache(resolution Resolution, config Config, content []byte, sourceRoot string) (string, error) {
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
	if err := cacheRelativeArtifacts(dir, sourceRoot, config); err != nil {
		return "", err
	}
	return path, nil
}

func cacheRelativeArtifacts(cacheDir, sourceRoot string, config Config) error {
	sourceRoot = filepath.Clean(strings.TrimSpace(sourceRoot))
	if sourceRoot == "" {
		return nil
	}
	for _, ref := range organizationArtifactReferences(config) {
		rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(ref)))
		if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if strings.Contains(rel, "://") || strings.Contains(rel, ":") {
			continue
		}
		src := filepath.Join(sourceRoot, rel)
		info, err := os.Lstat(src)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("organization artifact %q does not exist in resolved source", filepath.ToSlash(rel))
		}
		if err != nil {
			return fmt.Errorf("inspect organization artifact %q: %w", filepath.ToSlash(rel), err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("organization artifact %q must be a regular file", filepath.ToSlash(rel))
		}
		if info.Size() > 16<<20 {
			return fmt.Errorf("organization artifact %q exceeds 16 MiB", filepath.ToSlash(rel))
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read organization artifact %q: %w", filepath.ToSlash(rel), err)
		}
		dst := filepath.Join(cacheDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return fmt.Errorf("create organization artifact cache path: %w", err)
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return fmt.Errorf("cache organization artifact %q: %w", filepath.ToSlash(rel), err)
		}
	}
	return nil
}

func organizationArtifactReferences(config Config) []string {
	seen := map[string]struct{}{}
	var result []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, refs := range []map[string]Reference{config.Providers, config.Targets, config.Stacks, config.Trust, config.Policies} {
		for _, ref := range refs {
			add(ref.Reference)
		}
	}
	return result
}

func ResolveCachedArtifact(state ActiveState, reference string) (string, bool, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" || filepath.IsAbs(reference) || strings.Contains(reference, "://") || strings.Contains(reference, ":") {
		return "", false, nil
	}
	rel := filepath.Clean(filepath.FromSlash(reference))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("organization artifact reference %q escapes immutable cache", reference)
	}
	root := filepath.Dir(strings.TrimSpace(state.Resolution.CachePath))
	if root == "." || root == "" {
		return "", false, nil
	}
	path := filepath.Join(root, rel)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("organization artifact %q is not a regular file", reference)
	}
	return path, true, nil
}
