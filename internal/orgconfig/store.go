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
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return "", errors.New("cannot determine BaseHarbor organization config directory")
	}
	return filepath.Join(base, "baseharbor", "organization"), nil
}

// userConfigRoot returns the root-less organization state location with the
// documented environment precedence (XDG_CONFIG_HOME, then the user config
// home): the location `baha config organization set` records when the
// ambient developer installation activates an organization.
func userConfigRoot() (string, error) {
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
	root, err := organizationDirRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "active.json"), nil
}

// organizationDirRoot selects the organization state directory for the
// invocation: the isolated installation root's own location under
// BASEHARBOR_STATE_DIR, the ambient user location otherwise.
func organizationDirRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return filepath.Join(root, "config", "organization"), nil
	}
	return userConfigRoot()
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
	canonical, err := privateCanonicalPath()
	if err != nil {
		return ActiveState{}, false, err
	}
	legacy, err := legacyActivePath()
	if err != nil {
		return ActiveState{}, false, err
	}
	path := canonical
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return ActiveState{}, false, fmt.Errorf("read active organization configuration: %w", err)
		}
		// The private selection is absent. Two cases:
		//   - the durable once-only marker is present: the legacy /tmp selection
		//     was already adopted and frozen, so nothing exists to load and no
		//     legacy path may be read again — return the normal not-configured
		//     result;
		//   - otherwise a one-time migration may still apply if the legacy file
		//     still exists.
		if canonicalMigrationFrozen(canonical) {
			return ActiveState{}, false, nil
		}
		adopted, adoptErr := adoptLegacySelection(legacy, canonical)
		if adoptErr != nil {
			return ActiveState{}, false, adoptErr
		}
		if !adopted {
			return ActiveState{}, false, nil
		}
		// Adopted: the canonical path now holds the migrated selection.
		data, err = os.ReadFile(canonical)
		if err != nil {
			return ActiveState{}, false, fmt.Errorf("read active organization configuration after migration: %w", err)
		}
	}
	return decodeActive(data)
}

// decodeActive validates a decoded active organization configuration. It is
// shared by LoadActiveOptional (both the direct-read and one-time-migration
// paths) so adoption never falls through to a stale read error.
func decodeActive(data []byte) (ActiveState, bool, error) {
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
	// BF-01 (h-d28): with a dedicated state root selected, organization
	// state resolves exclusively below that root — the documented override
	// precedence (docs/explanation/targets.md: the override takes
	// precedence over XDG data and configuration paths). A resolution
	// that names a source or cache reference outside the selected root
	// belongs to a different installation; it must not act inside an
	// isolated root. A root-local state written through the CLI names its
	// own location below the root and stays authoritative.
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
func StateRootOwnsOrganizationState(state ActiveState) bool {
	if !StateRootSelected() {
		return true
	}
	return state.Resolution.selectsWithin(stateRootOverride())
}

// legacyTempSelectionRel is the relative path (under the world-writable config
// root) of the pre-fix persisted selection. It is adopted once into the private
// canonical location and then never consulted again.
const legacyTempSelectionRel = "active.json"

// canonicalFrozenMarker is a durable, once-only migration marker written into
// the private canonical organization directory when the persisted selection is
// adopted from a world-writable /tmp location. Its presence means the legacy
// /tmp selection has already been migrated, so the loader must never read a
// legacy path again — even if the canonical active.json is later deleted. It
// lives in the private canonical namespace (not the world-writable source) so
// an attacker cannot recreate it in /tmp to force a re-adoption.
const canonicalFrozenMarker = "active.frozen.tombstone"

// worldWritableCheckOptOutEnv, when set to "skip", disables the world-writable
// /tmp guard in tests that intentionally use a world-writable t.TempDir() as
// XDG_CONFIG_HOME. Production never sets it, so the guard stays enforced.
const worldWritableCheckOptOutEnv = "BASEHARBOR_SKIP_WORLD_WRITABLE_CHECK"

// legacyActivePath returns the prior world-writable location of the persisted
// selection under the (currently resolved) config root. It is used only for a
// one-time migration into the private fallback location (see
// adoptLegacySelection) and is never consulted a second time.
func legacyActivePath() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, legacyTempSelectionRel), nil
}

