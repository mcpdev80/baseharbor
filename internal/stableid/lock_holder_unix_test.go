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
	if got := strings.TrimSpace(string(holder)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("reported holder pid = %q, want %d", got, os.Getpid())
	}
	other, err := AcquireLifecycleLock(root)
	if err == nil {
		other()
		t.Fatal("concurrent lock unexpectedly admitted")
	}
	for _, word := range []string{"pid="+strconv.Itoa(os.Getpid()), "state=alive", "ps -p", "never delete the lock file"} {
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
