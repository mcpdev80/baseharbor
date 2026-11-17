package machinehttp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// durability_round6_test.go
// Runde 6 (w60-ci-authority-e) independent counter-probes for two gaps that
// review-1354d left open:
//
//  1. segment-cap HARD PRE-WRITE (finding 3 follow-up): two individually allowed
//     entries must never yield a single > 4 MiB segment. Asserted literally
//     against every on-disk segment file after many ~2.5 MiB entries, and the
//     last entry must still be recoverable after reopen.
//  2. fail-closed rotation (finding 2 follow-up): when the active segment
//     cannot be rotated (read-only directory while it is already at the cap) the
//     write fails closed and NO event is published for the transition that
//     could not be durably committed.
//
// Existing test functions in execution_journal_test.go / durability_round3_*_test.go
// / durability_round4_test.go are left untouched (commit rule: no existing Test*
// deleted or renamed).

func round6Actor(subject string) machine.ActorRef {
	return machine.ActorRef{
		Mode:      "operator",
		Issuer:    "https://issuer.example",
		Subject:   subject,
		Assurance: "high",
	}
}

func round6Context(target string) machine.OperationContext {
	return machine.OperationContext{
		Application: "demo",
		Environment: "prod",
		Target:      target,
	}
}

// marshalSeedLine writes one raw JSON journal line for (id, op, target) directly
// to path. Used to pre-fill the active segment to the cap so the next write must
// rotate.
func marshalSeedLine(path, id, op, target string) error {
	raw, err := json.Marshal(struct {
		ExecutionID string                   `json:"execution_id"`
		OperationID string                   `json:"operation_id"`
		State       machine.ExecutionState   `json:"state"`
		Actor       machine.ActorRef         `json:"actor,omitempty"`
		Context     machine.OperationContext `json:"context,omitempty"`
		StartedAt   *time.Time               `json:"started_at,omitempty"`
		FinishedAt  *time.Time               `json:"finished_at,omitempty"`
	}{
		ExecutionID: id,
		OperationID: op,
		State:       machine.ExecutionPending,
		Context:     machine.OperationContext{Application: "demo", Environment: "prod", Target: target},
	})
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

// TestRound6SegmentCapHeldAcrossManyEntries (Punkt 1): 12 executions each
// carrying a 419.000-byte Target. Each persisted line is ~2.5 MiB — under the
// single-entry cap — but two of them together would exceed the 4 MiB segment
// cap. This is the exact r1354d failure (two individually-allowed entries that
// produce a > 4 MiB segment under post-write-only rotation). Every on-disk
// segment file must be at most journalSegmentCap bytes, asserted literally, and
// the last execution must still be recoverable after a fresh reopen of the
// journal.
func TestRound6SegmentCapHeldAcrossManyEntries(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	const n = 12
	const entrySize = 419_000 // each line ~2.5 MiB; two lines would exceed 4 MiB
	lastID := ""
	for i := 0; i < n; i++ {
		bigTarget := make([]byte, entrySize) // zero-valued NUL bytes
		exec, createErr := store.create(
			strings.Repeat("op", i+1),
			round6Actor(strings.Repeat("o", 1024)),
			round6Context(string(bigTarget)),
		)
		if createErr != nil {
			t.Fatalf("create %d (~2.5MiB entry): %v", i, createErr)
		}
		if succeedErr := store.succeed(exec.ExecutionID, []byte("{}")); succeedErr != nil {
			t.Fatalf("succeed %d: %v", i, succeedErr)
		}
		lastID = exec.ExecutionID
	}

	// Literal per-file cap assertion: no segment may exceed the cap, including a
	// segment that would have held two 2.5 MiB lines under a post-write-only
	// rotation policy (that is exactly the 5.160.532-byte failure in r1354d).
	entries, readErr := os.ReadDir(filepath.Join(root, "journal"))
	if readErr != nil {
		t.Fatalf("read journal dir: %v", readErr)
	}
	if len(entries) == 0 {
		t.Fatal("no journal segment present after writes")
	}
	for _, e := range entries {
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}
		if info.Size() > journalSegmentCap {
			t.Fatalf("segment %s is %d bytes, exceeds hard cap %d (pre-write rotation broken)", e.Name(), info.Size(), journalSegmentCap)
		}
	}

	// The last execution must be recoverable after a fresh reopen — rotation must
	// never drop the newest line per id.
	reopened, reopenErr := newPersistedExecutionStore(root)
	if reopenErr != nil {
		t.Fatalf("reopen: %v", reopenErr)
	}
	got, ok := reopened.get(lastID)
	if !ok {
		t.Fatal("last execution not recoverable after many rotations")
	}
	if got.State != machine.ExecutionSucceeded {
		t.Fatalf("last execution state after reopen = %q, want succeeded", got.State)
	}
}