// privateFallbackConfigRoot returns the private, non-world-writable config root
// that ignores a broken XDG_CONFIG_HOME pointing at a world-writable directory.
// It follows the same 0700/0600 convention as the rest of the project: config
// under the user's home directory.
func privateFallbackConfigRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("cannot determine BaseHarbor user home directory")
	}
	return filepath.Join(home, ".config", "baseharbor", "organization"), nil
}

// privateCanonicalPath returns the private, non-world-writable path of the
// persisted selection. When XDG_CONFIG_HOME is a private directory it equals
// ActivePath(); when XDG_CONFIG_HOME (or BASEHARBOR_STATE_DIR) resolves to a
// world-writable location it falls back to the user's real config directory so
// the selection is never written under /tmp.
func privateCanonicalPath() (string, error) {
	candidate, err := ActivePath()
	if err != nil {
		return "", err
	}
	if os.Getenv(worldWritableCheckOptOutEnv) == "skip" || isWorldWritableDir(filepath.Dir(candidate)) {
		return privateFallbackPath()
	}
	return candidate, nil
}

// isWorldWritableCandidate reports whether the currently resolved selection
// location (ActivePath) sits under a world-writable directory, independent of
// the private fallback applied by privateCanonicalPath.
func isWorldWritableCandidate() bool {
	candidate, err := ActivePath()
	if err != nil {
		return false
	}
	return isWorldWritableDir(filepath.Dir(candidate))
}

// privateFallbackPath returns the private canonical selection path under the
// user's real config directory, independent of a broken world-writable
// XDG_CONFIG_HOME/BASEHARBOR_STATE_DIR.
func privateFallbackPath() (string, error) {
	root, err := privateFallbackConfigRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "active.json"), nil
}

// adoptLegacySelection migrates a stale selection from a world-writable /tmp
// location into the private canonical location exactly once. It is fail-closed:
// it rejects symlinks and any non-regular entry (Lstat, no-follow), refuses to
// read from a world-writable source, and refuses to overwrite a private
// selection that already exists. After a successful adopt the source file is
// frozen read-only with a tombstone marker and a durable once-only marker is
// written into the private canonical directory so the location can never
// receive a further write or a second adopt.
func adoptLegacySelection(legacyPath, canonical string) (bool, error) {
	info, err := os.Lstat(legacyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect legacy organization selection %q: %w", legacyPath, err)
	}
	// Fail closed: a symlink or any non-regular entry must not be adopted.
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("rejecting legacy organization selection at %q: symlink or non-regular entry", legacyPath)
	}
	// Refuse to read a world-writable source: an attacker may have pre-created or
	// replaced the file between the stat and the read.
	if info.Mode().Perm()&0o002 != 0 {
		return false, fmt.Errorf("rejecting world-writable legacy organization selection at %q", legacyPath)
	}
	// Never clobber an existing (private) selection.
	if _, err := os.Lstat(canonical); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect canonical organization selection %q: %w", canonical, err)
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return false, fmt.Errorf("read legacy organization selection %q: %w", legacyPath, err)
	}
	if err := migrateToCanonical(legacyPath, canonical, data); err != nil {
		return false, err
	}
	return true, nil
}

