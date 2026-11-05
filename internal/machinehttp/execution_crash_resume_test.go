package machinehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// execution_crash_resume_test.go
// Mission w191 round 1, delivery requirement (2); round-2 revision (w196): the
// crash-resume cell must be proven through the REAL HTTP `handleExecute`
// production seam with a genuine process death, not through in-process store
// calls plus a nil receiver.
//
// A running execution interrupted by a process death is not lost and not silently
// reconciled to a terminal state: after a restart the journal reports the
// execution as running (unreconciled/uncertain), and the operator observes that
// owned state before any explicit retry.
//
// How the crash is modelled exactly as production does it:
//   - The child process (a re-exe of this test binary via the helper env seam)
//     builds a real machinehttp.Handler over a persistent state-root and drives
//     the real POST /execute route (the exact route `New` registers). The
//     executor commits `start` (the handler runs runExecution synchronously on
//     the 202's tail) and then blocks forever, so the execution stays durably
//     `running` on disk.
//   - The parent waits for the child's readiness marker (proving start is
//     persisted), then SIGKILLs the child — the process is genuinely gone, its
//     in-memory state with it, exactly like a crash.
//   - The parent reopens a brand-new store over the SAME on-disk state-root and
//     asserts the interrupted apply reopened as running/uncertain, never
//     succeeded/failed/pending, and that an identical re-issue reconciles to the
//     SAME execution id.
//
// No test skips; a broken restart path (e.g. a load() that downgrades a running
// execution to pending) makes the reopen observe the wrong state and fails.

const crashHelperEnv = "BAHA_TEST_CRASH_RESUME"

// TestCrashDuringApplyLeavesRunningUncertainAfterRestart drives the real HTTP
// apply path, kills the applying process mid-Apply (SIGKILL) while the apply is
// durably `running`, reopens the same root, and asserts the execution is
// reopened as running/uncertain — never terminal, never pending.
func TestCrashDuringApplyLeavesRunningUncertainAfterRestart(t *testing.T) {
	root := t.TempDir()

	readiness := filepath.Join(t.TempDir(), "crash-ready")
	pidFile := filepath.Join(t.TempDir(), "crash-pid")

	child := exec.Command(os.Args[0], "-test.run", "^$")
	child.Env = append(os.Environ(),
		crashHelperEnv+"=1",
		"BAHA_TEST_CRASH_ROOT="+root,
		"BAHA_TEST_CRASH_READY="+readiness,
		"BAHA_TEST_CRASH_PID="+pidFile,
		"BAHA_TEST_CRASH_KEY=crash-apply-key",
	)
	child.Stdout = os.Stderr
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}

	// Wait for the readiness marker: the child has driven the real seam and the
	// `start` line is durably persisted as running.
	if !waitForFile(t, readiness, 15*time.Second) {
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatal("child never signalled readiness: the real apply path did not persist start")
	}

	// Crash: read the child PID and SIGKILL it mid-Apply. The process is gone,
	// its in-memory state with it, exactly like a hard process death.
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		t.Fatalf("parse child pid: %v", err)
	}
	if proc, ferr := os.FindProcess(pid); ferr == nil {
		_ = proc.Kill()
	}
	_ = child.Wait()

	// The committed journal lines must exist on disk (accepted commitments).
	journal := filepath.Join(root, "journal", "journal.jsonl")
	if _, err := os.Stat(journal); err != nil {
		t.Fatalf("expected persisted journal at %s after crash: %v", journal, err)
	}

	// --- Post-restart: reopen the SAME state-root. ---
	after, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("newPersistedExecutionStore (post-restart): %v", err)
	}

	ids := executionIDsFromJournal(t, root)
	if len(ids) == 0 {
		t.Fatal("no execution id persisted to the journal: the apply write was lost")
	}
	crashedID := ids[0]
	got, ok := after.get(crashedID)
	if !ok {
		t.Fatal("execution lost across crash: not durable")
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
	// a read returns running, so the caller must reconcile — it must NOT
	// auto-complete.
	switch got.State {
	case machine.ExecutionSucceeded, machine.ExecutionFailed:
		t.Fatal("a crashed apply must never silently resolve to a terminal state")
	case machine.ExecutionPending:
		t.Fatal("a crashed apply must never be downgraded to pending on restart")
	}
}