// TestRound6ReadOnlyDirRotatesAndFailsClosed (Punkt 1): pre-fill the active
// segment to the cap, then make the journal directory read-only. The next
// transition that MUST rotate (active segment already at the cap) must fail
// closed: the append error propagates and NO event is published on the
// subscriber channel for that non-durable transition. This exercises the
// fail-closed path of the pre-write rotation: a segment that cannot be rotated
// also cannot accept the line.
func TestRound6ReadOnlyDirRotatesAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	journalDir := filepath.Join(root, "journal")
	if err := os.MkdirAll(journalDir, 0o700); err != nil {
		t.Fatalf("mkdir journal: %v", err)
	}
	activePath := filepath.Join(journalDir, "journal.jsonl")

	// Create the execution first while the directory is writable so the record
	// exists in memory and a subscriber can observe whether an event leaks.
	exec, createErr := store.create("ro-fail", round6Actor("ro-subject"), round6Context("ro"))
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}

	// Pre-fill the active segment to the cap with one valid seed line so the next
	// write's pre-write check must rotate.
	if seedErr := marshalSeedLine(activePath, "seed-exec", "seed-op", string(make([]byte, 4_000_000))); seedErr != nil {
		t.Fatalf("write seed line: %v", seedErr)
	}

	// Confirm the active segment is at the cap (rotation needed on next write).
	activeInfo, statErr := os.Stat(activePath)
	if statErr != nil {
		t.Fatalf("stat active segment: %v", statErr)
	}
	if activeInfo.Size() <= journalSegmentCap {
		t.Fatalf("precondition failed: active segment %d bytes, expected already at/over cap", activeInfo.Size())
	}

	// Make the journal directory read-only so rotation (rename) fails closed.
	if chErr := os.Chmod(journalDir, 0o500); chErr != nil {
		t.Skipf("cannot set read-only journal dir: %v", chErr)
	}

	// The transition MUST fail closed: rotation is required but impossible.
	if startErr := store.start(exec.ExecutionID); startErr == nil {
		t.Fatal("start on full + read-only journal dir did not fail closed")
	}
	// Restore write permission so the test framework can clean up the temp dir
	// (a read-only directory would otherwise block RemoveAll).
	_ = os.Chmod(journalDir, 0o700)

	// No event may have been published on a failed append.
	_, ch, cancel, ok := store.subscribe(exec.ExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()
	select {
	case ev := <-ch:
		t.Fatalf("event published for a non-durable transition on read-only dir: %+v", ev)
	case <-time.After(150 * time.Millisecond):
		// OK: no event published.
	}
	got, _ := store.get(exec.ExecutionID)
	if got.State != machine.ExecutionPending {
		t.Fatalf("in-memory state mutated on failed read-only append: %q, want pending", got.State)
	}
}

// TestRound6SingleEntryOverCapStillRejected (Punkt 1, regression): a single entry
// that exceeds the cap is still rejected outright (pre-write hardening must not
// weaken the existing single-entry guard).
func TestRound6SingleEntryOverCapStillRejected(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}
	bigTarget := make([]byte, 5<<20) // 5 MiB
	for i := range bigTarget {
		bigTarget[i] = 'a'
	}
	if _, createErr := store.create("big", round6Actor(string(bigTarget)), round6Context(string(bigTarget))); createErr == nil {
		t.Fatal("create with a >4 MiB entry did not reject")
	}

	entries, readErr := os.ReadDir(filepath.Join(root, "journal"))
	if readErr == nil {
		for _, e := range entries {
			info, infoErr := e.Info()
			if infoErr != nil {
				continue
			}
			if info.Size() > journalSegmentCap {
				t.Fatalf("segment %s = %d bytes exceeds hard cap %d", e.Name(), info.Size(), journalSegmentCap)
			}
		}
	}
}

// rootTestExecutor is a minimal, real machinehttp.Executor for product-
// construction root tests (Punkt 2). It declares a real operation id so the
// production New() registry validation (OperationByID) passes.
type rootTestExecutor struct{}

func (rootTestExecutor) SupportedOperationIDs() []string { return []string{"status"} }
func (rootTestExecutor) Execute(_ context.Context, _ machine.Operation, _ machine.OperationContext, _ json.RawMessage, _ ProgressReporter) (json.RawMessage, error) {
	return nil, nil
}

