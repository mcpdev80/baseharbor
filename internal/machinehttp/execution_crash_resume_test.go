package machinehttp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// execution_crash_resume_test.go
// Mission w191 round 1, delivery requirement (2): a crash during the production
// apply path must leave the journal in a *durable, observable* non-terminal state.
// A running execution interrupted by a process death is not lost and not silently
// reconciled to a terminal state: after a restart the journal reports the execution
// as running (unreconciled/uncertain), and the operator observes that owned state
// before any explicit retry. This is the "wiederanlauf" (resumption) cell.
//
// It drives the real production apply path (the persisted store over a temp
// state-root), so it is a production-path test, not a helper-direct unit: the
// journal line is produced exactly as the live `execute` endpoint writes it. No
// test skips; a broken restart path (e.g. a load() that downgrades a running
// execution to pending) makes a later read observe the wrong state and fails.

func crashResumeActor() machine.ActorRef {
	return machine.ActorRef{
		Mode:      "operator",
		Issuer:    "https://issuer.example",
		Subject:   "operator-crash",
		Assurance: "high",
	}
}

// TestCrashDuringApplyLeavesRunningUncertainAfterRestart proves the core cell:
// an execution that was durably committed as *running* (apply in progress) when
// the process died is reopened as running/uncertain, never as succeeded/failed,
// and never as pending. The operator observes that owned state before any explicit
// retry is issued. A restart is modelled exactly as production does it: a brand
// new store instance opened over the same on-disk state-root (the old process is
// gone, its in-memory state with it).
func TestCrashDuringApplyLeavesRunningUncertainAfterRestart(t *testing.T) {
	root := t.TempDir()

	// --- Pre-crash: a real apply that started and was durably persisted. ---
	before, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore (pre-crash): %v", err)
	}
	beforeExec, err := before.create("status", crashResumeActor(), journalContext())
	if err != nil {
		t.Fatalf("create (pre-crash): %v", err)
	}
	if err := before.start(beforeExec.ExecutionID); err != nil {
		t.Fatalf("start (pre-crash): %v", err)
	}
	// A progress line proves the apply path was mid-flight when the "crash" hits.
	if err := before.progress(beforeExec.ExecutionID, machine.OperationProgress{Stage: "observe", Percent: 37}); err != nil {
		t.Fatalf("progress (pre-crash): %v", err)
	}
	// The committed journal lines must exist on disk (accepted commitments).
	journal := filepath.Join(root, "journal", "journal.jsonl")
	if _, err := os.Stat(journal); err != nil {
		t.Fatalf("expected persisted journal at %s before crash: %v", journal, err)
	}

	// Simulate the crash: the pre-crash store is discarded without a terminal
	// transition. A restart opens a brand-new store over the same state-root.
	before = nil

	// --- Post-restart: reopen the SAME state-root. ---
	after, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore (post-restart): %v", err)
	}
	got, ok := after.get(beforeExec.ExecutionID)
	if !ok {
		t.Fatal("execution lost across restart: not durable")
	}
	if got.State != machine.ExecutionRunning {
		t.Fatalf("interrupted apply reopened as %q; want running (unreconciled/uncertain)", got.State)
	}
	if got.StartedAt == nil {
		t.Fatal("a running execution reopened after crash must retain its StartedAt (owned state)")
	}
	if got.ExecutionID == "" || got.Actor.Mode == "" || got.Actor.Subject == "" {
		t.Fatal("accepted identity must be observable after restart for correlation")
	}

	// The operator observes this owned/uncertain state BEFORE any explicit retry:
	// a read of the execution returns running, so the caller must reconcile or
	// observe — it must NOT auto-complete.
	switch got.State {
	case machine.ExecutionSucceeded, machine.ExecutionFailed:
		t.Fatal("a crashed apply must never silently resolve to a terminal state")
	case machine.ExecutionPending:
		t.Fatal("a crashed apply must never be downgraded to pending on restart")
	}
}

// TestRestartReconcilesReissueToOriginalExecution proves that after a restart,
// an identical re-issue (same Idempotency-Key + content) reconciles to the SAME
// execution id — exactly as a lost-response reconnect does on a fresh process.
// The apply path itself never auto-completes the interrupted work: the
// reconciled execution is still running/uncertain until the operator performs an
// explicit retry action.
func TestRestartReconcilesReissueToOriginalExecution(t *testing.T) {
	root := t.TempDir()

	before, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	// create() passes an empty request-id, so the create line is NOT persisted to
	// the journal (a persisted journal needs a valid Idempotency-Key). The live
	// production apply path always sends one, so drive the crashed apply through
	// createRequest with a real, valid request identity — exactly as handleExecute
	// does — so the accepted request commitment is durable and survives restart.
	exec, created, err := before.createRequest("crash-apply-key", "destroy", crashResumeActor(), journalContext(), []byte("{}"))
	if err != nil || !created {
		t.Fatalf("create (pre-crash): created=%v err=%v", created, err)
	}
	if err := before.start(exec.ExecutionID); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Crash: no terminal transition.

	// Restart: a fresh store over the same state-root.
	after, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}

	// Identical re-issue after restart reconciles to the same execution id and
	// the same pending identity.
	reissue, created, err := after.createRequest("crash-apply-key", "destroy", crashResumeActor(), journalContext(), []byte("{}"))
	if err != nil {
		t.Fatalf("reissue after restart: %v", err)
	}
	if created || reissue.ExecutionID != exec.ExecutionID {
		t.Fatalf("reissue must reconcile to the original execution id=%s, got id=%s created=%v", exec.ExecutionID, reissue.ExecutionID, created)
	}
	if len(after.records) != 1 {
		t.Fatalf("reissue indexed a second execution; have %d want 1", len(after.records))
	}
	if got, _ := after.get(exec.ExecutionID); got.State != machine.ExecutionRunning {
		t.Fatalf("reconciled execution must still be running/uncertain before explicit retry, got %q", got.State)
	}
}
