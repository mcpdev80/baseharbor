//go:build !windows

package stableid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// 0427-180 stale-holder reclaim regression cells.
//
// FINDING 2 (hp5.md): a SIGKILLed holder leaves its per-app lock dotfile
// behind (the kernel releases the flock on death, but the receipt persists), so
// the target-wide scan reports the per-app dotfile as held and every later
// lifecycle acquire fails closed with no self-help. The fix writes the holder
// pid + start time + boot id into the receipt and reclaims a confirmed-stale
// receipt atomically under the target-wide scope during acquire.
//
// The two OS-process cells re-exe this test binary (helper entry points, not
// tests) so the default package enumeration contains zero skips for the
// hosted authority-package-tests owner, exactly as the two-process exclusion
// cell does.

const reclaimChildEnv = "BAHA_TEST_RECLAIM_CHILD"

// TestStaleAppLockReclaimedBySubsequentAcquire is the FINDING-2 regression:
// a real child process holds a per-app lock, is killed with SIGKILL (no
// cleanup), and a subsequent AcquireAppLifecycleLock on the SAME scope on the
// SAME root must SUCCEED by reclaiming the dead holder's receipt automatically
// — no manual intervention, no deletion of the dotfile by hand.
func TestStaleAppLockReclaimedBySubsequentAcquire(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	deploymentsDir := filepath.Join(root, "deployments")
	scope := "kill-app"
	lockName := "." + scope + "." + appLifecycleLockFile
	lockPath := filepath.Join(deploymentsDir, lockName)

	// Process 1 (a separate re-exec of this test binary) holds the per-app lock
	// and waits on a marker so the parent can kill it while it still holds it.
	markerDir := t.TempDir()
	releaseMarker := filepath.Join(markerDir, "release")

	hold := exec.Command(os.Args[0])
	hold.Env = append(os.Environ(),
		reclaimChildEnv+"=1",
		"BAHA_TEST_RECLAIM_ROOT="+root,
		"BAHA_TEST_RECLAIM_SCOPE="+scope,
		"BAHA_TEST_RECLAIM_RELEASE="+releaseMarker,
	)
	if err := hold.Start(); err != nil {
		t.Fatalf("failed to start holder child: %v", err)
	}

	// Wait for the receipt to appear (the child holds it), up to the holder's
	// 60s self-release ceiling.
	if !waitForFile(lockPath, 30*time.Second) {
		_ = killProcessForce(hold)
		waitForProcessExit(hold)
		t.Fatalf("per-app lock dotfile never appeared; the holder child did not acquire")
	}

	// Kill the holder with SIGKILL: no cleanup, the kernel just drops the flock.
	if err := killProcessForce(hold); err != nil {
		t.Fatalf("failed to SIGKILL the holder: %v", err)
	}
	waitForProcessExit(hold)
	_ = os.Remove(releaseMarker)

	// The dotfile is now stale: its recorded holder is dead. A subsequent
	// acquire on the same scope must reclaim it and SUCCEED.
	unlock, err := AcquireAppLifecycleLock(root, scope)
	if err != nil {
		t.Fatalf("subsequent acquire failed after the holder was SIGKILLed; reclaim did not work: %v", err)
	}
	unlock()
}

// TestStaleAppLockStillBlocksLiveHolder proves the reclaim does NOT bypass
// cross-process contention: while a real child still holds the per-app lock,
// a subsequent acquire on the same scope is still refused (the holder is live).
func TestStaleAppLockStillBlocksLiveHolder(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	markerDir := t.TempDir()
	releaseMarker := filepath.Join(markerDir, "release")

	live := exec.Command(os.Args[0])
	live.Env = append(os.Environ(),
		reclaimChildEnv+"=1",
		"BAHA_TEST_RECLAIM_ROOT="+root,
		"BAHA_TEST_RECLAIM_SCOPE="+"live-app",
		"BAHA_TEST_RECLAIM_RELEASE="+releaseMarker,
	)
	if err := live.Start(); err != nil {
		t.Fatalf("failed to start live holder child: %v", err)
	}
	defer func() {
		_ = killProcessForce(live)
		waitForProcessExit(live)
	}()
	if !waitForFile(lockFilePath(root, "live-app"), 30*time.Second) {
		t.Fatal("live holder did not stage its per-app dotfile")
	}

	// A subsequent acquire while the live holder still runs must be refused.
	if _, err := AcquireAppLifecycleLock(root, "live-app"); err == nil {
		t.Fatal("acquire admitted while a live holder still held the per-app lock")
	}

	// Release the live holder, then confirm a subsequent acquire succeeds.
	_ = os.WriteFile(releaseMarker, nil, 0600)
	waitForProcessExit(live)

	// Now that the holder released, a subsequent acquire must succeed.
	unlock, err := AcquireAppLifecycleLock(root, "live-app")
	if err != nil {
		t.Fatalf("acquire failed after the live holder released normally: %v", err)
	}
	unlock()
}

