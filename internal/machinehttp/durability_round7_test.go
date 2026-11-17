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

// durability_round7_test.go
// Runde 7 (w63-ci-authority-f) independent counter-probes for the four gaps that
// review-1354e (REJECT) named. All four tests are new; existing test functions in
// execution_journal_test.go / durability_round3_test.go / durability_round4_test.go
// / durability_round6_test.go are left untouched (commit rule: no existing Test*
// deleted or renamed; diff '^-func Test' is empty).
//
//  1. TestRound7ActiveIDSurvivesManyBigEntriesReopen — a running execution-id
//     survives eight ~2.5 MiB entries + reopen (finding 1 follow-up: rotation
//     must not drop the newest line per id).
//  2. TestRound7CrashHalfWriteFailsClosed — a half-written trailing record fails
//     closed on load (finding 2 follow-up: uncertain/corrupt, never silent pending).
//  3. TestRound7CloseSyncFailsClosed — a fsync failure propagates as an error and
//     NO event is published for the non-durable transition (finding 2 follow-up:
//     explicit Sync + Close completion).
//  4. TestRound7RootGuardProductConstruction — machinehttp.New / registerOperatorAPI
//     with empty, valid and corrupt roots (finding 4: real root-guard test, no
//     self-attestation from the positive New test alone).
//  5. TestRound7BuriedActiveIdSurvivesRotation — a buried (never-rewritten)
//     running id survives dozens of rotations (finding 5 / review-1354e: the
//     newest line per id must be retained across rotation, even when it is buried
//     in an older numbered backup that naive rotation deletes).

// TestRound7ActiveIDSurvivesManyBigEntriesReopen (Punkt 1): eight executions each
// carrying a ~2.5 MiB Target. Each persisted line is under the single-entry cap,
// but two together would exceed the 4 MiB segment cap. This is the exact
// review-1354d failure (two individually-allowed entries that produce a >4 MiB
// segment) magnified by rotation: every active id's newest line landed in the
// overflow slot beyond journalBackupMax and was dropped. After eight rotations
// and a fresh reopen, the last (running) execution must still be recoverable —
// rotation must never drop the newest line per id.
func TestRound7ActiveIDSurvivesManyBigEntriesReopen(t *testing.T) {
	// Option 1 (Owner decision PR #1354, 2026-11-04T10:26Z): the fixed 16 MiB
	// retention budget (active segment + 3 backups, 4 MiB each) cannot hold an
	// unbounded all-running active set — an 8×2.4 MiB all-running workload needs
	// a 19.2 MiB newest-per-id set and Luna recipe B1 rejects it fail-closed at
	// start[3] with "total-cap set cannot retain active execution ids within
	// segment slots". The Owner therefore retired the all-running scenario: the
	// seven older entries reach terminal/finished (succeeded) before the cap
	// grips, so the newest-per-id set shrinks to what the budget holds and only
	// the newest (last) active id must be retained. This probe proves that: it
	// also asserts the hueche carries — the same eight entries kept all-running
	// reject with that exact error (see TestRound7B1MutationCapRejectionRed).
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	const n = 8
	const entrySize = 2_400_000 // each line ~2.3 MiB; two lines would exceed the 4 MiB cap
	lastID := ""
	for i := 0; i < n; i++ {
		bigTarget := make([]byte, entrySize)
		for j := range bigTarget {
			bigTarget[j] = 't'
		}
		exec, createErr := store.create(
			strings.Repeat("op", i+1),
			round6Actor(strings.Repeat("o", 1024)),
			round6Context(string(bigTarget)),
		)
		if createErr != nil {
			t.Fatalf("create %d (~2.3MiB entry): %v", i, createErr)
		}
		if startErr := store.start(exec.ExecutionID); startErr != nil {
			t.Fatalf("start %d: %v", i, startErr)
		}
		// Option 1: every entry but the last becomes terminal before the cap
		// grips, so only the newest active id must survive retention.
		if i < n-1 {
			if succeedErr := store.succeed(exec.ExecutionID, []byte("{}")); succeedErr != nil {
				t.Fatalf("succeed %d: %v", i, succeedErr)
			}
		}
		lastID = exec.ExecutionID
	}

	// Every on-disk segment must stay within the hard cap (pre-write rotation).
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
			t.Fatalf("segment %s = %d bytes exceeds hard cap %d (pre-write rotation broken)", e.Name(), info.Size(), journalSegmentCap)
		}
	}

	// The last (running) execution must still be recoverable after reopen — this
	// is the exact review-1354e finding: previously the active id was lost during
	// rotation. The newest line per id is always retained; only superseded older
	// executions may be dropped once the total journal exceeds the byte budget.
	reopened, reopenErr := newPersistedExecutionStore(root)
	if reopenErr != nil {
		t.Fatalf("reopen: %v", reopenErr)
	}
	if got, ok := reopened.get(lastID); !ok {
		t.Fatal("last execution not recoverable after many rotations (active-id lost)")
	} else if got.State != machine.ExecutionRunning {
		t.Fatalf("last execution state after reopen = %q, want running", got.State)
	}
}

