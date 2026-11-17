package machinehttp

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestReconnectReturnsSameOperation covers a retained request identity. It does
// not guarantee exactly-once external effects across crashes.
func TestReconnectReturnsSameOperation(t *testing.T) {
	store := newExecutionStore()
	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	ctx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}
	original, _, err := store.createRequest("request-reconnect", "charge", actor, ctx, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("initial create failed: %v", err)
	}
	reconnect, created, err := store.createRequest("request-reconnect", "charge", actor, ctx, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("reconnect create failed: %v", err)
	}
	if created || reconnect.ExecutionID != original.ExecutionID {
		t.Fatalf("reconnect result=%#v created=%t; want original execution", reconnect, created)
	}
	if len(store.records) != 1 {
		t.Fatalf("reconnect indexed a second execution: have %d want 1", len(store.records))
	}
}