// TestRestartReconcilesReissueToOriginalExecution drives the crashed apply
// through the real HTTP seam, kills the process mid-Apply, then asserts an
// identical re-issue (same Idempotency-Key + content) after restart reconciles
// to the SAME execution id and the execution is still running/uncertain.
func TestRestartReconcilesReissueToOriginalExecution(t *testing.T) {
	root := t.TempDir()

	readiness := filepath.Join(t.TempDir(), "crash-ready")
	pidFile := filepath.Join(t.TempDir(), "crash-pid")

	child := exec.Command(os.Args[0], "-test.run", "^$")
	child.Env = append(os.Environ(),
		crashHelperEnv+"=1",
		"BAHA_TEST_CRASH_ROOT="+root,
		"BAHA_TEST_CRASH_READY="+readiness,
		"BAHA_TEST_CRASH_PID="+pidFile,
		"BAHA_TEST_CRASH_KEY=reissue-apply-key",
	)
	child.Stdout = os.Stderr
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	if !waitForFile(t, readiness, 15*time.Second) {
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatal("child never signalled readiness")
	}
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		t.Fatalf("parse child pid: %v", err)
	}
	if proc, ferr := os.FindProcess(pid); ferr == nil {
		_ = proc.Kill()
	}
	_ = child.Wait()

	// The crashed apply id (persisted through the real seam) is what the
	// re-issue must reconcile to.
	ids := executionIDsFromJournal(t, root)
	if len(ids) == 0 {
		t.Fatal("no execution id persisted: the crashed apply wrote nothing")
	}
	originalID := ids[0]

	// --- Restart: a brand-new process re-posts the IDENTICAL request. ---
	// A lost-response reconnect is modelled exactly as production does it: the
	// client retries the same Idempotency-Key + content on a fresh process. The
	// child writes "reconciled" (same id) or "new" (different id) to a marker.
	reconcileMarker := filepath.Join(t.TempDir(), "reissue-result")
	reissueChild := exec.Command(os.Args[0], "-test.run", "^$")
	debugMarker := filepath.Join(t.TempDir(), "reissue-debug")
	reissueChild.Env = append(os.Environ(),
		crashHelperEnv+"=2",
		"BAHA_TEST_CRASH_ROOT="+root,
		"BAHA_TEST_CRASH_RECONCILE="+reconcileMarker,
		"BAHA_TEST_CRASH_KEY=reissue-apply-key",
	)
	reissueChild.Stdout, reissueChild.Stderr = os.Stderr, os.Stderr
	if err := reissueChild.Start(); err != nil {
		t.Fatalf("start reissue child: %v", err)
	}
	if werr := reissueChild.Wait(); werr != nil {
		_ = os.WriteFile(debugMarker, []byte("child err: "+werr.Error()), 0o600)
		t.Fatalf("reissue child exited: %v", werr)
	}
	if _, derr := os.Stat(reconcileMarker); derr != nil {
		_ = os.WriteFile(debugMarker, []byte("marker missing (stderr shown above)"), 0o600)
		t.Fatalf("reissue child did not write reconcile marker (stderr shown above); marker path=%s", reconcileMarker)
	}
	reconcileData, rerr := os.ReadFile(reconcileMarker)
	if rerr != nil {
		t.Fatalf("read reconcile marker: %v", rerr)
	}
	result := strings.TrimSpace(string(reconcileData))
	if result == "new" || result == "error" {
		t.Fatalf("identical re-issue after restart must reconcile to the original execution, got %q", result)
	}

	// Reopen the same state-root and confirm the reconciled execution is still
	// running/uncertain before an explicit retry (the apply never auto-completes).
	after, err := newPersistedExecutionStore(root)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if got, _ := after.get(originalID); got.State != machine.ExecutionRunning {
		t.Fatalf("reconciled execution must still be running/uncertain before explicit retry, got %q", got.State)
	}
}

// crashResumeActor is the exact verified operator identity the child HTTP call
// (via withTestPrincipal) used, so an identical re-issue after restart takes the
// real reconcile path (same actor+context+input+operation) rather than a
// "request identity already accepted" mismatch.
func crashResumeActor() machine.ActorRef {
	return machine.ActorRef{
		Mode:      "authenticated",
		Issuer:    "https://issuer.example",
		Subject:   "operator-123",
		Assurance: "urn:mfa",
		Methods:   []string{"pwd", "otp"},
	}
}

// writeLine appends one line to path, creating it if needed.
func writeLine(path, line string) error {
	return os.WriteFile(path, []byte(line+"\n"), 0o600)
}