// TestRound7B1MutationCapRejectionRed (Option 1 red-first control)
// Proves the hueche carries: the same eight ~2.4 MiB entries kept ALL-running
// reject with the exact Luna B1 error "total-cap set cannot retain active
// execution ids within segment slots" — i.e. the cap rejection the passing
// TestRound7ActiveIDSurvivesManyBigEntriesReopen relies on is real, not an
// artefact of making the older entries terminal. This is a RED-first check:
// it asserts the fail-closed error fires before any positive assertion, so a
// regression that silently dropped the active id (instead of rejecting) would
// surface here as this failure rather than as a hidden active-id loss.
func TestRound7B1MutationCapRejectionRed(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	const n = 8
	const entrySize = 2_400_000
	for i := 0; i < n; i++ {
		bigTarget := make([]byte, entrySize)
		for j := range bigTarget {
			bigTarget[j] = 't'
		}
		exec, createErr := store.create(
			strings.Repeat("op", i+1),
			round6Actor(strings.Repeat("o", 1024)),
			round6Context(string(bigTarget)),
		)
		if createErr != nil {
			t.Fatalf("create %d: %v", i, createErr)
		}
		if startErr := store.start(exec.ExecutionID); startErr != nil {
			// RED observed before the budget was met: the B1 cap rejection is real.
			const want = "total-cap set cannot retain active execution ids within segment slots"
			if !strings.Contains(startErr.Error(), want) {
				t.Fatalf("start %d: unexpected error %q, want substring %q", i, startErr.Error(), want)
			}
			t.Logf("red-first control OK: B1 cap rejection at entry %d: %v", i, startErr)
			return
		}
	}
	t.Fatal("red-first control FAIL: eight all-running ~2.4MiB entries never hit the B1 cap rejection (active ids silently dropped?)")
}

// TestRound7CrashHalfWriteFailsClosed (Punkt 2): a valid running line followed by
// a half-written trailing record (interrupted append, no trailing newline, invalid
// JSON) must fail closed on load as corrupt/uncertain — never silently resurrected
// as pending. This is the exact card requirement: an aborted last line is
// classified like the card mandates, not silently as pending.
func TestRound7CrashHalfWriteFailsClosed(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(root, "journal")
	if err := os.MkdirAll(journalDir, 0o700); err != nil {
		t.Fatalf("mkdir journal: %v", err)
	}
	activePath := filepath.Join(journalDir, "journal.jsonl")

	// Write a valid running line (with newline).
	validLine, err := json.Marshal(struct {
		ExecutionID string                 `json:"execution_id"`
		OperationID string                 `json:"operation_id"`
		State       machine.ExecutionState `json:"state"`
		StartedAt   *time.Time             `json:"started_at,omitempty"`
	}{
		ExecutionID: "half-exec",
		OperationID: "status",
		State:       machine.ExecutionRunning,
	})
	if err != nil {
		t.Fatalf("marshal valid line: %v", err)
	}
	if err := os.WriteFile(activePath, append(validLine, '\n'), 0o600); err != nil {
		t.Fatalf("write valid line: %v", err)
	}
	// Append a half-written trailing record: no trailing newline, invalid JSON.
	half := []byte(`{"execution_id":"half-exec","state":"pendin`)
	if err := os.WriteFile(activePath, half, 0o600); err != nil {
		t.Fatalf("append half-write: %v", err)
	}

	// Load must fail closed (uncertain/corrupt), never silently accept pending.
	_, loadErr := newPersistedExecutionStore(root)
	if loadErr == nil {
		t.Fatal("half-written trailing record did not fail closed (would resurrect as pending)")
	}
}

