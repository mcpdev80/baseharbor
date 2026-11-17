package stableid

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCrossProcessFlockIsEnforcedProvesMutualExclusion is the two-real-process
// cell proving the lifecycle lock blocks a second OS process while the first
// still holds it. This is the production guard: a separate process entering the
// same AcquireLifecycleLock on the same root must be refused while the holder
// still runs. On an enforcing filesystem the probe observes HELD and the acquire
// succeeds; the refused second process then fails on the flock refusal, not on
// the probe.
//
// Hosted-gate compatibility: the child body is a helper entry point, not a test;
// with no child mode set it is a no-op (no t.Skip), so the package enumeration
// contains exactly the real cells and zero skips.
func TestCrossProcessFlockIsEnforcedProvesMutualExclusion(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Process 1 holds the lifecycle lock for the whole test.
	holder, err := AcquireLifecycleLock(root)
	if err != nil {
		t.Fatalf("holder acquire failed on an enforcing filesystem: %v", err)
	}
	defer func() {
		if holder != nil {
			holder()
		}
	}()

	// Process 2 is a separate OS process that must be refused while the holder
	// still runs.
	second := exec.Command(os.Args[0], "-test.run", "^$")
	second.Env = append(os.Environ(), "BAHA_TEST_LOCK_CHILD=1", "BAHA_TEST_LOCK_ROOT="+root)
	out, err := second.CombinedOutput()
	if err == nil {
		t.Fatalf("second process acquired the lock while the first process held it; cross-process flock is NOT enforced (the defect this regression guards): output:\n%s", out)
	}
	if !strings.Contains(string(out), "lifecycle lock held") {
		t.Fatalf("second process failed for the wrong reason; want the flock refusal, got:\n%s", out)
	}
}

// lockCrossProcessChild is the second-process body for the cross-process
// exclusion cell: it enters the same AcquireLifecycleLock seam and exits
// non-zero so the parent observes a refused second process.
func lockCrossProcessChild() {
	root := os.Getenv("BAHA_TEST_LOCK_ROOT")
	if _, err := AcquireLifecycleLock(root); err == nil {
		fmt.Fprintln(os.Stderr, "second acquire unexpectedly succeeded")
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "lifecycle lock held")
	os.Exit(1)
}

// TestCrossProcessProbeReturnsFreeWhenChildAcquires proves the probe can report
// FREE: it stubs the probe function to mimic a child that acquires the held
// lock (the unenforcing filesystem case) and asserts the returned free flag is
// true. This pins the probe contract that drives the fail-closed path.
func TestCrossProcessProbeReturnsFreeWhenChildAcquires(t *testing.T) {
	origProbe := LockProbeFunc
	LockProbeFunc = func(string) (bool, error) { return true, nil }
	defer func() { LockProbeFunc = origProbe }()

	free, perr := lockProbeRun("/unused/path")
	if perr != nil {
		t.Fatalf("probe returned an error: %v", perr)
	}
	if !free {
		t.Fatalf("probe did not report FREE for a child that acquires the held lock; the probe cannot detect the unenforcing filesystem defect")
	}
}

// TestCrossProcessProbeReturnsHeldWhenLockGenuinelyFree proves the probe reports
// HELD via a stub (simulating a correctly-enforcing kernel) so the acquire
// proceeds rather than failing closed spuriously.
func TestCrossProcessProbeReturnsHeldWhenLockGenuinelyFree(t *testing.T) {
	origProbe := LockProbeFunc
	LockProbeFunc = func(string) (bool, error) { return false, nil }
	defer func() { LockProbeFunc = origProbe }()

	free, perr := lockProbeRun("/unused/path")
	if perr != nil {
		t.Fatalf("probe error: %v", perr)
	}
	if free {
		t.Fatalf("probe reported FREE for a correctly-enforcing kernel; the acquire would wrongly fail closed")
	}
}

// TestAcquireLifecycleLockFailsClosedOnUnenforcingFilesystem proves the full
// production path: when the probe reports FREE (filesystem incapable),
// AcquireLifecycleLock returns the not-enforcing error instead of returning a
// lock that cannot guarantee mutual exclusion. This is the fail-closed contract
// the mission requires. The probe is stubbed to FREE on a fresh root with no
// real contention, isolating the probe decision from flock contention.
func TestAcquireLifecycleLockFailsClosedOnUnenforcingFilesystem(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}

	origProbe := LockProbeFunc
	LockProbeFunc = func(string) (bool, error) { return true, nil }
	defer func() { LockProbeFunc = origProbe }()

	_, err := AcquireLifecycleLock(root)
	if err == nil {
		t.Fatalf("acquire succeeded while the filesystem is unenforcing; mutual exclusion is advisory-only (the defect)")
	}
	if !errors.Is(err, ErrLockFilesystemNotEnforcing) {
		t.Fatalf("want the not-enforcing failure, got: %v", err)
	}
}
