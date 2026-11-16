package machinehttp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestReviewProbePersistenceFailureRollsBackWithoutEvent (Luna Befund-3 / recipe B3)
//
// Any persistence failure on the write+Sync+Close commit path MUST fail closed:
// `start` returns an error, the in-memory state stays pending (rolled back), and
// NO operation.started event is published.
//
// Deterministic driver: point the journal at a path whose parent is a regular
// file, so the append Open fails with ENOTDIR. This exercises the full commit
// path (append -> Sync -> Close) failing before commit, which is exactly what
// recipe B3 requires to roll back state and publish no event.
func TestReviewProbePersistenceFailureRollsBackWithoutEvent(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	exec, createErr := store.create("persist-fail", round6Actor("persist-subject"), round6Context("persist"))
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}

	// journalDir's parent is a regular file, so the append target path cannot be
	// opened for writing (ENOTDIR) — a deterministic persistence failure.
	journalDir := filepath.Join(root, "journal")
	if err := os.MkdirAll(journalDir, 0o700); err != nil {
		t.Fatalf("mkdir journal: %v", err)
	}
	if entries, rerr := os.ReadDir(journalDir); rerr == nil {
		for _, e := range entries {
			_ = os.Remove(filepath.Join(journalDir, e.Name()))
		}
	}
	if err := os.Remove(journalDir); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove journal dir: %v", err)
	}
	if err := os.WriteFile(journalDir, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write journal file: %v", err)
	}

	_, ch, cancel, ok := store.subscribe(exec.ExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()

	startErr := store.start(exec.ExecutionID)
	if startErr == nil {
		t.Fatal("FAIL (B3): start with an un-writable journal did not return an error")
	}

	got, _ := store.get(exec.ExecutionID)
	if got.State != machine.ExecutionPending {
		t.Fatalf("FAIL (B3): in-memory state not rolled back on persistence failure: %q, want pending", got.State)
	}

	select {
	case ev := <-ch:
		t.Fatalf("FAIL (B3): event published for a non-durable transition: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// OK: no event published.
	}
}
