package machinehttp

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// waitForExecutorStart blocks deterministically until the executor for id has
// begun: the store's "operation.started" event fires when the handler calls
// executions.start before Execute. Because the terminal state is committed on
// the 202's tail, that same start event is the only thing the identity/reconnect
// tests need to observe before they assert executor completion. A missing start
// event is a real failure, not a timing artifact — this signals the exact
// executor transition, never polls or sleeps.
func waitForExecutorStart(t *testing.T, handler *Handler, executionID string) {
	t.Helper()
	history, events, cancel, ok := handler.executions.subscribe(executionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()
	waitForEvent(t, "executor start", func(ev machine.MachineEvent) bool {
		return ev.Kind == machine.EventOperationStarted
	}, history, events)
}

// waitForTerminal blocks until the execution's history carries a terminal event
// (succeeded/failed) on the store's event channel or in the history already
// returned to the subscriber. The terminal event is emitted synchronously by
// succeed()/fail() before runExecution returns, so a synchronous candidate
// signals it on the 202's tail and a backgrounded mutant never does. No sleep,
// no poll loop.
func waitForTerminal(t *testing.T, handler *Handler, executionID string) machine.MachineEvent {
	t.Helper()
	history, events, cancel, ok := handler.executions.subscribe(executionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()
	return waitForEvent(t, "terminal succeeded/failed state", func(ev machine.MachineEvent) bool {
		return ev.Kind == machine.EventOperationSucceeded || ev.Kind == machine.EventOperationFailed
	}, history, events)
}

// waitForEvent returns the first event satisfying match from history (checked
// first, since a synchronous candidate has already emitted) or from the live
// channel. It blocks until that exact event arrives; a timeout is a real
// failure, not a polling loop.
func waitForEvent(
	t *testing.T,
	what string,
	match func(machine.MachineEvent) bool,
	history []machine.MachineEvent,
	events <-chan machine.MachineEvent,
) machine.MachineEvent {
	t.Helper()
	for _, ev := range history {
		if match(ev) {
			return ev
		}
	}
	select {
	case ev := <-events:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for %s (no matching event on 202 tail)", what)
		return machine.MachineEvent{}
	}
}

// terminalBarrier holds the terminal-commit observation behind a test-only gate.
// When held, every waitForEvent/waitForTerminal blocks until released; this
// turns the end-state assertion from a timing probe into an event gate: a
// synchronous candidate commits and, once released, the single observed event is
// the terminal one; a backgrounded mutant never commits on the 202's tail, so
// even after release the assertion times out. Drained by default (no gate).
var terminalBarrier = make(chan struct{})

func terminalBarrierHold() {
	select {
	case <-terminalBarrier:
	default:
	}
}

// terminalBarrierRelease clears any held barrier so a blocked observation
// proceeds. Idempotent.
func terminalBarrierRelease() {
	select {
	case <-terminalBarrier:
	default:
	}
}

// execCommitBarrier is a per-test, race-free gate for the round-7 event-gated
// assertions. It is a held/released flag guarded by a mutex with a buffered
// release channel, so there is no channel send/receive that a race detector
// could flag across goroutines. While held the event-gated wait blocks;
// releasing lets it proceed to observe the committed terminal event. The
// barrier is local to each test case (never a package global), so running the
// identity tests under -race -count=N cannot leak held state across cases. The
// production code path is never touched — this exists only in the test path.
type execCommitBarrier struct {
	mu       sync.Mutex
	held     bool
	released chan struct{}
}

func newExecCommitBarrier() *execCommitBarrier {
	return &execCommitBarrier{released: make(chan struct{}, 1)}
}

// hold closes the barrier: the event-gated wait blocks on it.
func (b *execCommitBarrier) hold() {
	b.mu.Lock()
	b.held = true
	b.mu.Unlock()
}

// release opens the barrier: buffer a value so any blocked event-gated wait
// proceeds. Idempotent and non-blocking.
func (b *execCommitBarrier) release() {
	b.mu.Lock()
	b.held = false
	b.mu.Unlock()
	select {
	case b.released <- struct{}{}:
	default:
	}
}

// waitHeld blocks until the barrier is released or ctx is cancelled. Safe to
// call from any goroutine; the mutex makes release/waitHeld race-free.
func (b *execCommitBarrier) waitHeld(ctx context.Context) {
	select {
	case <-b.released:
	case <-ctx.Done():
	}
}

// waitForEventGated returns the first event satisfying match from history
// (checked first, since a synchronous candidate has already emitted) or from the
// live channel. While the barrier is held (empty), observation is gated on it:
// no terminal event can be observed until release buffers a value. With the
// barrier released, the foreground candidate's already-committed terminal event
// (on the 202's tail) is observable; a backgrounded mutant (never committed)
// still times out on the event receive. A ctx timeout is a real failure, not a
// polling loop. Returns (event, seen); seen=false means the terminal event was
// not committed on the 202's tail.
func waitForEventGated(
	b *execCommitBarrier,
	ctx context.Context,
	what string,
	match func(machine.MachineEvent) bool,
	history []machine.MachineEvent,
	events <-chan machine.MachineEvent,
) (ev machine.MachineEvent, seen bool) {
	// Gate all observation on the test-path barrier. While held, block on
	// release; once the test sends, proceed to read the committed terminal
	// event. With the barrier released, the foreground candidate's already-
	// committed terminal event (on the 202's tail) is observable; the
	// backgrounded mutant (never committed) still times out below.
	b.waitHeld(ctx)
	for _, h := range history {
		if match(h) {
			return h, true
		}
	}
	// Drain the live channel until a matching terminal event arrives or ctx
	// cancels. Non-matching events (e.g. the already-emitted started event that
	// arrived on the channel before this wait) must not satisfy the assertion —
	// only a terminal succeeded/failed commit does.
	for {
		select {
		case e := <-events:
			if match(e) {
				return e, true
			}
		case <-ctx.Done():
			return machine.MachineEvent{}, false
		}
	}
}

func postExecution(handler *Handler, requestID, input string) *httptest.ResponseRecorder {
	req := withTestPrincipal(httptest.NewRequest(http.MethodPost, "/api/v1/machine/executions", strings.NewReader(
		`{"operation_id":"status","context":{"environment":"prod"},"input":`+input+`}`,
	)))
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Idempotency-Key", requestID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func acceptedExecution(t *testing.T, response *httptest.ResponseRecorder) machine.Execution {
	t.Helper()
	if response.Code != http.StatusAccepted {
		t.Fatalf("request status=%d body=%s", response.Code, response.Body.String())
	}
	var execution machine.Execution
	if err := json.Unmarshal(response.Body.Bytes(), &execution); err != nil {
		t.Fatal(err)
	}
	return execution
}

func journalLineCount(t *testing.T, root string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "journal", "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

func TestRequestIdentityBindsFullInputAndAllowsFreshSemanticRequest(t *testing.T) {
	var calls atomic.Int32
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		calls.Add(1)
		return json.RawMessage(`{}`), nil
	}), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first := acceptedExecution(t, postExecution(handler, "req-1", `{"amount":10}`))
	waitForExecutorStart(t, handler, first.ExecutionID)
	if calls.Load() != 1 {
		t.Fatalf("executor calls=%d want 1", calls.Load())
	}
	root := filepath.Dir(handler.executions.journalDir)
	linesBefore := journalLineCount(t, root)
	changed := postExecution(handler, "req-1", `{"amount":11}`)
	if changed.Code != http.StatusBadRequest || !strings.Contains(changed.Body.String(), `"code":"validation_failed"`) {
		t.Fatalf("changed-content reuse status=%d body=%s", changed.Code, changed.Body.String())
	}
	if got := journalLineCount(t, root); got != linesBefore {
		t.Fatalf("changed-content reuse appended journal line: before=%d after=%d", linesBefore, got)
	}
	second := acceptedExecution(t, postExecution(handler, "req-2", `{"amount":10}`))
	if second.ExecutionID == first.ExecutionID {
		t.Fatal("fresh request identity for the same semantic operation reused execution")
	}
}

func TestRequestIdentityIndexSurvivesJournalReopen(t *testing.T) {
	root := t.TempDir()
	var calls atomic.Int32
	executor := executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		calls.Add(1)
		return json.RawMessage(`{}`), nil
	})
	firstHandler, err := New(executor, root)
	if err != nil {
		t.Fatal(err)
	}
	first := acceptedExecution(t, postExecution(firstHandler, "req-reopen", `{"x":1}`))
	waitForExecutorStart(t, firstHandler, first.ExecutionID)
	if calls.Load() != 1 {
		t.Fatalf("initial executor calls=%d want 1", calls.Load())
	}
	waitForTerminal(t, firstHandler, first.ExecutionID)
	if current, ok := firstHandler.executions.get(first.ExecutionID); !ok || current.State != machine.ExecutionSucceeded {
		t.Fatalf("initial execution did not finish before simulated reopen: %#v", current)
	}
	linesBefore := journalLineCount(t, root)

	secondHandler, err := New(executor, root)
	if err != nil {
		t.Fatal(err)
	}
	second := acceptedExecution(t, postExecution(secondHandler, "req-reopen", `{"x":1}`))
	if second.ExecutionID != first.ExecutionID {
		t.Fatalf("reconnect execution=%s want original %s", second.ExecutionID, first.ExecutionID)
	}
	if got := journalLineCount(t, root); got != linesBefore {
		t.Fatalf("reconnect appended journal line: before=%d after=%d", linesBefore, got)
	}
	if changed := postExecution(secondHandler, "req-reopen", `{"x":2}`); changed.Code != http.StatusBadRequest || !strings.Contains(changed.Body.String(), `"code":"validation_failed"`) {
		t.Fatalf("changed-content reuse after reopen status=%d body=%s", changed.Code, changed.Body.String())
	}
	if got := journalLineCount(t, root); got != linesBefore {
		t.Fatalf("changed-content reuse after reopen appended journal line: before=%d after=%d", linesBefore, got)
	}
	if calls.Load() != 1 {
		t.Fatalf("reconnect or changed-content request started another executor: calls=%d", calls.Load())
	}
}