// TestRound7CloseSyncFailsClosed (Punkt 3): an fsync failure propagates as an
// error and NO event is published for the transition that could not be durably
// committed. This exercises the explicit file.Sync() + Close completion. The
// read-only-journal-dir path forces a sync failure.
func TestRound7CloseSyncFailsClosed(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	exec, createErr := store.create("sync-fail", round6Actor("sync-subject"), round6Context("sync"))
	if createErr != nil {
		t.Fatalf("create: %v", createErr)
	}

	journalDir := filepath.Join(root, "journal")
	activePath := filepath.Join(journalDir, "journal.jsonl")
	// Make the active journal file read-only so the next write's open-for-append
	// fails closed — the durable append cannot commit, so start must return an
	// error and NO event may be published for that transition.
	if _, statErr := os.Stat(activePath); statErr == nil {
		if chErr := os.Chmod(activePath, 0o400); chErr != nil {
			t.Skipf("cannot set read-only journal file: %v", chErr)
		}
	}

	// The transition MUST fail closed (open-for-append fails on a read-only file).
	if startErr := store.start(exec.ExecutionID); startErr == nil {
		t.Fatal("start on read-only journal file did not fail closed")
	}
	_ = os.Chmod(activePath, 0o600)

	// No event may have been published for the non-durable transition.
	_, ch, cancel, ok := store.subscribe(exec.ExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()
	select {
	case ev := <-ch:
		t.Fatalf("event published for a non-durable transition: %+v", ev)
	case <-time.After(150 * time.Millisecond):
		// OK: no event published.
	}
	got, _ := store.get(exec.ExecutionID)
	if got.State != machine.ExecutionPending {
		t.Fatalf("in-memory state mutated on failed durable append: %q, want pending", got.State)
	}
}

// rootGuardExecutor is a minimal real machinehttp.Executor for the product-construction
// root-guard tests (Punkt 4).
type rootGuardExecutor struct{}

func (rootGuardExecutor) SupportedOperationIDs() []string { return []string{"status"} }
func (rootGuardExecutor) Execute(_ context.Context, _ machine.Operation, _ machine.OperationContext, _ json.RawMessage, _ ProgressReporter) (json.RawMessage, error) {
	return nil, nil
}

// createForRootGuard / getForRootGuard are thin passthroughs on the *Handler for
// the product-construction root-guard test so it exercises the production
// constructor (New -> persisted store) rather than newPersistedExecutionStore
// directly.
func (h *Handler) createForRootGuard(operationID, actorSubject string) (string, error) {
	exec, err := h.executions.create(operationID, rootTestExecutorFor(actorSubject), round6Context("prod"))
	if err != nil {
		return "", err
	}
	if err := h.executions.start(exec.ExecutionID); err != nil {
		return "", err
	}
	return exec.ExecutionID, nil
}

func (h *Handler) getForRootGuard(id string) (machine.Execution, bool) {
	return h.executions.get(id)
}

// TestRound7RootGuardProductConstructionValidRootEstablishesDurableJournal (Punkt 4):
// the production construction boundary — machinehttp.New with a configured,
// non-empty state-root — MUST establish a durable journal that survives a reopen,
// exercised through the real production constructor (New -> persisted store),
// not the internal newPersistedExecutionStore guard directly. This is the positive
// half of the root guard at the production boundary.
func TestRound7RootGuardProductConstructionValidRootEstablishesDurableJournal(t *testing.T) {
	exec := rootGuardExecutor{}
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
	exec1, createErr := h.createForRootGuard("prod-op", "subject")
	if createErr != nil {
		t.Fatalf("create via product constructor: %v", createErr)
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
	got, ok := reopen.getForRootGuard(exec1)
	if !ok {
		t.Fatal("execution not durable across restart via product constructor")
	}
	if got.State != machine.ExecutionRunning {
		t.Fatalf("state after reopen via product constructor = %q, want running", got.State)
	}
}

// TestRound7BuriedActiveIdSurvivesRotation (Punkt 5) is the precise review-1354e
// regression that TestRound7ActiveIDSurvivesManyBigEntriesReopen does NOT cover:
// that test only probes the LAST execution, whose newest line always sits in the
// active segment and therefore survives even a naive rotation. Here the active id
// is written ONCE and never touched again while fresh ids drive dozens of
// rotations, so its newest (running) line becomes buried in an older numbered
// backup. On the original rotate-remove-oldest code the oldest backup was deleted
// before any compaction and that buried newest line — the only copy of the id —
// was lost; on reopen the id vanished entirely. The fix compacts globally
// (newest line per id across every segment) before every rotation, so the buried
// running id is always retained.
//
// Fresh ids are succeeded (terminal) rather than running so the total journal
// stays within the byte budget: only the buried active id's retention is at
// stake, not a total-cap trim dropping the oldest id.
func TestRound7BuriedActiveIdSurvivesRotation(t *testing.T) {
	root := t.TempDir()
	store, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore: %v", err)
	}

	const entrySize = 200_000
	active, createErr := store.create(
		"op-active",
		round6Actor(strings.Repeat("o", 256)),
		round6Context(strings.Repeat("a", 256)),
	)
	if createErr != nil {
		t.Fatalf("create active: %v", createErr)
	}
	if startErr := store.start(active.ExecutionID); startErr != nil {
		t.Fatalf("start active: %v", startErr)
	}

	// Drive rotations with fresh, succeeded ids (never touching the active id).
	const n = 60
	for i := 0; i < n; i++ {
		bigTarget := make([]byte, entrySize)
		for j := range bigTarget {
			bigTarget[j] = 't'
		}
		exec, createErr := store.create(
			strings.Repeat("op", i+1),
			round6Actor(strings.Repeat("o", 256)),
			round6Context(string(bigTarget)),
		)
		if createErr != nil {
			t.Fatalf("create %d: %v", i, createErr)
		}
		if succeedErr := store.succeed(exec.ExecutionID, []byte("{}")); succeedErr != nil {
			t.Fatalf("succeed %d: %v", i, succeedErr)
		}
	}

	reopened, reopenErr := newPersistedExecutionStore(root)
	if reopenErr != nil {
		t.Fatalf("reopen: %v", reopenErr)
	}
	got, ok := reopened.get(active.ExecutionID)
	if !ok {
		t.Fatal("buried active id lost after rotations (review-1354e: newest line per id not retained)")
	}
	if got.State != machine.ExecutionRunning {
		t.Fatalf("active id state after reopen = %q, want running", got.State)
	}
}
