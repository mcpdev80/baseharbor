package orgconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ActiveState struct {
	Resolution Resolution `json:"resolution"`
	Config     Config     `json:"config"`
}

func configRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return filepath.Join(root, "config", "organization"), nil
	}
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

// StateRootSelected reports whether BASEHARBOR_STATE_DIR selects a dedicated
// isolated installation root for this invocation. When it does, the
// installation's organization selection and cached configuration resolve
// exclusively below that root (BF-01, h-d28 akte;
// docs/explanation/targets.md): the resolver must never fall through to an
// ambient organization state left by a different installation, and an ambient
// resolution's cached artifacts must never answer below an isolated root.
func StateRootSelected() bool {
	return stateRootOverride() != ""
}

// stateRootSelected is the package-internal alias of StateRootSelected.
func stateRootSelected() bool {
	return StateRootSelected()
}

// stateRootOverride returns the trimmed BASEHARBOR_STATE_DIR value: the
// authoritative isolated installation root for this invocation (the same
// precedence source deployment.ConfigPath and DataRoot honour).
func stateRootOverride() string {
	return strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR"))
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
	// BF-01 (h-d28): with a dedicated state root selected, the organization
	// resolves exclusively below that root. The source location is the
	// state root's own claim about where its organization state lives; a
	// selection written by a different installation — or an ambient one
	// with no location claim at all — must not act inside an isolated
	// root. Root-local organization state written through the CLI names
	// its own location below the root and stays authoritative.
	if StateRootSelected() && !state.Resolution.selectsWithin(stateRootOverride()) {
		return ActiveState{}, false, nil
	}
	return state, true, nil
}

// selectsWithin reports whether the resolution's organization state belongs
// to the given installation state root: its source or cache reference lives
// below that root, it names the root's own organization state directory, or
// it carries no location claim at all — an empty resolution is the root's
// state wherever the root keeps it (BF-01, h-d28 akte).
func (r Resolution) selectsWithin(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return true
	}
	source := filepath.Clean(strings.TrimSpace(r.Source.Location))
	if source == "" && strings.TrimSpace(r.CachePath) == "" {
		return true
	}
	for _, path := range []string{source, r.CachePath} {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return true
		}
	}
	return source == filepath.Join(root, "config", "organization") || source == filepath.Join(root, "config")
}

// StateRootOwnsOrganizationState reports whether the active organization
// state is the selected installation root's own state: under a dedicated
// state root, the state resolves exclusively below that root, so the
// selection a call consults is this installation's selection — never the
// organization default of a different installation (BF-01, h-d28 akte;
// docs/explanation/targets.md: the override takes precedence over XDG data
// and configuration paths). Without a selected root the state is the
// root-less user state it has always been.
func StateRootOwnsOrganizationState(state ActiveState) bool {
	if !StateRootSelected() {
		return true
	}
	return state.Resolution.selectsWithin(stateRootOverride())
}

// OrganizationTargetSelection returns the target the organization state
// selects: the organization's own default when it names one, otherwise the
// first per-environment target. Empty means the organization state selects
// no target.
func OrganizationTargetSelection(state ActiveState) string {
	if target := strings.TrimSpace(state.Config.Defaults.Target); target != "" {
		return target
	}
	names := make([]string, 0, len(state.Config.Environments))
	for name := range state.Config.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if target := strings.TrimSpace(state.Config.Environments[name].Target); target != "" {
			return target
		}
	}
	return ""
}

// underRoot reports whether path equals or lives below root.
func underRoot(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	return path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
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