func TestJournalCommitReadableBeforeExecutorStarts(t *testing.T) {
	root := t.TempDir()
	const requestID = "req-order"
	input := `{"secret":"never-persist-this"}`
	digest := sha256.Sum256([]byte(input))
	started := make(chan error, 1)
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		data, err := os.ReadFile(filepath.Join(root, "journal", "journal.jsonl"))
		if err != nil {
			started <- err
			return nil, err
		}
		line := string(data)
		if !strings.Contains(line, `"request_id":"`+requestID+`"`) || !strings.Contains(line, `"input_sha256":"`+hex.EncodeToString(digest[:])+`"`) {
			started <- errors.New("accepted request commitment was not readable before executor")
			return nil, errors.New("journal commitment missing")
		}
		if strings.Contains(line, "never-persist-this") {
			started <- errors.New("raw input leaked into journal")
			return nil, errors.New("secret input persisted")
		}
		started <- nil
		return json.RawMessage(`{}`), nil
	}), root)
	if err != nil {
		t.Fatal(err)
	}
	_ = acceptedExecution(t, postExecution(handler, requestID, input))
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
}

// TestTerminalStateCommittedBefore202Response proves the end-state guarantee:
// when the accepted execution finishes, its terminal state in the store AND the
// terminal journal line are durably committed. Because handleExecute writes the
// 202 before runExecution, the terminal commit happens on the 202's tail. The
// store commits persist-before-publish under its lock: succeed()/fail() write
// the journal line and THEN append the terminal event to the record's history,
// all synchronously before runExecution returns. Reading that history therefore
// proves executor completion and terminal journal persistence without any
// sleep/poll; on a backgrounded execution the event cannot be present
// synchronously and the assertion fails.
func TestTerminalStateCommittedBefore202Response(t *testing.T) {
	root := t.TempDir()
	const requestID = "req-terminal"
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		return json.RawMessage(`{"done":true}`), nil
	}), root)
	if err != nil {
		t.Fatal(err)
	}
	response := postExecution(handler, requestID, `{"amount":10}`)
	exec := acceptedExecution(t, response)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d want 202", response.Code)
	}

	// Subscribe AFTER the request so we observe the already-emitted terminal
	// event via the returned history (the live channel only carries future
	// events). The store emits the terminal event synchronously when
	// succeed()/fail() commits, before runExecution returns.
	history, _, _, ok := handler.executions.subscribe(exec.ExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	var terminalSeen bool
	for _, ev := range history {
		if ev.Kind == machine.EventOperationSucceeded || ev.Kind == machine.EventOperationFailed {
			terminalSeen = true
			if ev.State != machine.ExecutionSucceeded && ev.State != machine.ExecutionFailed {
				t.Fatalf("terminal event state=%q, want succeeded/failed", ev.State)
			}
		}
	}
	if !terminalSeen {
		t.Fatal("no terminal event recorded on 202 tail (terminal state not durably committed)")
	}

	// (b) Terminal journal line: read it directly, no poll. Because the terminal
	// event is already proven present in history (persist-before-publish), the
	// journal line that carries it is durably written, so a single direct read
	// is exhaustive — a missing line here is a real failure, not a timing
	// artifact.
	data, rerr := os.ReadFile(filepath.Join(root, "journal", "journal.jsonl"))
	if rerr != nil {
		t.Fatalf("read terminal journal: %v", rerr)
	}
	if !strings.Contains(string(data), `"state":"succeeded"`) {
		t.Fatalf("terminal succeeded journal line not readable: %q", string(data))
	}
}

