package orgconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type ActiveState struct {
	Resolution Resolution `json:"resolution"`
	Config     Config     `json:"config"`
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

// StateRootSelected reports whether this invocation uses an isolated
// installation root instead of the ambient user configuration.
func StateRootSelected() bool {
	return stateRootOverride() != ""
}

func stateRootOverride() string {
	return strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR"))
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

// ActivePath returns the private, canonical location of the persisted selection
// under the user config directory. It is never under a world-writable path.
func ActivePath() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "active.json"), nil
}

// legacyActivePath returns the prior world-writable location of the persisted
// selection under the (currently resolved) config root. It is used only for a
// one-time migration into the private fallback location (see
// migrateFromWorldWritable) and is never consulted a second time.
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
//
// legacyPath is the world-writable location that was resolved (and refused by
// SaveActive/LoadActive); canonical is the private fallback the value is adopted
// into. It returns true when a legacy file was adopted, false when there was
// nothing to adopt, and a non-nil error on any safety violation.
func adoptLegacySelection(legacyPath, canonical string) (bool, error) {
	info, err := os.Lstat(legacyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect legacy organization selection %q: %w", legacyPath, err)
	}
	uid := uint32(os.Geteuid())
	if err := validateLegacySelection(info, legacyPath, uid); err != nil {
		return false, err
	}
	if err := validateLegacyAncestors(filepath.Dir(legacyPath), uid); err != nil {
		return false, err
	}

	// Never clobber an existing (private) selection.
	if _, err := os.Lstat(canonical); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect canonical organization selection %q: %w", canonical, err)
	}

	// Open once, then validate the opened inode before reading. This closes the
	// Lstat-to-ReadFile replacement window: a substituted symlink or foreign
	// inode will not match the validated entry and cannot be imported.
	f, err := os.Open(legacyPath)
	if err != nil {
		return false, fmt.Errorf("read legacy organization selection %q: %w", legacyPath, err)
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect opened legacy organization selection %q: %w", legacyPath, err)
	}
	if !os.SameFile(info, openedInfo) {
		return false, fmt.Errorf("rejecting changed legacy organization selection at %q", legacyPath)
	}
	if err := validateLegacySelection(openedInfo, legacyPath, uid); err != nil {
		return false, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return false, fmt.Errorf("read legacy organization selection %q: %w", legacyPath, err)
	}
	if err := migrateToCanonical(legacyPath, canonical, data); err != nil {
		// The marker step can only fail AFTER the canonical selection has
		// landed (ordering contract: data before marker). The migrated value
		// is then preserved on the private path: report adopted=true together
		// with the error so no caller re-runs the adoption against the legacy
		// path, while the failure stays visible. Without a canonical file
		// nothing was migrated, so the ordinary fail-closed false+error path
		// applies.
		if _, statErr := os.Lstat(canonical); statErr == nil {
			return true, err
		}
		return false, err
	}
	return true, nil
}

func validateLegacySelection(info os.FileInfo, path string, uid uint32) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("rejecting legacy organization selection at %q: non-regular entry", path)
	}
	owner, ok := fileOwnerUID(info)
	if !ok || owner != uid {
		return fmt.Errorf("rejecting legacy organization selection at %q: file is not owned by the current user", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("rejecting group- or world-writable legacy organization selection at %q", path)
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint32, bool) {
	if info == nil || info.Sys() == nil {
		return 0, false
	}
	v := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return 0, false
	}
	uid := v.FieldByName("Uid")
	if !uid.IsValid() || !uid.CanUint() {
		return 0, false
	}
	return uint32(uid.Uint()), true
}

// validateLegacyAncestors allows a migration only through directories owned
// by root or the current user. A writable ancestor must be sticky, which
// protects entries from replacement by other users (as in /tmp).
func validateLegacyAncestors(dir string, uid uint32) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve legacy organization selection directory: %w", err)
	}
	for current := filepath.Clean(abs); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect legacy organization selection directory %q: %w", current, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("rejecting legacy organization selection: untrusted directory %q", current)
		}
		owner, ok := fileOwnerUID(info)
		if !ok || (owner != uid && owner != 0) {
			return fmt.Errorf("rejecting legacy organization selection: directory %q has untrusted owner", current)
		}
		if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("rejecting legacy organization selection: writable directory %q is not trusted", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}