// rootTestExecutorFor builds a small operator actor for the product-construction
// root test.
func rootTestExecutorFor(subject string) machine.ActorRef {
	return machine.ActorRef{
		Mode:      "operator",
		Issuer:    "https://issuer.example",
		Subject:   subject,
		Assurance: "high",
	}
}

// rootTestCreate / rootTestStart / rootTestGet are thin passthroughs on the
// *Handler produced by the product constructor so this test exercises the real
// production path (New -> persisted store) rather than the internal
// newPersistedExecutionStore directly.
func (h *Handler) rootTestCreate(operationID, actorSubject string) (string, error) {
	exec, err := h.executions.create(operationID, rootTestExecutorFor(actorSubject), round6Context("prod"))
	if err != nil {
		return "", err
	}
	if err := h.executions.start(exec.ExecutionID); err != nil {
		return "", err
	}
	return exec.ExecutionID, nil
}

func (h *Handler) rootTestGet(id string) (machine.Execution, bool) {
	return h.executions.get(id)
}

// TestRound6ProductConstructionValidOpsDirEstablishesDurableJournal (Punkt 2):
// the production construction boundary — machinehttp.New with a concrete,
// non-empty ops-dir — must construct successfully and establish a durable
// journal that survives a reopen of the same store. This exercises the same
// production constructor that registerOperatorAPI calls for a configured
// runtime state-root, rather than the internal newPersistedExecutionStore
// guard directly.
func TestRound6ProductConstructionValidOpsDirEstablishesDurableJournal(t *testing.T) {
	exec := rootTestExecutor{}
	root := t.TempDir()

	h, cErr := New(exec, root)
	if cErr != nil {
		t.Fatalf("machinehttp.New with a valid ops-dir failed: %v", cErr)
	}
	if h == nil {
		t.Fatal("machinehttp.New returned a nil handler for a valid ops-dir")
	}

	// The production constructor must have bound the store to a durable journal
	// under the ops-root: the journal directory exists and a real write persists
	// to disk and survives a reopen (durable, not in-memory).
	if _, statErr := os.Stat(filepath.Join(root, "journal")); statErr != nil {
		t.Fatalf("machinehttp.New did not establish a durable journal dir under ops-dir: %v", statErr)
	}

	exec1, cErr := h.rootTestCreate("prod-op", root)
	if cErr != nil {
		t.Fatalf("create via product constructor: %v", cErr)
	}
	// The journal file must physically exist on disk after the first transition.
	if _, pErr := os.Stat(filepath.Join(root, "journal", "journal.jsonl")); pErr != nil {
		t.Fatalf("journal not persisted to disk after first transition: %v", pErr)
	}
	// Reopen the same store: the running execution is durable across restart.
	reopen, rErr := New(exec, root)
	if rErr != nil {
		t.Fatalf("reopen via product constructor: %v", rErr)
	}
	if _, ok := reopen.rootTestGet(exec1); !ok {
		t.Fatal("execution not durable across restart via product constructor")
	}
}

// TestRound6ProductConstructionNilExecutorFailsClosed (Punkt 2): the production
// constructor rejects a nil executor with the concrete required-field error.
func TestRound6ProductConstructionNilExecutorFailsClosed(t *testing.T) {
	if _, err := New(nil, t.TempDir()); err == nil {
		t.Fatal("machinehttp.New with a nil executor did not fail closed")
	}
}

// TestRound6JournalLineCarriesOperationID is a focused regression check that
// persisted lines carry the identity-critical operation_id required for reopen
// correlation (runde 4 / finding 1), asserted on the active segment.
func TestRound6JournalLineCarriesOperationID(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}
	if _, createErr := store.create("ro6-opid", round6Actor("ro6"), round6Context("ro6")); createErr != nil {
		t.Fatalf("create: %v", createErr)
	}
	lines, readErr := os.ReadFile(store.journalPath)
	if readErr != nil {
		t.Fatalf("read journal: %v", readErr)
	}
	for _, raw := range splitLines(lines) {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var row struct {
			ExecutionID string `json:"execution_id"`
			OperationID string `json:"operation_id"`
		}
		if jsonErr := json.Unmarshal([]byte(raw), &row); jsonErr != nil {
			t.Fatalf("line not JSON: %v", jsonErr)
		}
		if row.OperationID != "ro6-opid" {
			t.Fatalf("journal line missing operation_id, got %q", row.OperationID)
		}
	}
}
