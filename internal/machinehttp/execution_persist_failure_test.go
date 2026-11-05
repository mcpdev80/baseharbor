package machinehttp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestRunExecutionPropagatesTerminalPersistFailure (review finding 2, round 4):
// a write/sync/close failure on the terminal transition (succeed) must NOT be
// swallowed by runExecution. The store rolls the execution back to a non-terminal
// state and returns the error; runExecution must propagate that error to its
// caller rather than returning success for a write that could not be durably
// committed.
//
// Deterministic driver: the executor commits `start` (the handler calls
// runExecution synchronously, so start runs before the executor body), then
// corrupts the journal path (replaces the journal directory with a regular file,
// so the terminal succeed() append Open fails with ENOTDIR). The failing
// transition is therefore the terminal succeed, not start. On the corrected
// candidate runExecution returns the terminal persist error and leaves the
// execution in the non-terminal running state; on the pre-round-3 candidate
// (which swallows the terminal error) runExecution returns nil and the test
// fails.
func TestRunExecutionPropagatesTerminalPersistFailure(t *testing.T) {
	root := t.TempDir()
	handler, err := New(executorFunc(func(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error) {
		// start has already committed by the time the executor runs; break the
		// journal so the terminal succeed() append fails closed.
		journalDir := filepath.Join(root, "journal")
		if derr := os.RemoveAll(journalDir); derr != nil {
			_ = derr
		}
		_ = os.WriteFile(journalDir, []byte("not a dir"), 0o600)
		return json.RawMessage(`{"done":true}`), nil
	}), root)
	if err != nil {
		t.Fatal(err)
	}

	exec, created, cErr := handler.executions.createRequest("req-persistfail", "status", journalActor(), journalContext(), []byte(`{"amount":10}`))
	if cErr != nil {
		t.Fatalf("createRequest: %v", cErr)
	}
	if !created {
		t.Fatal("expected a fresh execution")
	}

	runErr := handler.runExecution(context.Background(), exec.ExecutionID, machineOp("status"), journalContext(), []byte(`{"amount":10}`))
	if runErr == nil {
		t.Fatal("FAIL (finding 2): runExecution with an un-writable terminal did not return a persist error")
	}

	// The execution must NOT be left in a terminal succeeded state — the store
	// rolled it back on the failed append.
	current, ok := handler.executions.get(exec.ExecutionID)
	if !ok {
		t.Fatal("execution missing after failed terminal persist")
	}
	if current.State == machine.ExecutionSucceeded {
		t.Fatalf("FAIL (finding 2): execution left in terminal succeeded state after persist failure: %q", current.State)
	}
}

// machineOp returns a minimal operation handle for the status operation used by
// the handler-level persist-failure driver.
func machineOp(id string) machine.Operation {
	op, ok := machine.OperationByID(id)
	if !ok {
		// Fall back to the first supported operation for the test seam.
		for _, o := range machine.Operations() {
			return o
		}
	}
	return op
}
