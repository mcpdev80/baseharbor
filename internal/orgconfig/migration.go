package orgconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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

// orgconfigFsync, orgconfigRename, orgconfigMarkerEvent and orgconfigSyncDir are
// seams over the ordered filesystem operations of the migration (0428-310
// finding 3). Production performs the real call; tests record the exact order
// (temp fsync < rename < directory fsync < marker creation) or inject a
// deterministic data-directory-sync failure to prove the completion marker is
// never created after such a failure.
var (
	orgconfigFsync       = func(string) {}
	orgconfigRename      = func(from, to string) error { return os.Rename(from, to) }
	orgconfigMarkerEvent = func(string) {}
	orgconfigSyncDir     = func(dir string) error { return syncDir(dir) }
	// orgconfigSyncDirFault, when non-nil, is the deterministic fault injected
	// through the REAL syncDir node in the regression test. syncDir wraps it
	// into a *syncDurabilityError so the seam returns the same contract the
	// production path does (errors.As-conform); the test replaces syncDir's
	// directory open with this hook rather than swapping the whole seam, so the
	// error is never returned unwrapped.
	orgconfigSyncDirFault = func(dir string) error { return nil }
)

// errSyncDirFailure is the deterministic data-directory-sync fault injected
// through orgconfigSyncDir in the regression test. It stands in for a real
// open/Sync/Close I/O error: the canonical data directory could not be
// confirmed durable, so the once-only migration must not be claimed complete.
var errSyncDirFailure = errors.New("test fault: data directory fsync refused")

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
	// Durability contract (0428-310 finding: data-directory sync is part of the
	// data-before-completion contract): the canonical data directory must be
	// confirmed durable BEFORE the once-only completion marker is created. The
	// marker is the completion claim, so a data-directory sync error aborts
	// here and the marker is never written — the migration is NOT complete.
	// This goes through orgconfigSyncDir so the ordered-operations seam records
	// it and the deterministic fault hook applies here as well.
	if err := orgconfigSyncDir(filepath.Dir(dst)); err != nil {
		return err
	}
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
	return orgconfigSyncDir(filepath.Dir(dst))
}

// syncDurabilityError classifies a failed directory fsync so the caller can
// tell an explicitly directory-fsync-unsupported platform apart from a
// transient I/O error. On a platform that explicitly does NOT support
// directory fsync (read-only mount / EROFS, or a directory that cannot be
// opened for fsync) the once-only migration can never be claimed durable, so
// the caller fails closed and never writes the completion marker. A transient
// I/O error (for example EIO) is also a hard failure: the data directory could
// not be confirmed durable, so the completion marker must not be written
// either. Every returned error therefore fails the migration closed; the
// classification only distinguishes WHY, so a future caller can surface the
// platform-unsupported case without ever treating it as a best-effort no-op.
type syncDurabilityError struct {
	err         error
	unsupported bool
}

func (e *syncDurabilityError) Error() string {
	return fmt.Sprintf("orgconfig: directory fsync failed: %v", e.err)
}

func (e *syncDurabilityError) Unwrap() error { return e.err }

// isUnsupportedDirectorySync reports whether the directory fsync failed because
// the platform/file-system explicitly does not support it (a read-only mount,
// EROFS) rather than a transient I/O error. EROFS is the one errno that is
// available on every Go platform build here; a directory that cannot be opened
// for fsync (EISDIR-style open failure) is treated as the same unsupported
// case. Everything else is a real I/O error that is still reported to the
// caller — it never falls through to a best-effort no-op.
func isUnsupportedDirectorySync(err error) bool {
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		return false
	}
	return pathErr.Err == syscall.EROFS
}

// syncDir fsyncs the directory so a rename or a newly created entry is durable.
// The result is now part of the data-before-completion contract (0428-310
// dispute finale): a real open/Sync/Close I/O error is returned, not swallowed,
// so the caller never claims the once-only migration durable when the data
// directory could not be confirmed durable. The explicitly directory-fsync-
// unsupported platform case (ESYNC / ENOTSUP / EROFS) is identified and also
// fails closed: on that platform the migration can never be claimed durable, so
// it is reported, not mistaken for a transient error or treated as a best-effort
// no-op.
func syncDir(dir string) error {
	orgconfigFsync("dir:" + filepath.Base(dir))
	if fault := orgconfigSyncDirFault(dir); fault != nil {
		// Wrap the injected fault into the contract type so the seam returns
		// exactly what a real open/Sync/Close failure returns: a
		// *syncDurabilityError the caller can errors.As on.
		return &syncDurabilityError{err: fault}
	}
	handle, err := os.Open(dir)
	if err != nil {
		return &syncDurabilityError{err: fmt.Errorf("open orgconfig directory for fsync: %w", err), unsupported: true}
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	if syncErr != nil {
		return &syncDurabilityError{err: fmt.Errorf("sync orgconfig directory: %w", syncErr)}
	}
	if closeErr != nil {
		return &syncDurabilityError{err: fmt.Errorf("close orgconfig directory: %w", closeErr)}
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
	return orgconfigSyncDir(dir)
}