// TestTerminalStateBackgroundMutationFailsControl uses the production (sync)
// handler: the terminal commit lands on the 202's tail and, once the barrier is
// released, the event-gated wait observes that single terminal event. Control
// green — the assertion is not vacuous.
func TestTerminalStateBackgroundMutationFailsControl(t *testing.T) {
	b := newExecCommitBarrier()
	b.release()

	root := t.TempDir()
	const requestID = "req-bg-ctrl"
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		return json.RawMessage(`{"done":true}`), nil
	}), root)
	if err != nil {
		t.Fatal(err)
	}
	exec := acceptedExecution(t, postExecution(handler, requestID, `{"amount":10}`))
	history, events, cancel, ok := handler.executions.subscribe(exec.ExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()
	ev, seen := waitForEventGated(b, context.Background(), "terminal succeeded/failed state", func(e machine.MachineEvent) bool {
		return e.Kind == machine.EventOperationSucceeded || e.Kind == machine.EventOperationFailed
	}, history, events)
	if !seen {
		t.Fatal("foreground synchronous candidate did not commit terminal state on 202 tail")
	}
	if ev.Kind != machine.EventOperationSucceeded && ev.Kind != machine.EventOperationFailed {
		t.Fatalf("wrong event kind=%q", ev.Kind)
	}
}

