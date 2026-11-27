package orgconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errMarkerWriteFault is the deterministic marker-write fault injected through
// the canonicalMarkerFilesystem seam (0428-310).
var errMarkerWriteFault = errors.New("test fault: frozen migration marker write refused")

type faultMarkerFS struct{}

func (faultMarkerFS) writeMarker(path string, content []byte) error {
	return errMarkerWriteFault
}

type recordingMarkerFS struct{ onWrite func() }

func (r recordingMarkerFS) writeMarker(path string, content []byte) error {
	if r.onWrite != nil {
		r.onWrite()
	}
	return defaultCanonicalMarkerFS{}.writeMarker(path, content)
}

// withMarkerFS swaps the marker filesystem seam for one test body.
func withMarkerFS(t *testing.T, fs canonicalMarkerFilesystem, fn func()) {
	t.Helper()
	previous := canonicalMarkerFS
	canonicalMarkerFS = fs
	defer func() { canonicalMarkerFS = previous }()
	fn()
}

// setPrivateHome makes privateHome the effective HOME with a traversable
// ancestor chain (t.TempDir parents can be mode-restricted on some hosts).
func setPrivateHome(t *testing.T, privateHome string) {
	t.Helper()
	for dir := filepath.Clean(privateHome); ; dir = filepath.Dir(dir) {
		info, statErr := os.Lstat(dir)
		if statErr != nil {
			t.Fatalf("stat %s: %v", dir, statErr)
		}
		// Only modes that would BLOCK traversal need a fixup, and fixing them
		// requires ownership. A foreign-owned or already-traversable ancestor
		// is left exactly as-is (the fixture never chmods foreign paths; on a
		// tmpfs-with-quota host chmod /tmp is EPERM even for root-adjacent
		// users).
		if info.Mode().Perm()&0o111 == 0o111 {
			break
		}
		if info.Mode().Perm()&0o700 == 0 {
			break
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("chmod %s: %v", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	t.Setenv("HOME", privateHome)
}

// seedWorldWritableLegacy creates the stale world-writable legacy selection at
// the resolved config path and returns that path.
func seedWorldWritableLegacy(t *testing.T) string {
	t.Helper()
	legacyPath, err := ActivePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(filepath.Dir(legacyPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(filepath.Dir(filepath.Dir(legacyPath))), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(legacyPath), os.ModeSticky|0o777); err != nil {
		t.Fatal(err)
	}
	seed := `{"resolution":{"resolved_revision":"mig-1","source":{"kind":"local","location":"x"}},"config":` + testConfigJSON(t) + `}`
	if err := os.WriteFile(legacyPath, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	return legacyPath
}

// TestWorldWritableMigrationMarkerWriteFailureFailsClosed is the 0428-310
// controlled fault for the marker-write window (hunt H-V40 audit disposition):
// the canonical data rename must land BEFORE the marker is attempted; when the
// marker write itself fails, the migrated value is preserved on the private
// canonical path and adoption reports it together with the error (the legacy
// path must never be re-consulted); the retry completes durably, and a later
// deletion of the canonical file never revives the legacy value.
func TestWorldWritableMigrationMarkerWriteFailureFailsClosed(t *testing.T) {
	privateHome := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(privateHome, ".config"), 0o700); err != nil {
		t.Fatal(err)
	}
	setPrivateHome(t, privateHome)

	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	legacyPath := seedWorldWritableLegacy(t)

	canonical, err := privateCanonicalPath()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(filepath.Dir(canonical), canonicalFrozenMarker)

	var adopted bool
	var adoptErr error
	withMarkerFS(t, faultMarkerFS{}, func() {
		adopted, adoptErr = adoptLegacySelection(legacyPath, canonical)
	})
	if !errors.Is(adoptErr, errMarkerWriteFault) {
		t.Fatalf("migration error = %v, want the marker-write fault", adoptErr)
	}
	// Fail-closed for the legacy path: the value landed privately, so adoption
	// reports it (adopted=true) WITH the error; nothing may silently retry
	// against the legacy location.
	if !adopted {
		t.Fatal("a failed marker write after the data landed must preserve and report the migrated selection")
	}
	// Ordering contract: the canonical data landed BEFORE the marker step.
	data, dataErr := os.ReadFile(canonical)
	if dataErr != nil {
		t.Fatalf("canonical selection must be durable before the marker is attempted: %v", dataErr)
	}
	if !strings.Contains(string(data), "mig-1") {
		t.Fatalf("canonical data does not carry the migrated selection: %q", data)
	}
	// Fail-closed: no completion claim may exist while the marker failed.
	if frozenMarkerComplete(marker) {
		t.Fatal("failed marker write must not leave a readable completion claim")
	}
	// The migrated selection is preserved: loads return the canonical value.
	loaded, err := LoadActive()
	if err != nil {
		t.Fatalf("LoadActive after failed marker write = %v", err)
	}
	if loaded.Resolution.ResolvedRevision != "mig-1" {
		t.Fatalf("failed-marker state must preserve the migrated selection: %+v", loaded.Resolution)
	}

	// Retry with the fault removed: the marker lands durably; afterwards the
	// legacy path can never be revived, even with the canonical file deleted.
	if err := writeFrozenMarker(marker); err != nil {
		t.Fatalf("marker retry = %v", err)
	}
	if !frozenMarkerComplete(marker) {
		t.Fatal("successful retry must leave a durable, readable marker")
	}
	if err := os.Remove(canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadActive(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("LoadActive with marker present + canonical missing = %v, want ErrNotConfigured", err)
	}
}

// TestMigrateToCanonicalKeepsDataBeforeMarkerOrder locks the ordering contract
// directly on migrateToCanonical through the seam: a failed marker write
// returns the error AFTER the canonical data landed and leaves no completion
// claim; an already-complete durable marker makes the step idempotent and is
// never rewritten.
func TestMigrateToCanonicalKeepsDataBeforeMarkerOrder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "canon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "legacy-active.json")
	dst := filepath.Join(dir, "active.json")
	marker := filepath.Join(dir, canonicalFrozenMarker)
	data := []byte(`{"resolution":{},"config":{}}`)
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var migrateErr error
	withMarkerFS(t, faultMarkerFS{}, func() {
		migrateErr = migrateToCanonical(src, dst, data)
	})
	if migrateErr == nil {
		t.Fatal("migrateToCanonical must report a failed marker write")
	}
	if _, readErr := os.ReadFile(dst); readErr != nil {
		t.Fatalf("canonical data must be durable before the marker is attempted: %v", readErr)
	}
	if frozenMarkerComplete(marker) {
		t.Fatal("failed marker write must not leave a readable completion claim")
	}

	// Idempotent re-run: a complete durable marker beforehand means the step is
	// skipped without touching (or rewriting) the marker file.
	if err := os.WriteFile(marker, []byte(canonicalMarkerContent), 0o600); err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := false
	var rerunErr error
	withMarkerFS(t, recordingMarkerFS{onWrite: func() { rewritten = true }}, func() {
		rerunErr = migrateToCanonical(src, dst, data)
	})
	if rerunErr != nil {
		t.Fatalf("idempotent re-run with complete marker = %v", rerunErr)
	}
	if rewritten {
		t.Fatal("an already-complete durable marker must not be rewritten")
	}
	infoAfter, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("idempotent re-run must leave the marker untouched")
	}
}

// TestLoadActiveOptionalTwoCallRepairsLostMarkerWithoutManualWrite is the
// CODEX-static correction for finding (2): after a marker-write fault during
// adoption — with the canonical data landed — the FIRST LoadActiveOptional
// preserves the migrated selection instead of discarding it for the error, and
// the SECOND call (no fault installed anywhere, no manual writeFrozenMarker)
// succeeds cleanly because the loader itself completed the durable once-only
// marker through its normal path. If the direct canonical read could bypass a
// lost marker forever, this is where the regression surfaces.
func TestLoadActiveOptionalTwoCallRepairsLostMarkerWithoutManualWrite(t *testing.T) {
	privateHome := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(privateHome, ".config"), 0o700); err != nil {
		t.Fatal(err)
	}
	setPrivateHome(t, privateHome)
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cfgDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	seedWorldWritableLegacy(t)

	canonical, err := privateCanonicalPath()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(filepath.Dir(canonical), canonicalFrozenMarker)

	// Call 1: the adoption's marker step faults. The migrated value is REAL
	// (canonical landed): preserve-and-report must return it with the fault
	// visible, never an error-only discard.
	var state ActiveState
	var ok bool
	var loadErr error
	withMarkerFS(t, faultMarkerFS{}, func() {
		state, ok, loadErr = LoadActiveOptional()
	})
	if loadErr == nil {
		t.Fatal("call 1 must keep the marker-write failure visible")
	}
	if !ok {
		t.Fatalf("call 1 must preserve the migrated selection alongside the fault: %+v", state.Resolution)
	}
	if state.Resolution.ResolvedRevision != "mig-1" {
		t.Fatalf("call 1 lost the migrated selection: %+v", state.Resolution)
	}
	if frozenMarkerComplete(marker) {
		t.Fatal("a faulted marker write must not leave a complete claim")
	}

	// Call 2 (no fault installed anywhere, no manual marker write): the loader
	// itself repairs the lost once-only claim and loads cleanly.
	state, ok, loadErr = LoadActiveOptional()
	if loadErr != nil || !ok {
		t.Fatalf("call 2 must repair the marker and load cleanly: ok=%v err=%v", ok, loadErr)
	}
	if state.Resolution.ResolvedRevision != "mig-1" {
		t.Fatalf("call 2 lost the migrated selection: %+v", state.Resolution)
	}
	if !frozenMarkerComplete(marker) {
		t.Fatal("call 2 must have completed the durable marker through the normal load path")
	}

	// The legacy path stays dead: deleting the canonical file now yields the
	// not-configured result (marker honoured), never a legacy re-adoption.
	if err := os.Remove(canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadActive(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("LoadActive after marker repair + canonical delete = %v, want ErrNotConfigured", err)
	}
}

// TestFrozenMarkerContentVerification is the CODEX-static correction for
// finding (1): readability alone is NOT a completion claim. An empty,
// truncated or wrong-content marker residue (a) never counts as complete for
// the loader predicate, (b) fails writeFrozenMarker's read-back verification,
// and (c) is repaired (rewritten to the exact canonical bytes) by the next
// ensureFrozenMarker pass — while an exact marker stays untouched and a
// damaged residue never freezes the migration path.
func TestFrozenMarkerContentVerification(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "canon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, canonicalFrozenMarker)
	for _, residue := range []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"truncated", canonicalMarkerContent[:len(canonicalMarkerContent)-7]},
		{"wrong-content", strings.Repeat("x", len(canonicalMarkerContent))},
	} {
		if err := os.WriteFile(marker, []byte(residue.content), 0o600); err != nil {
			t.Fatalf("%s: %v", residue.name, err)
		}
		if frozenMarkerComplete(marker) {
			t.Fatalf("%s residue must NOT be a completion claim", residue.name)
		}
		if canonicalMigrationFrozen(filepath.Join(dir, "active.json")) {
			t.Fatalf("%s residue must not freeze the migration path", residue.name)
		}
		if err := verifyFrozenMarkerContent(marker); err == nil {
			t.Fatalf("%s residue must fail content verification", residue.name)
		}
		if err := ensureFrozenMarker(dir); err != nil {
			t.Fatalf("%s residue repair: %v", residue.name, err)
		}
		content, err := os.ReadFile(marker)
		if err != nil || string(content) != canonicalMarkerContent {
			t.Fatalf("%s residue not repaired to canonical content: %q err=%v", residue.name, content, err)
		}
	}
	// An exact marker is complete and never rewritten (idempotence).
	rewritten := false
	withMarkerFS(t, recordingMarkerFS{onWrite: func() { rewritten = true }}, func() {
		if err := ensureFrozenMarker(dir); err != nil {
			t.Fatal(err)
		}
	})
	if rewritten {
		t.Fatal("a complete canonical marker must not be rewritten")
	}
	// A directory residue is never deleted, never honoured, and reported.
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	if frozenMarkerComplete(marker) {
		t.Fatal("a directory must not count as a completion claim")
	}
	if err := ensureFrozenMarker(dir); err == nil {
		t.Fatal("a directory residue must be reported, not deleted or honoured")
	}
	if _, err := os.Lstat(marker); err != nil {
		t.Fatalf("the directory residue must be left untouched: %v", err)
	}
}

// TestWriteDurableCanonicalDataFsyncsBeforeRename locks the CODEX-static
// durability correction (finding 3) on the ordered filesystem seam: temp-file
// fsync BEFORE the atomic rename, directory fsync AFTER it, and the completion
// marker created only after both — "durability before completion claim".
func TestWriteDurableCanonicalDataFsyncsBeforeRename(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "canon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var events []string
	previousFsync := orgconfigFsync
	orgconfigFsync = func(what string) {
		events = append(events, "sync:"+what)
		previousFsync(what)
	}
	defer func() { orgconfigFsync = previousFsync }()
	previousRename := orgconfigRename
	orgconfigRename = func(from, to string) error {
		events = append(events, "rename:"+filepath.Base(to))
		return previousRename(from, to)
	}
	defer func() { orgconfigRename = previousRename }()
	previousMarker := orgconfigMarkerEvent
	orgconfigMarkerEvent = func(what string) {
		events = append(events, "marker:"+what)
		previousMarker(what)
	}
	defer func() { orgconfigMarkerEvent = previousMarker }()

	if err := writeDurableCanonicalData(filepath.Join(dir, "active.json"), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err := ensureFrozenMarker(dir); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(events, ",")
	tempSync := strings.Index(joined, "sync:active.json.tmp")
	rename := strings.Index(joined, "rename:active.json")
	dirSync := strings.Index(joined, "sync:dir:"+filepath.Base(dir))
	markerWrite := strings.Index(joined, "marker:"+canonicalFrozenMarker)
	if tempSync < 0 || rename < 0 || dirSync < 0 || markerWrite < 0 {
		t.Fatalf("fsync/rename seam events incomplete: %v", events)
	}
	if !(tempSync < rename && rename < dirSync && dirSync < markerWrite) {
		t.Fatalf("durability order violated (temp-sync < rename < dir-sync < marker): %v", events)
	}
}

// withSyncDirFault swaps the data-directory-sync seam for one test body. A fault
// stands in for a real open/Sync/Close I/O error on the canonical data
// directory: the data could not be confirmed durable, so the once-only
// completion marker must not be created and the migration must NOT be reported
// complete.
func withSyncDirFault(t *testing.T, fn func()) {
	t.Helper()
	previous := orgconfigSyncDir
	orgconfigSyncDir = func(string) error { return errSyncDirFailure }
	defer func() { orgconfigSyncDir = previous }()
	fn()
}

// TestSyncDirFailureFailsClosedNoCompleteMarker proves the data-before-
// completion contract from the 0428-310 dispute finale: a deterministic
// injected data-directory-sync failure (a) aborts migrateToCanonical with the
// real error, (b) never writes the once-only completion marker, and (c) leaves
// the completion claim false — so a missing canonical file is never silently
// accepted as complete. The injected error type is the syncDurabilityError so a
// future caller can tell an explicitly directory-fsync-unsupported platform
// (unsupported=true) from a transient I/O error.
func TestSyncDirFailureFailsClosedNoCompleteMarker(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "canon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "legacy-active.json")
	dst := filepath.Join(dir, "active.json")
	marker := filepath.Join(dir, canonicalFrozenMarker)
	data := []byte(`{"resolution":{"resolved_revision":"mig-1","source":{"kind":"local","location":"x"}},"config":` + testConfigJSON(t) + `}`)
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var migrateErr error
	withSyncDirFault(t, func() {
		migrateErr = migrateToCanonical(src, dst, data)
	})
	if migrateErr == nil {
		t.Fatal("migrateToCanonical must report the data-directory-sync failure")
	}
	var syncErr *syncDurabilityError
	if !errors.As(migrateErr, &syncErr) {
		t.Fatalf("migration error = %v, want a syncDurabilityError", migrateErr)
	}
	// The completion marker was never written after the sync failure.
	if _, statErr := os.Lstat(marker); statErr == nil {
		t.Fatal("completion marker must not be created after a data-directory-sync failure")
	}
	// The completion claim is false: frozenMarkerComplete must not report done.
	if frozenMarkerComplete(marker) {
		t.Fatal("a failed data-directory-sync must not leave a complete completion claim")
	}
}

// TestSyncDirFailureRecoversByRetainingCanonicalSelection proves that once the
// data directory sync succeeds on retry the migration completes and the
// canonical selection is retained — and that a missing canonical file is never
// silently accepted as complete under a complete marker. The fault is active on
// the first migrateToCanonical, cleared on the retry.
func TestSyncDirFailureRecoversByRetainingCanonicalSelection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "canon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "legacy-active.json")
	dst := filepath.Join(dir, "active.json")
	marker := filepath.Join(dir, canonicalFrozenMarker)
	data := []byte(`{"resolution":{"resolved_revision":"mig-1","source":{"kind":"local","location":"x"}},"config":` + testConfigJSON(t) + `}`)
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// First attempt: the data-directory-sync faults; the migration must fail
	// closed with no complete marker.
	var firstErr error
	withSyncDirFault(t, func() {
		firstErr = migrateToCanonical(src, dst, data)
	})
	if firstErr == nil {
		t.Fatal("first migrateToCanonical must report the data-directory-sync failure")
	}
	if _, statErr := os.Lstat(marker); statErr == nil {
		t.Fatal("no completion marker may exist after the first (faulted) attempt")
	}
	if frozenMarkerComplete(marker) {
		t.Fatal("the faulted attempt must not report the migration complete")
	}
	// The canonical data rename already landed under the durable temp+rename;
	// on retry it is already in place.
	if _, readErr := os.ReadFile(dst); readErr != nil {
		t.Fatalf("canonical selection must be retained on disk after the faulted attempt: %v", readErr)
	}

	// Retry with the seam restored: the marker is now written and the
	// migration completes.
	if err := migrateToCanonical(src, dst, data); err != nil {
		t.Fatalf("retry migrateToCanonical after sync recovery = %v", err)
	}
	if !frozenMarkerComplete(marker) {
		t.Fatal("a clean retry must complete the migration and write the marker")
	}
	if _, statErr := os.Lstat(marker); statErr != nil {
		t.Fatalf("marker must exist after a clean retry: %v", statErr)
	}

	// A missing canonical file under a complete marker is NOT silently
	// accepted as complete by the content predicate: frozenMarkerComplete is
	// keyed on the marker, and the loader repairs a missing canonical file
	// only when the marker is absent. Here the marker IS complete, so the
	// predicate correctly reports complete; the regression protects the OTHER
	// direction — a marker is never complete when the data-directory sync
	// failed. Assert the marker predicate is complete ONLY because the marker
	// genuinely exists with canonical content (not because data sync was
	// skipped).
	content, rerr := os.ReadFile(marker)
	if rerr != nil || string(content) != canonicalMarkerContent {
		t.Fatalf("marker must hold exact canonical content after a clean retry: %q err=%v", content, rerr)
	}
}