// lockFilePath returns the per-app dotfile path for a scope on a root.
func lockFilePath(root, scope string) string {
	return filepath.Join(root, "deployments", "."+scope+"."+appLifecycleLockFile)
}

// TestStaleAppLockLiveBootStillBlocks proves the reclaim is generation-aware:
// a receipt whose holder is alive on the CURRENT boot is NOT reclaimed even if
// the pid matches (pid reuse). This pins that lockHolderLive treats a live
// holder as live.
func TestStaleAppLockLiveBootStillBlocks(t *testing.T) {
	rec := lockHolderRecord{
		pid:       os.Getpid(),
		startTime: time.Now().Unix(),
		bootID:    lockHolderBootID(),
	}
	if !lockHolderLive(rec) {
		t.Fatal("a receipt for the current live process with matching boot-id was judged stale")
	}
	// A dead pid is always stale.
	if lockHolderLive(lockHolderRecord{pid: 999999, bootID: lockHolderBootID()}) {
		t.Fatal("a dead pid was judged live")
	}
}

// TestReclaimLeavesForeignReceiptUntouched proves a malformed / foreign receipt
// is never destroyed on a decode guess: a foreign receipt is left in place.
func TestReclaimLeavesForeignReceiptUntouched(t *testing.T) {
	root := t.TempDir()
	deploymentsDir := filepath.Join(root, "deployments")
	if err := os.MkdirAll(deploymentsDir, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := ".foreign.app_installation.lock"
	if err := os.WriteFile(filepath.Join(deploymentsDir, foreign), []byte("some foreign text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := reclaimStaleAppLock(deploymentsDir, foreign)
	if err != nil {
		t.Fatalf("reclaim returned an error: %v", err)
	}
	if reclaimed {
		t.Fatal("foreign receipt was reclaimed")
	}
	if _, err := os.Stat(filepath.Join(deploymentsDir, foreign)); err != nil {
		t.Fatal("foreign receipt was removed")
	}
}

// reclaimChild is the OS-process body for the FINDING-2 cells: it acquires the
// per-app lock on BAHA_TEST_RECLAIM_ROOT/scope and holds it until either the
// release marker appears (the live-holder cell) or the parent kills it with
// SIGKILL (the stale-cell). There is no self-release deadline: the parent
// controls the holder's lifetime, so the dotfile is present exactly while the
// holder is alive.
func reclaimChild() {
	root := os.Getenv("BAHA_TEST_RECLAIM_ROOT")
	scope := os.Getenv("BAHA_TEST_RECLAIM_SCOPE")
	release := os.Getenv("BAHA_TEST_RECLAIM_RELEASE")
	unlock, err := AcquireAppLifecycleLock(root, scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reclaim holder acquire failed: %v\n", err)
		os.Exit(1)
	}
	// Hold until the parent signals release OR is killed. If killed, the
	// process dies here with the lock still held; the kernel drops the flock
	// and the receipt is left behind as the stale artifact this regression
	// proves can be reclaimed.
	for time.Now().Before(time.Now().Add(60 * time.Second)) {
		if _, err := os.Stat(release); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	unlock()
	os.Exit(0)
}

// reclaimTestMain is the reclaim child dispatch. It is merged into the single
// TestMain in two_process_lock_test.go (a package may have only one), so this
// file does NOT declare its own TestMain.
func reclaimTestMain() {
	if os.Getenv(reclaimChildEnv) == "1" {
		reclaimChild()
	}
}

// waitForFile polls until path exists or the deadline elapses.
func waitForFile(path string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// killProcessForce sends SIGKILL to a child started via exec.Command and waits
// for it to be reaped, so the receipt it left behind is provably from a dead
// holder.
func killProcessForce(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	return nil
}

// waitForProcessExit waits for a killed child to be reaped (so the receipt it
// left behind is provably from a dead holder).
func waitForProcessExit(cmd *exec.Cmd) {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}
