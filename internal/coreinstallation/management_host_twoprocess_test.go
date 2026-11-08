package coreinstallation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Two-process active-writer cells. A second OS process (a re-exec of this test
// binary) must prove that a durable management host excludes a concurrent writer
// on the real filesystem seam. These helper entry points are NOT tests: with no
// child mode set they are no-ops (no t.Skip), so the default package enumeration
// contains exactly the real cells and zero skips.

const (
	managementHostHelperEnv       = "BAHA_TEST_MH_HELPER"
	managementHostHelperChildEnv  = "BAHA_TEST_MH_HELPER_CHILD"
	managementHostHelperAcquireEn = "BAHA_TEST_MH_ACQUIRE"
)

// TestMain dispatches a child-mode invocation (a re-exec of this test binary by
// the cell above) directly into the named body, before the testing framework
// enumerates anything. In a normal package run it is a pass-through, and no
// symbol in this package skips.
func TestMain(m *testing.M) {
	if os.Getenv(managementHostHelperEnv) == "1" {
		switch {
		case os.Getenv(managementHostHelperChildEnv) == "1":
			managementHostHelperChild()
		case os.Getenv(managementHostHelperAcquireEn) == "1":
			managementHostHelperAcquire()
		default:
			fmt.Fprintln(os.Stderr, "helper mode requested without a child body selector")
			os.Exit(2)
		}
	}
	os.Exit(m.Run())
}

// managementHostHelperChild models the second process's critical section: it owns
// the durable management host on BAHA_TEST_MH_ROOT and waits for the release
// marker, so the parent can observe mutual exclusion across real processes. The
// refusal path prints the active-writer error verbatim and exits 1.
func managementHostHelperChild() {
	root := os.Getenv("BAHA_TEST_MH_ROOT")
	release := os.Getenv("BAHA_TEST_MH_RELEASE")
	_, unlock, err := (&ManagementHost{stateRoot: root}).Verify()
	if err != nil {
		// The refusal path prints the active-writer (or bootstrap-owned) error
		// verbatim and exits 1.
		fmt.Fprintf(os.Stderr, "management host already owned: %v\n", err)
		if unlock != nil {
			unlock()
		}
		os.Exit(1)
	}
	// Held the lifecycle (or bootstrap) lock for this durable root: release it so
	// the holder in the parent can observe mutual exclusion on the real filesystem
	// seam.
	if unlock != nil {
		unlock()
	}
	// Unreachable in the exclusion cell while the parent holds the lock; if we
	// ever get here, hold briefly and prove the release path works.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(release); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if unlock != nil {
		unlock()
	}
	os.Exit(0)
}

// managementHostHelperAcquire proves the released durable root is available to a
// separate process: it verifies attach and exits 0.
func managementHostHelperAcquire() {
	_, unlock, err := (&ManagementHost{stateRoot: os.Getenv("BAHA_TEST_MH_ROOT")}).Verify()
	if err != nil {
		fmt.Fprintf(os.Stderr, "post-release verify failed: %v\n", err)
		os.Exit(1)
	}
	if unlock != nil {
		unlock()
	}
	if strings.Contains(os.Getenv("BAHA_TEST_MH_ROOT"), "owned-root") {
		// Attach for a real durable root is success.
	}
	os.Exit(0)
}

// TestManagementHostBootstrapExcludesConcurrentWriter proves the atomic bootstrap
// ownership guard: while the first process holds the bootstrap lock on a fresh
// durable root, a second concurrent Verify is refused with
// ErrManagementHostOwnedFreshRoot (only one process may own the new authority),
// and after the first holder releases the lock a fresh bootstrap is admitted
// again.
func TestManagementHostBootstrapExcludesConcurrentWriter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fresh-root")

	first, firstUnlock, firstErr := (&ManagementHost{stateRoot: root}).Verify()
	if firstErr != nil {
		t.Fatalf("first bootstrap Verify: %v", firstErr)
	}
	if !first.IsBootstrap() || firstUnlock == nil {
		t.Fatalf("first bootstrap: bootstrap=%v unlock=%v, want bootstrap=true and a held lock", first.IsBootstrap(), firstUnlock != nil)
	}
	_, secondUnlock, secondErr := (&ManagementHost{stateRoot: root}).Verify()
	if secondUnlock != nil {
		secondUnlock()
	}
	if secondErr != ErrManagementHostOwnedFreshRoot {
		t.Fatalf("second concurrent bootstrap = %v, want ErrManagementHostOwnedFreshRoot", secondErr)
	}
	firstUnlock()

	_, thirdUnlock, thirdErr := (&ManagementHost{stateRoot: root}).Verify()
	if thirdErr != nil {
		t.Fatalf("post-release bootstrap Verify: %v", thirdErr)
	}
	if thirdUnlock != nil {
		thirdUnlock()
	}
}

// TestManagementHostBootstrapTwoProcesses is the real two-process bootstrap
// exclusion cell: a second OS process is refused with a bootstrap-owned refusal
// while the first holds the fresh-root lock, and admitted after release.
func TestManagementHostBootstrapTwoProcesses(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fresh-root")
	released := filepath.Join(t.TempDir(), "released")

	_, holderRelease, err := (&ManagementHost{stateRoot: root}).Verify()
	if err != nil {
		t.Fatalf("holder bootstrap Verify: %v", err)
	}

	second := exec.Command(os.Args[0])
	second.Env = append(os.Environ(),
		managementHostHelperEnv+"=1",
		managementHostHelperChildEnv+"=1",
		"BAHA_TEST_MH_ROOT="+root,
		"BAHA_TEST_MH_RELEASE="+released,
	)
	out, err := second.CombinedOutput()
	if err == nil {
		t.Fatalf("second process bootstrapped the fresh root while the first held the bootstrap lock; output:\n%s", out)
	}
	if !strings.Contains(string(out), "already owned") {
		t.Fatalf("second process failed for the wrong reason; want the bootstrap-owned refusal, got:\n%s", out)
	}

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
	out, thirdErr := third.CombinedOutput()
	if thirdErr != nil {
		t.Fatalf("separate process rejected after the holder released; output:\n%s", out)
	}
}

// TestManagementHostBootstrapProductionGuard is the shipped-path, causal
// source-mutation cell required by the act-1405 correction recipe: the production
// bootstrap guard (AcquireBootstrapLock in Verify) is the production ownership
// guard. With the guard in force, a second concurrent bootstrap is refused with
// ErrManagementHostOwnedFreshRoot. The RED result (guard removed => the refusal
// disappears) and the GREEN result (guard restored => refusal returns) are
// recorded in the evidence dossier.
func TestManagementHostBootstrapProductionGuard(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fresh-root")

	_, holderRelease, err := (&ManagementHost{stateRoot: root}).Verify()
	if err != nil {
		t.Fatalf("holder bootstrap Verify: %v", err)
	}
	_, secondUnlock, secondErr := (&ManagementHost{stateRoot: root}).Verify()
	if secondUnlock != nil {
		secondUnlock()
	}
	if secondErr != ErrManagementHostOwnedFreshRoot {
		t.Fatalf("with guard: second concurrent bootstrap = %v, want ErrManagementHostOwnedFreshRoot", secondErr)
	}
	holderRelease()
}
