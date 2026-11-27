package orgconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

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

// orgconfigFsync, orgconfigRename and orgconfigMarkerEvent are observability
// seams over the ordered filesystem operations of the migration (0428-310
// finding 3). Production performs the real call; tests record the exact order
// (temp fsync < rename < directory fsync < marker creation) through them.
var (
	orgconfigFsync       = func(string) {}
	orgconfigRename      = func(from, to string) error { return os.Rename(from, to) }
	orgconfigMarkerEvent = func(string) {}
)

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