// waitForFile polls for the existence of path until the deadline.
func waitForFile(t *testing.T, path string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// executionIDsFromJournal returns the execution ids found in the journal, in
// line order. The first id is the crashed apply's id.
func executionIDsFromJournal(t *testing.T, root string) []string {
	t.Helper()
	lines, err := readJournalLines(root)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var ids []string
	for _, ln := range lines {
		var line struct {
			ExecutionID string `json:"execution_id"`
		}
		if jerr := json.Unmarshal([]byte(ln), &line); jerr != nil {
			t.Fatalf("journal line not valid JSON: %q: %v", ln, jerr)
		}
		if line.ExecutionID != "" {
			ids = append(ids, line.ExecutionID)
		}
	}
	return ids
}

// crashChildMain is the helper-mode entry point (a re-exe of this test binary).
//
//	mode == "1": build a real machinehttp.Handler over the shared state-root and
//	  drive the real POST /execute route exactly as production does it:
//	  createRequest is called, the 202 is coded, then runExecution commits `start`
//	  and then the executor blocks forever so the parent can catch the process
//	  mid-Apply and kill it. The child writes its PID and a readiness marker first
//	  so the parent can target the crash precisely.
func crashChildMain() {
	root := os.Getenv("BAHA_TEST_CRASH_ROOT")
	key := os.Getenv("BAHA_TEST_CRASH_KEY")
	readiness := os.Getenv("BAHA_TEST_CRASH_READY")
	pidFile := os.Getenv("BAHA_TEST_CRASH_PID")

	started := make(chan struct{})
	block := make(chan struct{})
	ex := executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		// runExecution has already committed `start` (synchronously) before this
		// body runs; the execution is durably `running`. Announce readiness, then
		// hang forever so the parent's SIGKILL lands mid-Apply.
		select {
		case <-started:
		default:
			close(started)
			_ = writeLine(readiness, "started")
		}
		<-block
		return json.RawMessage(`{"blocked":true}`), nil
	})
	h, err := New(ex, root)
	if err != nil {
		_ = writeLine(readiness, "New failed: "+err.Error())
		os.Exit(2)
	}
	// Publish our PID so the parent can SIGKILL exactly this process.
	_ = writeLine(pidFile, strconv.Itoa(os.Getpid()))

	// Drive the real POST /execute route: decode -> authorize -> createRequest
	// -> (202) -> runExecution. This is the exact production seam.
	reqBody := strings.NewReader(`{"operation_id":"status","context":{"application":"demo","environment":"prod","target":"prod-eu"}}`)
	req, nerr := http.NewRequest(http.MethodPost, "/api/v1/machine/executions", reqBody)
	if nerr != nil {
		_ = writeLine(readiness, "new request failed: "+nerr.Error())
		os.Exit(2)
	}
	req = withTestPrincipal(req)
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		_ = writeLine(readiness, "execute status="+strconv.Itoa(rec.Code))
		os.Exit(3)
	}
	// Block forever; the parent kills this process mid-Apply.
	<-block
}

// crashReissueChildMain is the helper-mode entry point for the lost-response
// reconnect (mode == "2"): a fresh process reopens the SAME persistent root (a
// brand-new store instance == a restart) and re-issues the IDENTICAL
// Idempotency-Key + content through the real createRequest path. Reconciliation
// is decided by createRequest's created flag: an identical request returns
// created==false bound to the original execution id; a different request would
// return created==true (a "new" execution). The child reports which by writing
// "reconciled" or "new" to the reconcile marker.
func crashReissueChildMain() {
	root := os.Getenv("BAHA_TEST_CRASH_ROOT")
	key := os.Getenv("BAHA_TEST_CRASH_KEY")
	reconcileMarker := os.Getenv("BAHA_TEST_CRASH_RECONCILE")

	h, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		return json.RawMessage(`{"done":true}`), nil
	}), root)
	if err != nil {
		_ = writeLine(reconcileMarker, "error:"+err.Error())
		os.Exit(2)
	}
	_, created, cerr := h.executions.createRequest(key, "status", crashResumeActor(), journalContext(), []byte("{}"))
	if cerr != nil {
		_ = writeLine(reconcileMarker, "error:"+cerr.Error())
		os.Exit(4)
	}
	if created {
		_ = writeLine(reconcileMarker, "new")
	} else {
		_ = writeLine(reconcileMarker, "reconciled")
	}
	os.Exit(0)
}
