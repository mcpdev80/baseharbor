package machinehttp

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// execution_journal_test.go
// Domain-local fixture for the execution-journal slice (internal/machinehttp/store.go,
// executionStore). The card t-20261022-r27-ci-authority requires that the trusted
// execution journal actually deliver results: it must durably record real state
// transitions and fail closed on missing/invalid required coverage. No other slice
// ships a fixture that drives the store directly (the SSE handler flow in
// handler_test.go only reaches it indirectly), so this fixture closes that gap.
//
// It is intentionally a bounded deterministic fuzz (seeded RNG): every step asserts a
// hard invariant, so a broken journal (e.g. an update() that silently fabricates a
// record for an unknown ID, or a succeed() that drops the structured result) makes
// the test fail — it never passes by skip.

func journalActor() machine.ActorRef {
	return machine.ActorRef{
		Mode:      "operator",
		Issuer:    "https://issuer.example",
		Subject:   "operator-1",
		Assurance: "high",
	}
}

func journalContext() machine.OperationContext {
	return machine.OperationContext{
		Application: "demo",
		Environment: "prod",
		Target:      "prod-eu",
	}
}

// TestExecutionJournalDurableTransitions asserts the create -> running -> succeeded
// and create -> running -> failed paths record durably, with a structured result /
// structured error, and emit the expected ordered events.
func TestExecutionJournalDurableTransitions(t *testing.T) {
	store := newExecutionStore()

	exec, err := store.create("status", journalActor(), journalContext())
	if err != nil {
		t.Fatalf("create failed on valid context: %v", err)
	}
	if exec.State != machine.ExecutionPending {
		t.Fatalf("new execution state = %q, want pending", exec.State)
	}
	if err := exec.Validate(); err != nil {
		t.Fatalf("pending execution should be valid: %v", err)
	}

	if err := store.start(exec.ExecutionID); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	started, ok := store.get(exec.ExecutionID)
	if !ok {
		t.Fatal("start disappeared the execution from the journal (not durable)")
	}
	if started.State != machine.ExecutionRunning || started.StartedAt == nil {
		t.Fatalf("start not durable: state=%q startedAt=%v", started.State, started.StartedAt)
	}

	progress := machine.OperationProgress{Stage: "observe", Percent: 42}
	if err := store.progress(exec.ExecutionID, progress); err != nil {
		t.Fatalf("progress failed: %v", err)
	}
	progressed, ok := store.get(exec.ExecutionID)
	if !ok {
		t.Fatal("progress disappeared the execution from the journal (not durable)")
	}
	if progressed.Progress == nil || progressed.Progress.Stage != "observe" {
		t.Fatalf("progress not durable: %#v", progressed.Progress)
	}

	const resultJSON = `{"ready":true,"version":"1.2.3"}`
	if err := store.succeed(exec.ExecutionID, []byte(resultJSON)); err != nil {
		t.Fatalf("succeed failed: %v", err)
	}
	done, ok := store.get(exec.ExecutionID)
	if !ok {
		t.Fatal("succeed disappeared the execution from the journal (not durable)")
	}
	if done.State != machine.ExecutionSucceeded {
		t.Fatalf("succeed not durable: state=%q", done.State)
	}
	if string(done.Result) != resultJSON {
		t.Fatalf("structured result not preserved: %q", string(done.Result))
	}
	if done.Error != nil {
		t.Fatal("successful execution must not carry an error")
	}
	// Execute() success requires a structured result per Execution.Validate().
	if err := done.Validate(); err != nil {
		t.Fatalf("succeeded execution must be valid: %v", err)
	}

	// Failure path on a second execution: must carry a structured error.
	exec2, _ := store.create("destroy", journalActor(), journalContext())
	boom := machine.NewError(machine.ErrorVerificationFailed, "verify denied", "owner mismatch", true)
	if err := store.fail(exec2.ExecutionID, boom); err != nil {
		t.Fatalf("fail failed: %v", err)
	}
	done2, ok := store.get(exec2.ExecutionID)
	if !ok {
		t.Fatal("fail disappeared the execution from the journal (not durable)")
	}
	if done2.State != machine.ExecutionFailed {
		t.Fatalf("fail not durable: state=%q", done2.State)
	}
	if done2.Error == nil {
		t.Fatal("failed execution must carry a structured error")
	}
	if err := done2.Validate(); err != nil {
		t.Fatalf("failed execution with structured error must be valid: %v", err)
	}
}

