package stableid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLifecycleLockExcludesTwoSeparateProcesses is the real two-process lock
// cell required by t-20261018-r27-mutation-lock. It is NOT a goroutine test:
// a second OS process (a re-exec of this test binary) enters the same
// filesystem seam (AcquireLifecycleLock on the same root) and must be
// rejected while the first process still holds the lock, and admitted only
// after the holder releases it. The holder signals acquire/release through
// marker files so the sequence is deterministic, not timing-based.
//
// Hosted-gate compatibility: the authority-package-tests validation owner
// (scripts/pr-check-runner.py, go_package_tests) fails an authority package
// whose run contains ANY skipped test. The two child bodies are helper entry
// points, not tests: with no child mode set they are no-ops (no t.Skip), and
// TestMain below dispatches a child-mode invocation directly into the named
// body and exits, so the default package enumeration contains exactly the
// real cells and zero skips, while the two-process exclusion stays a real
// OS-process contract (re-exec, no goroutine substitution).

const (
	lifecycleLockHelperEnv       = "BAHA_TEST_LOCK_HELPER"
	lifecycleLockHelperChildEnv  = "BAHA_TEST_LOCK_HELPER_CHILD"
	lifecycleLockHelperAcquireEn = "BAHA_TEST_LOCK_HELPER_ACQUIRE"

	lifecycleLockCrossChildEnv = "BAHA_TEST_LOCK_CHILD"
)

func TestLifecycleLockExcludesTwoSeparateProcesses(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	markerDir := t.TempDir()
	held := filepath.Join(markerDir, "held")
	released := filepath.Join(markerDir, "released")

	// Process 1: this goroutine models the first process's critical section.
	holderRelease, err := AcquireLifecycleLock(root)
	if err != nil {
		t.Fatalf("holder acquire: %v", err)
	}
	if err := os.WriteFile(held, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Process 2: a separate process must be excluded on the real filesystem seam.
	second := exec.Command(os.Args[0])
	second.Env = append(os.Environ(),
		lifecycleLockHelperEnv+"=1",
		lifecycleLockHelperChildEnv+"=1",
		"BAHA_TEST_LOCK_ROOT="+root,
		"BAHA_TEST_LOCK_RELEASE="+released,
	)
	out, err := second.CombinedOutput()
	if err == nil {
		t.Fatalf("second process entered the lifecycle lock while the first process held it; output:\n%s", out)
	}
	if !strings.Contains(string(out), "lifecycle lock held") {
		t.Fatalf("second process failed for the wrong reason; want the flock refusal, got:\n%s", out)
	}

	// Release and re-run: the lock must now be admitted by a separate process.
	if err := os.WriteFile(released, nil, 0600); err != nil {
		t.Fatal(err) // signal the child to exit its hold section
	}
	holderRelease()
	holderRelease = nil
	third := exec.Command(os.Args[0])
	third.Env = append(os.Environ(),
		lifecycleLockHelperEnv+"=1",
		lifecycleLockHelperAcquireEn+"=1",
		"BAHA_TEST_LOCK_ROOT="+root,
	)
	if out, err := third.CombinedOutput(); err != nil {
		t.Fatalf("separate process rejected after the holder released; output:\n%s", out)
	}
}

// lockHelperChild is the second-process body: while it runs, the process holds
// the lifecycle lock on BAHA_TEST_LOCK_ROOT and waits for the release marker,
// so the parent can observe mutual exclusion across real processes. The
// refusal path prints the flock error verbatim and exits 1.
func lockHelperChild() {
	root := os.Getenv("BAHA_TEST_LOCK_ROOT")
	release := os.Getenv("BAHA_TEST_LOCK_RELEASE")
	unlock, err := AcquireLifecycleLock(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lifecycle lock held: %v\n", err)
		os.Exit(1)
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
	unlock()
	fmt.Fprintln(os.Stderr, "child acquired after release")
	os.Exit(0)
}

// lockHelperChildAcquire proves the released lock is available to a separate
// process: it acquires, releases and exits 0.
func lockHelperChildAcquire() {
	unlock, err := AcquireLifecycleLock(os.Getenv("BAHA_TEST_LOCK_ROOT"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "post-release acquire failed: %v\n", err)
		os.Exit(1)
	}
	unlock()
	os.Exit(0)
}

// TestMain dispatches a child-mode invocation (re-exec of this test binary by
// the cell above) directly into the named body, before the testing framework
// enumerates anything. In a normal package run it is a pass-through, and no
// symbol in this package skips.
func TestMain(m *testing.M) {
	if os.Getenv(lifecycleLockHelperEnv) == "1" {
		switch {
		case os.Getenv(lifecycleLockHelperChildEnv) == "1":
			lockHelperChild()
		case os.Getenv(lifecycleLockHelperAcquireEn) == "1":
			lockHelperChildAcquire()
		default:
			fmt.Fprintln(os.Stderr, "helper mode requested without a child body selector")
			os.Exit(2)
		}
	}
	if os.Getenv(lifecycleLockCrossChildEnv) == "1" {
		lockCrossProcessChild()
	}
	reclaimTestMain()
	os.Exit(m.Run())
}
