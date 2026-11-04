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
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
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
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("initial executor calls=%d want 1", calls.Load())
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if current, ok := firstHandler.executions.get(first.ExecutionID); ok && current.State == machine.ExecutionSucceeded {
			break
		}
		time.Sleep(time.Millisecond)
	}
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