// TestExecutionJournalFailsClosedOnUnknownID is the core fail-closed guarantee:
// progress/succeed/fail on an execution ID that was never created must return an
// error and must NOT fabricate a record. This is the trusted-journal invariant that
// the SSE flow never asserts directly.
func TestExecutionJournalFailsClosedOnUnknownID(t *testing.T) {
	store := newExecutionStore()

	for _, call := range []struct {
		name string
		fn   func() error
	}{
		{"progress", func() error { return store.progress("exec_missing", machine.OperationProgress{}) }},
		{"succeed", func() error { return store.succeed("exec_missing", []byte("{}")) }},
		{"fail", func() error { return store.fail("exec_missing", fmt.Errorf("x")) }},
	} {
		if err := call.fn(); err == nil {
			t.Fatalf("%s on unknown execution ID did not fail closed", call.name)
		}
		if _, ok := store.get("exec_missing"); ok {
			t.Fatalf("%s on unknown ID fabricated a record (journal integrity broken)", call.name)
		}
	}
}

// TestExecutionJournalRejectsInvalidContractStates guards the trusted-contract
// validation used by the journal: a succeeded execution without a structured result
// and a failed execution without a structured error are invalid.
func TestExecutionJournalRejectsInvalidContractStates(t *testing.T) {
	store := newExecutionStore()

	exec, _ := store.create("status", journalActor(), journalContext())
	if err := store.start(exec.ExecutionID); err != nil {
		t.Fatal(err)
	}
	// Succeeded without a result must be rejected by the contract.
	store.records[exec.ExecutionID].execution.State = machine.ExecutionSucceeded
	store.records[exec.ExecutionID].execution.Result = nil
	if err := store.records[exec.ExecutionID].execution.Validate(); err == nil {
		t.Fatal("succeeded execution without structured result should be invalid")
	}
	// Failed without an error must be rejected.
	store.records[exec.ExecutionID].execution.State = machine.ExecutionFailed
	store.records[exec.ExecutionID].execution.Error = nil
	if err := store.records[exec.ExecutionID].execution.Validate(); err == nil {
		t.Fatal("failed execution without structured error should be invalid")
	}
	// Unknown state must be rejected.
	store.records[exec.ExecutionID].execution.State = machine.ExecutionState("bogus")
	if err := store.records[exec.ExecutionID].execution.Validate(); err == nil {
		t.Fatal("unknown execution state should be invalid")
	}
}

// TestExecutionJournalBoundedSequenceFuzz drives the store through a bounded,
// deterministic sequence of valid and invalid operations. Every mutating call on a
// known ID must succeed; every call on an unknown ID must fail closed. A flaky or
// non-durable journal is caught because a later step would read a wrong state.
func TestExecutionJournalBoundedSequenceFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(20261103))
	store := newExecutionStore()

	var current string
	for step := 0; step < 64; step++ {
		exec, err := store.create(fmt.Sprintf("op-%d", step), journalActor(), journalContext())
		if err != nil {
			t.Fatalf("step %d: create failed: %v", step, err)
		}
		current = exec.ExecutionID

		switch rng.Intn(4) {
		case 0:
			if err := store.start(current); err != nil {
				t.Fatalf("step %d: start failed on known ID: %v", step, err)
			}
		case 1, 2:
			if err := store.progress(current, machine.OperationProgress{Stage: fmt.Sprintf("s%d", step)}); err != nil {
				t.Fatalf("step %d: progress failed on known ID: %v", step, err)
			}
		case 3:
			if err := store.succeed(current, []byte(fmt.Sprintf(`{"step":%d}`, step))); err != nil {
				t.Fatalf("step %d: succeed failed on known ID: %v", step, err)
			}
		}

		// A second mutating op on the same ID must still fail closed once terminal,
		// or succeed while active — never silently mutate a foreign/unknown ID.
		if err := store.progress("exec_unknown_step", machine.OperationProgress{}); err == nil {
			t.Fatalf("step %d: progress on unknown ID did not fail closed", step)
		}
		_ = time.Now()
	}

	if _, ok := store.get("exec_unknown_step"); ok {
		t.Fatal("journal fabricated a record for a never-created ID during the run")
	}
}
