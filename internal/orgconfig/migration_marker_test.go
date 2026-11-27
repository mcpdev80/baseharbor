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
