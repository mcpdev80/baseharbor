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

const lifecycleLockHelperEnv = "BAHA_TEST_LOCK_HELPER"

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
	second := exec.Command(os.Args[0], "-test.run=^TestLifecycleLockHelperChild$")
	second.Env = append(os.Environ(),
		lifecycleLockHelperEnv+"=1",
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
	third := exec.Command(os.Args[0], "-test.run=^TestLifecycleLockHelperChildAcquire$")
	third.Env = append(os.Environ(),
		lifecycleLockHelperEnv+"=1",
		"BAHA_TEST_LOCK_ROOT="+root,
	)
	if out, err := third.CombinedOutput(); err != nil {
		t.Fatalf("separate process rejected after the holder released; output:\n%s", out)
	}
}

// TestLifecycleLockHelperChild is not a test: it is the second-process body
// driven above. While this branch runs, the process holds the lifecycle lock
// on BAHA_TEST_LOCK_ROOT and waits for the release marker, so the parent can
// observe mutual exclusion across real processes.
func TestLifecycleLockHelperChild(t *testing.T) {
	if os.Getenv(lifecycleLockHelperEnv) != "1" {
		t.Skip("lock helper child mode not enabled")
	}
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

// TestLifecycleLockHelperChildAcquire proves the released lock is available to
// a separate process: it acquires, signals nothing, and exits 0.
func TestLifecycleLockHelperChildAcquire(t *testing.T) {
	if os.Getenv(lifecycleLockHelperEnv) != "1" {
		t.Skip("lock helper child mode not enabled")
	}
	unlock, err := AcquireLifecycleLock(os.Getenv("BAHA_TEST_LOCK_ROOT"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "post-release acquire failed: %v\n", err)
		os.Exit(1)
	}
	unlock()
}
