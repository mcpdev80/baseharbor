package machinehttp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestReviewProbeBuriedActiveIdSurvivesTotalCap (Luna Befund-1 / recipe B1)
//
// A buried pending/running/unknown execution-id must survive total-cap
// enforcement. Once enough terminal entries push the journal past the 16 MiB
// total cap, a running id that is never rewritten again must NOT be dropped by
// the newest-per-id compaction — either its newest line is retained, or the
// incoming write that would drop it is rejected (fail closed).
//
// Red-first contra-probe: without the B1 fix this fails — after many large
// terminal entries the buried running id vanishes from every on-disk segment
// even though active records block compaction below the cap.
func TestReviewProbeBuriedActiveIdSurvivesTotalCap(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	// One buried running id, never rewritten after this.
	active, ce := store.create("op-active", round6Actor("a"), round6Context(strings.Repeat("r", 400)))
	if ce != nil {
		t.Fatalf("create active: %v", ce)
	}
	if se := store.start(active.ExecutionID); se != nil {
		t.Fatalf("start active: %v", se)
	}

	// Drive enough terminal (succeeded) entries that total-cap enforcement must
	// rotate repeatedly; the buried running id is never touched again.
	const n = 200
	const entrySize = 300_000
	rejectedAt := -1
	for i := 0; i < n; i++ {
		target := make([]byte, entrySize)
		for j := range target {
			target[j] = 't'
		}
		exec, ce := store.create(strings.Repeat("op", i+1), round6Actor(strings.Repeat("o", 256)), round6Context(string(target)))
		if ce != nil {
			// A total-cap rejection of an active-preventing write is the B1 fix
			// working, not a test failure.
			if strings.Contains(ce.Error(), "total-cap set cannot retain active") {
				rejectedAt = i
				goto done
			}
			t.Fatalf("create %d: %v", i, ce)
		}
		if se := store.succeed(exec.ExecutionID, []byte("{}")); se != nil {
			if strings.Contains(se.Error(), "total-cap set cannot retain active") {
				rejectedAt = i
				goto done
			}
			t.Fatalf("succeed %d: %v", i, se)
		}
	}
done:
	t.Logf("total-cap enforcement rejected an active-preventing write at index %d", rejectedAt)

	// The running id must still be physically present on disk (not a hole) —
	// either retained (fix retains it) or the write that would drop it was
	// rejected (fix rejects it). Either way it must NOT be silently gone.
	entries, _ := os.ReadDir(filepath.Join(root, "journal"))
	onDisk := false
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(root, "journal", e.Name()))
		if strings.Contains(string(data), active.ExecutionID) {
			onDisk = true
		}
	}
	if !onDisk {
		t.Fatal("FAIL (B1): buried running id dropped from every on-disk segment during total-cap enforcement")
	}

	reopened, re := newPersistedExecutionStore(root)
	if re != nil {
		t.Fatalf("reopen: %v", re)
	}
	got, ok := reopened.get(active.ExecutionID)
	if !ok {
		t.Fatal("FAIL (B1): buried running id lost on reopen after total-cap enforcement")
	}
	if got.State != machine.ExecutionRunning {
		t.Fatalf("FAIL (B1): buried active state after reopen = %q, want running", got.State)
	}
}