// canonicalMarkerContent is the exact content of the durable once-only
// migration marker. writeFrozenMarker writes it through the seam, syncs and
// re-verifies it by read-back so a crash during the write can never leave a
// completion claim that later reads would honour while unreadable or empty.
const canonicalMarkerContent = "# One-time world-writable migration marker.\n" +
	"Wrote the adopted selection into this private directory. Do NOT re-read\n" +
	"any legacy /tmp selection, even if active.json is missing.\n"

// canonicalMarkerFilesystem is the seam through which the migration marker is
// created (0428-310). Production keeps defaultCanonicalMarkerFS; tests inject a
// deterministic fault at the marker-write step only, without widening
// permissions anywhere.
type canonicalMarkerFilesystem interface {
	// writeMarker creates path with content mode 0600. A failure must leave no
	// readable completion claim behind.
	writeMarker(path string, content []byte) error
}

type defaultCanonicalMarkerFS struct{}

func (defaultCanonicalMarkerFS) writeMarker(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create frozen migration marker: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure frozen migration marker: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write frozen migration marker: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync frozen migration marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close frozen migration marker: %w", err)
	}
	return nil
}

var canonicalMarkerFS canonicalMarkerFilesystem = defaultCanonicalMarkerFS{}

// writeFrozenMarker durably writes the migration marker through the seam and
// verifies its content afterwards (read-back). A marker that cannot be
// verified is NOT a completion claim: an error is returned so the caller never
// treats a half-written migration as finished. A pre-existing unreadable
// residue (e.g. a directory at the marker path) is reported, not deleted.
func writeFrozenMarker(path string) error {
	orgconfigMarkerEvent(filepath.Base(path))
	if err := canonicalMarkerFS.writeMarker(path, []byte(canonicalMarkerContent)); err != nil {
		return err
	}
	return verifyFrozenMarkerContent(path)
}

// verifyFrozenMarkerContent reads the marker back and compares it against the
// EXACT canonical content constant (0428-310 finding 1). Readability alone is
// NOT a completion claim: an empty, truncated or wrong-content marker returns
// an error so the caller never treats a half-written migration as finished.
func verifyFrozenMarkerContent(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify frozen migration marker: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("verify frozen migration marker: %q is a directory residue", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		// The content is not readable: never present a half-written
		// completion claim to later loads.
		return fmt.Errorf("verify frozen migration marker: %w", err)
	}
	if string(content) != canonicalMarkerContent {
		return fmt.Errorf("verify frozen migration marker: read-back content does not match the canonical marker at %q", path)
	}
	return nil
}

// frozenMarkerComplete reports whether path holds a marker whose read-back
// content equals the canonical marker content exactly (not a directory, not an
// unreadable or content-mismatched residue). This is the predicate the loader
// honours as a completion claim.
func frozenMarkerComplete(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() {
		return false
	}
	content, err := os.ReadFile(path)
	return err == nil && string(content) == canonicalMarkerContent
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
	if err := writeDurableCanonicalData(dst, data); err != nil {
		return err
	}
	// Freeze the source as a tombstone; ignore failure to chmod (best effort)
	// but leave the tombstone marker regardless.
	_ = os.Chmod(src, 0o400)
	legacyTomb := filepath.Join(filepath.Dir(src), "active.world-writable.tombstone")
	_ = os.WriteFile(legacyTomb, []byte("# Legacy organization selection path frozen after one-time migration.\n"+
		"This file exists to prevent a second migration.\n"+
		"Original location: "+src+"\n"), 0o600)
	// Durable once-only marker in the private canonical namespace, written
	// LAST (see ensureFrozenMarker): the data migration holds BEFORE the
	// completion claim, and the claim itself is fsynced into the directory.
	return ensureFrozenMarker(filepath.Dir(dst))
}

