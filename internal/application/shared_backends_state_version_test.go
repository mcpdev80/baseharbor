package application

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func sharedBackendStateVersionString(n int) string { return strconv.Itoa(n) }

// TestSharedBackendStateVersionMismatchNamesVersions ensures the human surface
// gets a distinct, upgrade-from-outside message: it names the version found and
// the supported version and points at the supported next step, and it is
// distinguishable from a corrupt or unreadable state file.
func TestSharedBackendStateVersionMismatchNamesVersions(t *testing.T) {
	for _, found := range []int{1, 2, 99} {
		data := `{"version":` + strconv.Itoa(found) + `,"environment":"dev"}`
		_, err := decodeSharedBackendState([]byte(data))
		if err == nil {
			t.Fatalf("version %d must fail closed", found)
		}
		var mismatch *SharedBackendStateVersionMismatch
		if !errors.As(err, &mismatch) {
			t.Fatalf("version %d did not return SharedBackendStateVersionMismatch: %v", found, err)
		}
		if mismatch.Found != found {
			t.Fatalf("mismatch.Found = %d, want %d", mismatch.Found, found)
		}
		if mismatch.Expected != sharedBackendStateVersion {
			t.Fatalf("mismatch.Expected = %d, want %d", mismatch.Expected, sharedBackendStateVersion)
		}
		msg := err.Error()
		if !strings.Contains(msg, "upgrade-from-outside") {
			t.Fatalf("message must signal upgrade-from-outside: %q", msg)
		}
		if !strings.Contains(msg, strconv.Itoa(found)) || !strings.Contains(msg, strconv.Itoa(sharedBackendStateVersion)) {
			t.Fatalf("message must name found and expected version: %q", msg)
		}
	}
}

// TestSharedBackendStateVersionMismatchIsDistinguishableFromCorruption ensures
// IsSharedBackendStateVersionMismatch is true only for the version gate, so a
// caller can tell it apart from a corrupt or unreadable file.
func TestSharedBackendStateVersionMismatchIsDistinguishableFromCorruption(t *testing.T) {
	_, err := decodeSharedBackendState([]byte(`{"version":2}`))
	if err == nil {
		t.Fatal("version gate must report as version mismatch")
	}
	if !IsSharedBackendStateVersionMismatch(err) {
		t.Fatal("version gate must report as version mismatch")
	}
	// A parse failure is corruption, not a version mismatch.
	_, err = decodeSharedBackendState([]byte(`not json`))
	if IsSharedBackendStateVersionMismatch(err) {
		t.Fatal("corrupt state must not be reported as version mismatch")
	}
	if IsSharedBackendStateVersionMismatch(nil) {
		t.Fatal("nil must not be a version mismatch")
	}
}

// TestSharedBackendStateLoadReportsVersionReason runs the real load path over a
// v2 state written to disk and asserts the version reason (not a generic error)
// reaches the caller, and that nothing on disk was migrated.
func TestSharedBackendStateLoadReportsVersionReason(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	v2 := `{"version":2,"environment":"dev"}`
	if err := os.WriteFile(statePath, []byte(v2), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadSharedBackendState(statePath, "dev")
	if err == nil {
		t.Fatal("v2 state must fail closed")
	}
	var mismatch *SharedBackendStateVersionMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected version mismatch, got %v", err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("load must not rewrite the provider state (fail-closed stays byte-identical)")
	}
}

// TestSharedBackendStateCorrectVersionStillLoads keeps the happy path green so
// the version gate cannot drift into a silent accept.
func TestSharedBackendStateCorrectVersionStillLoads(t *testing.T) {
	data := `{"version":` + strconv.Itoa(sharedBackendStateVersion) + `,"environment":"dev"}`
	state, err := decodeSharedBackendState([]byte(data))
	if err != nil {
		t.Fatalf("supported version must load: %v", err)
	}
	if state.Version != sharedBackendStateVersion {
		t.Fatalf("version = %d, want %d", state.Version, sharedBackendStateVersion)
	}
}
