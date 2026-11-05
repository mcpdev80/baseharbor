package machinehttp

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestCreateDuplicateRequestDifferentContentRejects(t *testing.T) {
	store := newExecutionStore()
	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	base := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}
	first, created, err := store.createRequest("request-1", "charge", actor, base, []byte(`{"amount":10}`))
	if err != nil || !created {
		t.Fatalf("first create failed: %v", err)
	}

	dupCtx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t2"}
	_, _, err = store.createRequest("request-1", "charge", actor, dupCtx, []byte(`{"amount":11}`))
	if machine.Classify(err).Code != machine.ErrorValidationFailed {
		t.Fatalf("changed request identity must return typed validation error, got %v", err)
	}
	if _, ok := store.get(first.ExecutionID); !ok {
		t.Fatal("accepted execution is no longer retrievable after rejected duplicate")
	}
	if len(store.records) != 1 {
		t.Fatalf("rejected duplicate indexed another execution: have %d want 1", len(store.records))
	}
	other, created, err := store.createRequest("request-2", "charge", actor, base, []byte(`{"amount":10}`))
	if err != nil || !created {
		t.Fatalf("fresh request identity for the same operation must be allowed: %v", err)
	}
	if other.ExecutionID == first.ExecutionID {
		t.Fatal("fresh request identity reused the previous execution")
	}
}

func TestDuplicateAfterStartStillRejects(t *testing.T) {
	store := newExecutionStore()
	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	ctx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}
	original, _, err := store.createRequest("request-running", "charge", actor, ctx, []byte(`{"target":"t1"}`))
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	if err := store.start(original.ExecutionID); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	dupCtx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t2"}
	_, _, err = store.createRequest("request-running", "charge", actor, dupCtx, []byte(`{"target":"t2"}`))
	if machine.Classify(err).Code != machine.ErrorValidationFailed {
		t.Fatalf("changed-content duplicate after start must return typed validation error, got %v", err)
	}
	if len(store.records) != 1 {
		t.Fatalf("duplicate indexed another execution: have %d want 1", len(store.records))
	}
	got, ok := store.get(original.ExecutionID)
	if !ok || got.State != machine.ExecutionRunning {
		t.Fatalf("original execution must stay running, got %#v", got)
	}
}

func TestCreateReconnectIdenticalReturnsSameOperation(t *testing.T) {
	store := newExecutionStore()
	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	ctx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}
	first, _, err := store.createRequest("request-reconnect", "charge", actor, ctx, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	reconnect, created, err := store.createRequest("request-reconnect", "charge", actor, ctx, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("identical reconnect must not error, got: %v", err)
	}
	if created {
		t.Fatal("identical request was marked as a new execution")
	}
	if reconnect.ExecutionID != first.ExecutionID {
		t.Fatalf("reconnect execution id=%s want %s", reconnect.ExecutionID, first.ExecutionID)
	}
	if len(store.records) != 1 {
		t.Fatalf("reconnect indexed a second execution: have %d want 1", len(store.records))
	}
}