// migrateToCanonical writes data into canonical using a temp+rename within the
// canonical directory, freezes the source as a read-only tombstone so it can
// never receive a further write, and writes the durable once-only marker into
// the private canonical directory so adoption can never fire a second time,
// even if the canonical active.json is deleted.
func migrateToCanonical(src, dst string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create organization config directory: %w", err)
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write active organization configuration: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("secure active organization configuration: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace active organization configuration: %w", err)
	}
	// Freeze the source as a tombstone; ignore failure to chmod (best effort)
	// but leave the tombstone marker regardless.
	_ = os.Chmod(src, 0o400)
	legacyTomb := filepath.Join(filepath.Dir(src), "active.world-writable.tombstone")
	_ = os.WriteFile(legacyTomb, []byte("# Legacy organization selection path frozen after one-time migration.\n"+
		"This file exists to prevent a second migration.\n"+
		"Original location: "+src+"\n"), 0o600)
	// Durable once-only marker in the private canonical namespace. Write it
	// last (after the canonical file is in place) so a crash before the file
	// lands cannot leave a marker without the migrated selection.
	if err := os.WriteFile(filepath.Join(filepath.Dir(dst), canonicalFrozenMarker), []byte(
		"# One-time world-writable migration marker.\n"+
			"Wrote the adopted selection into this private directory. Do NOT re-read\n"+
			"any legacy /tmp selection, even if active.json is missing.\n"), 0o600); err != nil {
		return fmt.Errorf("write frozen migration marker: %w", err)
	}
	return nil
}

// isWorldFrozenTombstone reports whether dir carries the tombstone marker that
// adoptLegacySelection writes after adopting a stale world-writable selection.
func isWorldFrozenTombstone(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "active.world-writable.tombstone"))
	return err == nil
}

// canonicalMigrationFrozen reports whether the private canonical organization
// directory already carries the durable once-only migration marker.
func canonicalMigrationFrozen(canonical string) bool {
	marker := filepath.Join(filepath.Dir(canonical), canonicalFrozenMarker)
	info, err := os.Lstat(marker)
	return err == nil && !info.Mode().IsDir()
}

// isWorldWritableDir reports whether dir is a directory that any local user may
// write into: a world-writable directory that is not sticky, or a sticky-bit
// world-writable directory (the classic /tmp pattern).
func isWorldWritableDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		// A missing directory still cannot be trusted under a world-writable
		// parent.
		return isWorldWritableDir(filepath.Dir(dir))
	}
	mode := info.Mode()
	if !mode.IsDir() || mode&0o002 == 0 {
		return false
	}
	return true
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
	canonical, err := privateCanonicalPath()
	if err != nil {
		return err
	}
	legacy, err := legacyActivePath()
	if err != nil {
		return err
	}
	// One-time migration: adopt a stale world-writable /tmp selection into
	// the private canonical path before writing, so the write lands in the
	// safe location and never re-touches the /tmp source. The durable
	// once-only marker in the private canonical namespace is consulted
	// first so a deleted canonical file cannot restore the stale legacy
	// selection or force another freeze.
	adopted := false
	if !canonicalMigrationFrozen(canonical) {
		var adoptErr error
		adopted, adoptErr = adoptLegacySelection(legacy, canonical)
		if adoptErr != nil {
			return adoptErr
		}
	}
	// If the resolved selection location is world-writable and there is no
	// private selection to write into, the write would land in a
	// world-writable spot. That is refused: the caller must point
	// XDG_CONFIG_HOME/BASEHARBOR_STATE_DIR at a private directory.
	if !adopted {
		if _, statErr := os.Stat(canonical); errors.Is(statErr, os.ErrNotExist) && isWorldWritableCandidate() {
			return fmt.Errorf("organization selection state resolves to a world-writable path and must be written to a private user directory first")
		}
	}
	if err := os.MkdirAll(filepath.Dir(canonical), 0o700); err != nil {
		return fmt.Errorf("create organization config directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode active organization configuration: %w", err)
	}
	data = append(data, '\n')
	tmp := canonical + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write active organization configuration: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("secure active organization configuration: %w", err)
	}
	if err := os.Rename(tmp, canonical); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace active organization configuration: %w", err)
	}
	return nil
}

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
