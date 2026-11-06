package coreinstallation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// seedInstallation provisions a genuine Core installation at root, which the
// management-host attach tests reuse. Run creates the private state root itself.
func seedInstallation(t *testing.T, root string) State {
	t.Helper()
	state, err := Run(context.Background(), root, Spec{
		Target:      "core",
		Runtime:     "docker",
		MachineRole: Development,
	}, Steps{
		Preflight: func(ctx context.Context, s State) error { return nil },
		SQL:       func(ctx context.Context, s State) error { return nil },
		Secrets:   func(ctx context.Context, s State) error { return nil },
		Identity:  func(ctx context.Context, s State) (string, error) { return "https://identity.example/v1", nil },
		Verify:    func(ctx context.Context, s State) error { return nil },
	})
	if err != nil {
		t.Fatalf("seed install at %s: %v", root, err)
	}
	if !state.Ready {
		t.Fatalf("seeded installation not ready")
	}
	return state
}

// TestManagementHostBootstrapWhenRootMissing proves that a durable state root
// that does not yet exist is a bootstrap, not an error: the caller may then
// provision a fresh durable installation on the host.
func TestManagementHostBootstrapWhenRootMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "empty-root")
	host, err := NewManagementHost(root)
	if err != nil {
		t.Fatalf("NewManagementHost: %v", err)
	}
	bootstrap, unlock, verr := host.Verify()
	// A fresh durable state root is a bootstrap: exactly one process may hold
	// the atomic bootstrap lock that precedes ownership. Release it here; the
	// caller provisions the fresh installation while the lock is held in the
	// real serve path.
	if verr != nil {
		if unlock != nil {
			unlock()
		}
		t.Fatalf("Verify on missing root = %v, want nil (bootstrap)", verr)
	}
	if !bootstrap {
		t.Fatalf("Verify on missing root: bootstrap = false, want true")
	}
	if unlock != nil {
		unlock()
	}
}

// TestManagementHostAttachWhenValidRoot proves that a durable state root holding
// a genuine Core installation attaches (bootstrap == false) and hands the caller
// the active-writer lifecycle lock.
func TestManagementHostAttachWhenValidRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owned-root")
	seedInstallation(t, root)
	host, err := NewManagementHost(root)
	if err != nil {
		t.Fatalf("NewManagementHost: %v", err)
	}
	bootstrap, unlock, verr := host.Verify()
	// Attach hands the lifecycle lock; release it.
	if verr != nil {
		if unlock != nil {
			unlock()
		}
		t.Fatalf("Verify on valid root = %v, want nil (attach)", verr)
	}
	if bootstrap {
		t.Fatalf("Verify on valid root: bootstrap = true, want false (attach)")
	}
}

// TestManagementHostForeignRootFailsClosed proves that a durable state root
// holding an installation that does not belong to this host selection fails
// closed with a named sentinel and selects no local fallback.
func TestManagementHostForeignRootFailsClosed(t *testing.T) {
	// A root that holds a non-baseharbor or incompatible installation is foreign.
	root := filepath.Join(t.TempDir(), "foreign-root")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Write an incompatible installation.json (wrong owner) at the root.
	bad := `{"version":"baseharbor.core-installation/v1","installation_id":"00000000-0000-0000-0000-000000000000","owner":"other","spec":{"target":"core","runtime":"docker","machine_role":"development","ha":false},"defaults":{"local_workspaces":true,"explicit_sources":true,"protected_management":true},"placement":"shared","phase":"ready","ready":true,"capabilities":{"sql":true,"secrets":true,"identity":true}}`
	if err := os.WriteFile(filepath.Join(root, "installation.json"), []byte(bad), 0600); err != nil {
		t.Fatal(err)
	}
	host, err := NewManagementHost(root)
	if err != nil {
		t.Fatalf("NewManagementHost: %v", err)
	}
	_, unlock, verr := host.Verify()
	if unlock != nil {
		unlock()
	}
	if verr != ErrManagementHostRootForeign {
		t.Fatalf("Verify on foreign root = %v, want ErrManagementHostRootForeign", verr)
	}
}

// TestManagementHostProtectedRootFailsClosed proves that a durable state root
// that is not private (group/world accessible) fails closed, because the single
// durable authority must be owned privately.
func TestManagementHostProtectedRootFailsClosed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "protected-root")
	seedInstallation(t, root)
	// Expose the root to group/world; the durable authority must refuse.
	if err := os.Chmod(root, 0750); err != nil {
		t.Fatal(err)
	}
	host, err := NewManagementHost(root)
	if err != nil {
		t.Fatalf("NewManagementHost: %v", err)
	}
	_, unlock, verr := host.Verify()
	if unlock != nil {
		unlock()
	}
	if verr != ErrManagementHostRootProtected {
		t.Fatalf("Verify on unprotected root = %v, want ErrManagementHostRootProtected", verr)
	}
}

// TestManagementHostEmptyRootFailsClosed proves that an empty state root string
// is rejected: a management host must have exactly one durable state root.
func TestManagementHostEmptyRootFailsClosed(t *testing.T) {
	host, err := NewManagementHost("   ")
	if err == nil {
		t.Fatalf("NewManagementHost(empty) = nil err, want error")
	}
	_ = host
}

// TestManagementHostActiveWriterExcludesSecondProcess is the real two-process
// active-writer cell: a second OS process (a re-exec of this test binary) enters
// the same filesystem seam (ManagementHost.Verify on the same root) and must be
// rejected with ErrManagementHostAlreadyOwned while the first process still holds
// the lifecycle lock, and admitted after the holder releases it. The holder
// signals acquire/release through marker files so the sequence is deterministic,
// not timing-based.
func TestManagementHostActiveWriterExcludesSecondProcess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owned-root")
	seedInstallation(t, root)
	markerDir := t.TempDir()
	held := filepath.Join(markerDir, "held")
	released := filepath.Join(markerDir, "released")

	// Process 1: hold the durable management-host lock for this root.
	_, holderRelease, err := (&ManagementHost{stateRoot: root}).Verify()
	if err != nil {
		t.Fatalf("holder Verify: %v", err)
	}
	if err := os.WriteFile(held, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Process 2: a separate process must be excluded on the real filesystem seam.
	second := exec.Command(os.Args[0])
	second.Env = append(os.Environ(),
		managementHostHelperEnv+"=1",
		managementHostHelperChildEnv+"=1",
		"BAHA_TEST_MH_ROOT="+root,
		"BAHA_TEST_MH_RELEASE="+released,
	)
	out, err := second.CombinedOutput()
	if err == nil {
		t.Fatalf("second process owned the management host while the first process held it; output:\n%s", out)
	}
	if !strings.Contains(string(out), "management host already owned") {
		t.Fatalf("second process failed for the wrong reason; want the active-writer refusal, got:\n%s", out)
	}

	// Release and re-run: the durable root must now attach for a separate process.
	if err := os.WriteFile(released, nil, 0600); err != nil {
		t.Fatal(err)
	}
	holderRelease()
	holderRelease = nil
	third := exec.Command(os.Args[0])
	third.Env = append(os.Environ(),
		managementHostHelperEnv+"=1",
		managementHostHelperAcquireEn+"=1",
		"BAHA_TEST_MH_ROOT="+root,
	)
	if out, err := third.CombinedOutput(); err != nil {
		t.Fatalf("separate process rejected after the holder released; output:\n%s", out)
	}
}