// writeDurableCanonicalData persists the migrated selection into dst through a
// same-directory temp file + atomic rename. Durability contract (akte design
// "durability before completion claim", 0428-310 finding 3): the temp FILE is
// fsynced BEFORE the rename AND the containing DIRECTORY is fsynced AFTER it,
// so a crash cannot lose the canonical name while a later marker claims the
// migration finished.
func writeDurableCanonicalData(dst string, data []byte) error {
	tmp := dst + ".tmp"
	tmpFile, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write active organization configuration: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("secure active organization configuration: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write active organization configuration: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("sync active organization configuration: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close active organization configuration: %w", err)
	}
	orgconfigFsync(filepath.Base(tmp))
	if err := orgconfigRename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace active organization configuration: %w", err)
	}
	return syncDir(filepath.Dir(dst))
}

// syncDir fsyncs the directory so a rename or a newly created entry is durable
// (where supported). A platform that refuses to open or sync a directory keeps
// the file-level durability and the ordering contract; this stays a documented
// best-effort no-op there.
func syncDir(dir string) error {
	orgconfigFsync("dir:" + filepath.Base(dir))
	handle, err := os.Open(dir)
	if err != nil {
		return nil
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	if syncErr != nil || closeErr != nil {
		// Directory fsync is unsupported on some platforms; the data-before-
		// marker ordering contract does not depend on it.
		return nil
	}
	return nil
}

// ensureFrozenMarker writes the durable once-only completion marker for the
// directory dir, fsyncing both the marker file and its directory so the claim
// itself is durable. A marker that already reads back COMPLETE is left
// untouched (idempotent re-run); a damaged or wrong-content residue is NOT a
// completion claim and is rewritten through the seam.
func ensureFrozenMarker(dir string) error {
	markerPath := filepath.Join(dir, canonicalFrozenMarker)
	residue, residueErr := os.Lstat(markerPath)
	if residueErr == nil && residue.IsDir() {
		// A directory at the marker path is foreign residue: never delete it,
		// never honour it; report so no completion claim is fabricated.
		return fmt.Errorf("frozen migration marker path %q is a directory residue", markerPath)
	}
	if frozenMarkerComplete(markerPath) {
		return nil
	}
	if err := writeFrozenMarker(markerPath); err != nil {
		return err
	}
	return syncDir(dir)
}

// orgconfigFsync, orgconfigRename and orgconfigMarkerEvent are observability
// seams over the ordered filesystem operations of the migration (0428-310
// finding 3). Production performs the real call; tests record the exact order
// (temp fsync < rename < directory fsync < marker creation) through them.
var (
	orgconfigFsync       = func(string) {}
	orgconfigRename      = func(from, to string) error { return os.Rename(from, to) }
	orgconfigMarkerEvent = func(string) {}
)

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
		//     legacy path may be read again. Return the normal not-configured
		//     result (do NOT fall through to decodeActive with nil data, which
		//     would surface a JSON EOF error instead of ErrNotConfigured);
		//   - otherwise a one-time migration may still apply if the legacy file
		//     still exists.
		if canonicalMigrationFrozen(canonical) {
			return ActiveState{}, false, nil
		}
		adopted, adoptErr := adoptLegacySelection(legacy, canonical)
		if adoptErr != nil {
			// Preserve-and-report (0428-310 finding 2): the marker step can
			// only fail AFTER the canonical selection has landed (ordering
			// contract: data before marker), so adopted=true alongside the
			// error means the migration is REAL. The value already lives on the
			// private canonical path: the loader returns it (never an
			// error-only discard), completes the once-only marker in the same
			// call so the claim can never be lost, and keeps the fault
			// visible. Only a fail-closed adopted=false keeps the pure
			// error path.
			if !adopted {
				return ActiveState{}, false, adoptErr
			}
			migrated, readErr := os.ReadFile(canonical)
			if readErr != nil {
				return ActiveState{}, false, errors.Join(adoptErr, fmt.Errorf("read active organization configuration after migration: %w", readErr))
			}
			markerErr := ensureFrozenMarker(filepath.Dir(canonical))
			state, ok, decodeErr := decodeActive(migrated)
			if markerErr != nil {
				if decodeErr == nil && ok {
					return state, true, errors.Join(adoptErr, markerErr)
				}
				return ActiveState{}, false, errors.Join(adoptErr, markerErr, decodeErr)
			}
			if decodeErr != nil || !ok {
				return ActiveState{}, false, errors.Join(adoptErr, decodeErr)
			}
			return state, true, adoptErr
		}
		if !adopted {
			return ActiveState{}, false, nil
		}
		// Adopted: the canonical path now holds the migrated selection.
		data, err = os.ReadFile(canonical)
		if err != nil {
			return ActiveState{}, false, fmt.Errorf("read active organization configuration after migration: %w", err)
		}
	} else if !frozenMarkerComplete(filepath.Join(filepath.Dir(canonical), canonicalFrozenMarker)) {
		// Direct canonical read succeeded while the completion claim is
		// missing or damaged: the migration REALLY happened (data before
		// marker), so the load repairs the marker before anything else and
		// NEVER discards the canonical selection for a repair fault — the
		// value rides along with the joined error; only an undecodable
		// selection without a claim fails closed.
		markerErr := ensureFrozenMarker(filepath.Dir(canonical))
		state, ok, decodeErr := decodeActive(data)
		if markerErr != nil {
			if decodeErr == nil && ok {
				return state, true, markerErr
			}
			return ActiveState{}, false, errors.Join(markerErr, decodeErr)
		}
		if decodeErr != nil || !ok {
			return state, ok, decodeErr
		}
		if StateRootSelected() && !state.Resolution.selectsWithin(stateRootOverride()) {
			return ActiveState{}, false, nil
		}
		return state, true, nil
	}
	state, ok, err := decodeActive(data)
	if err != nil || !ok {
		return state, ok, err
	}
	if StateRootSelected() && !state.Resolution.selectsWithin(stateRootOverride()) {
		return ActiveState{}, false, nil
	}
	return state, true, nil
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
	return state, true, nil
}