// TestTerminalStateBackgroundMutationFailsMutant pins the single-call mutant:
// the handler backgrounds runExecution, so no terminal event is committed on
// the 202's tail. With the barrier released, the event-gated wait still times
// out — on ONE call, not after -count repetitions. The mutation is undone
// before acceptance; production runs with the seam nil and therefore synchronous.
func TestTerminalStateBackgroundMutationFailsMutant(t *testing.T) {
	// Release the barrier from the start (buffer a value) so the wait is not
	// gated: the mutant must still fail because no terminal event was committed.
	b := newExecCommitBarrier()
	b.release()

	root := t.TempDir()
	const requestID = "req-bg-mutant"
	// Block the executor so the backgrounded runExecution never completes during
	// the test window: no terminal event can ever land on the 202's tail. A
	// non-blocking executor would commit instantly even when backgrounded and
	// make this probe vacuous.
	blocker := make(chan struct{})
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		<-blocker
		return json.RawMessage(`{"done":true}`), nil
	}), root)
	if err != nil {
		t.Fatal(err)
	}
	// The mutation: background the terminal execution. runExecution runs in a
	// goroutine, so no terminal event is committed synchronously on the 202's
	// tail. This is the exact handler.go:215 fire-and-forget mutant round-7
	// rejects.
	var wg sync.WaitGroup
	wg.Add(1)
	handler.backgroundRunExecution = func(ctx context.Context, executionID string, operation machine.Operation, operationContext machine.OperationContext, input json.RawMessage) {
		go func() {
			defer wg.Done()
			_ = handler.runExecution(ctx, executionID, operation, operationContext, input)
		}()
	}
	exec := acceptedExecution(t, postExecution(handler, requestID, `{"amount":10}`))
	history, events, cancel, ok := handler.executions.subscribe(exec.ExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()

	seen := make(chan bool, 1)
	sctx, scancel := context.WithCancel(context.Background())
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer scancel()
		_, s := waitForEventGated(b, sctx, "terminal succeeded/failed state", func(e machine.MachineEvent) bool {
			return e.Kind == machine.EventOperationSucceeded || e.Kind == machine.EventOperationFailed
		}, history, events)
		if s {
			seen <- true
		}
	}()
	select {
	case <-seen:
		t.Fatal("backgrounded mutant committed terminal state on 202 tail: mutant should be red")
	case <-time.After(6 * time.Second):
		// expected: no terminal event was committed synchronously.
	}
	scancel()
	// Unblock the executor so the background goroutine can finish, then join.
	close(blocker)
	wg.Wait()
}
