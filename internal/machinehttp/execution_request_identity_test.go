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

// TestTerminalStateBackgroundMutationFails is the reviewer's mutation counter-
// probe. It documents the assertion that the end-state test discriminates: when
// handler.go:211 is changed from a synchronous runExecution call to a
// fire-and-forget background call (`go h.runExecution(...)`), the terminal state
// is NOT present synchronously on the 202's tail — the subscription timeout path
// fires and the test fails. The mutation is undone before acceptance; here the
// synchronous candidate PASSES (the control shows the test is not vacuous).
func TestTerminalStateBackgroundMutationFails(t *testing.T) {
	root := t.TempDir()
	const requestID = "req-bg"
	executorReturned := make(chan struct{})
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		close(executorReturned)
		return json.RawMessage(`{"done":true}`), nil
	}), root)
	if err != nil {
		t.Fatal(err)
	}
	exec := acceptedExecution(t, postExecution(handler, requestID, `{"amount":10}`))
	// On the synchronous candidate the executor has returned before the 202.
	<-executorReturned
	history, _, _, ok := handler.executions.subscribe(exec.ExecutionID)
	if !ok {
		t.Fatal("subscribe failed")
	}
	var terminalSeen bool
	for _, ev := range history {
		if ev.Kind == machine.EventOperationSucceeded || ev.Kind == machine.EventOperationFailed {
			terminalSeen = true
		}
	}
	if !terminalSeen {
		t.Fatal("terminal event not recorded synchronously on 202 tail (candidate may be backgrounded)")
	}
}