// StateRootOwnsOrganizationState reports whether an active organization
// resolution belongs to this invocation's isolated state root.
func StateRootOwnsOrganizationState(state ActiveState) bool {
	return !StateRootSelected() || state.Resolution.selectsWithin(stateRootOverride())
}

// OrganizationTargetSelection returns the organization's default target, or
// the first configured environment target in stable name order.
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
	// One-time migration: adopt a stale world-writable /tmp selection into the
	// private canonical path before writing, so the write lands in the safe
	// location and never re-touches the /tmp source. The durable once-only
	// marker in the private canonical namespace is consulted first so a deleted
	// canonical file cannot restore the stale legacy selection or force another
	// freeze.
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
	// XDG_CONFIG_HOME/BASEHARBOR_STATE_DIR at a private directory. Once a
	// private selection exists (after a migration), the write proceeds to that
	// private location. A non-world-writable private candidate is always safe.
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

// isWorldFrozenTombstone reports whether dir carries the tombstone marker that
// adoptLegacySelection writes after adopting a stale world-writable selection.
// Its presence means the selection has already been migrated and the old path
// must never be written again.
func isWorldFrozenTombstone(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "active.world-writable.tombstone"))
	return err == nil
}

// canonicalMigrationFrozen reports whether the private canonical organization
// directory already carries the durable once-only migration marker. When it
// does, the legacy /tmp selection has already been adopted and the loader must
// never consult a legacy path again — even if the canonical active.json was
// deleted in the meantime. This is consulted before any legacy-path access so
// a deleted canonical file cannot restore the stale legacy selection.
func canonicalMigrationFrozen(canonical string) bool {
	// The completion claim is the CONTENT of the marker, not mere existence:
	// an empty or wrong-content residue must not freeze the migration path
	// (the next migration/re-load pass repairs it through ensureFrozenMarker).
	marker := filepath.Join(filepath.Dir(canonical), canonicalFrozenMarker)
	return frozenMarkerComplete(marker)
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
