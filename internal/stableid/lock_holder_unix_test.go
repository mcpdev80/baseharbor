//go:build !windows

package stableid

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestLifecycleLockContentionExplainsPIDStateAndSafeRelease(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	unlock, err := AcquireLifecycleLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	holder, err := os.ReadFile(root + "/" + lifecycleLockFile)
	if err != nil {
		t.Fatal(err)
	}
	// The receipt is a versioned, line-oriented record (0427-180) whose first
	// field is the holder pid. The pre-0427-180 contract was the bare pid line;
	// the pid is still the authoritative holder identity, so assert it via the
	// documented decoder rather than a brittle whole-line match.
	rec, ok := parseLockHolderRecord(holder)
	if !ok {
		t.Fatalf("receipt is not a recognised baseharbor record: %q", strings.TrimSpace(string(holder)))
	}
	if rec.pid != os.Getpid() {
		t.Fatalf("reported holder pid = %d, want %d", rec.pid, os.Getpid())
	}
	other, err := AcquireLifecycleLock(root)
	if err == nil {
		other()
		t.Fatal("concurrent lock unexpectedly admitted")
	}
	for _, word := range []string{"pid=" + strconv.Itoa(os.Getpid()), "state=alive", "ps -p", "never delete the lock file"} {
		if !strings.Contains(err.Error(), word) {
			t.Fatalf("missing safe holder diagnostic %q: %v", word, err)
		}
	}
	unlock()
	unlock = nil
	next, err := AcquireLifecycleLock(root)
	if err != nil {
		t.Fatalf("legitimate release failed: %v", err)
	}
	next()
}
