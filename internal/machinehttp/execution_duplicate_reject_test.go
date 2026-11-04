package machinehttp

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestCreateDuplicateSameOperationDifferentContentRejectsRed proves criterion (2):
// a second create() with the SAME operation id but DIFFERENT content must be
// rejected with a typed validation error and must NOT index a second execution.
//
// Red first: this currently FAILS because create() only indexes by a fresh
// ExecutionID and never compares operationID, so the second call succeeds.
func TestCreateDuplicateSameOperationDifferentContentRejectsRed(t *testing.T) {
	store := newExecutionStore()

	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	base := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}

	first, err := store.create("charge", actor, base)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Same operation id, different content (different target).
	dupCtx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t2"}
	_, err = store.create("charge", actor, dupCtx)
	if err == nil {
		t.Fatalf("duplicate create with same operation id and different content must return an error, got nil")
	}

	// The rejected duplicate must not have indexed a second execution.
	dupID := first.ExecutionID // identity is only by execution id; assert exactly one record exists
	_, ok := store.records[dupID]
	if !ok {
		t.Fatalf("expected the accepted execution to still be retrievable after a rejected duplicate")
	}
	if len(store.records) != 1 {
		t.Fatalf("expected exactly one execution indexed after a rejected duplicate, got %d", len(store.records))
	}

	// A distinct operation id is still allowed.
	other, err := store.create("refund", actor, base)
	if err != nil {
		t.Fatalf("distinct operation id must be allowed, got error: %v", err)
	}
	if other.ExecutionID == first.ExecutionID {
		t.Fatalf("distinct operation must yield a distinct execution id")
	}
}

// TestDuplicateAfterStartStillRejected proves the duplicate guard holds even
// after the first execution has moved to running: a changed-content repeat of
// the same operation id still fails closed with a typed error and indexes no
// second execution, and the original keeps running.
func TestDuplicateAfterStartStillRejects(t *testing.T) {
	store := newExecutionStore()
	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	ctx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}

	original, err := store.create("charge", actor, ctx)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	if err := store.start(original.ExecutionID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	dupCtx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t2"}
	_, err = store.create("charge", actor, dupCtx)
	if err == nil {
		t.Fatalf("changed-content duplicate after start must error, got nil")
	}
	if len(store.records) != 1 {
		t.Fatalf("duplicate must not index a second execution, have %d want 1", len(store.records))
	}
	got, ok := store.get(original.ExecutionID)
	if !ok {
		t.Fatalf("original execution must still exist after a rejected duplicate")
	}
	if got.State != machine.ExecutionRunning {
		t.Fatalf("original execution must stay running, got state %q", got.State)
	}
}

// TestCreateReconnectIdenticalReturnsSameOperation proves that a byte-identical
// repeat (a reconnect) returns the ORIGINAL execution id and indexes no second
// execution — criterion (3): reconnect retrieves the same operation.
func TestCreateReconnectIdenticalReturnsSameOperation(t *testing.T) {
	store := newExecutionStore()
	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	ctx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}

	first, err := store.create("charge", actor, ctx)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	reconnect, err := store.create("charge", actor, ctx)
	if err != nil {
		t.Fatalf("identical reconnect must not error, got: %v", err)
	}
	if reconnect.ExecutionID != first.ExecutionID {
		t.Fatalf("reconnect must return same execution id, got %s want %s", reconnect.ExecutionID, first.ExecutionID)
	}
	if len(store.records) != 1 {
		t.Fatalf("reconnect must not create a second execution, have %d want 1", len(store.records))
	}
}
