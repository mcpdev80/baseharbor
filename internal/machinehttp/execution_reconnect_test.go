package machinehttp

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestReconnectReturnsSameOperationProvesRed proves criterion (3): reconnecting
// with the same operation id must return the SAME execution (same execution id)
// instead of minting a second execution, so the operation is replayed once.
//
// Red first: this currently FAILS because there is no reconnect path at all —
// create() always mints a fresh execution id, so a "reconnect" yields a brand
// new execution id rather than the original.
func TestReconnectReturnsSameOperationProvesRed(t *testing.T) {
	store := newExecutionStore()
	actor := machine.ActorRef{Mode: "trusted", Subject: "operator-a"}
	ctx := machine.OperationContext{Application: "billing", Environment: "prod", Target: "t1"}

	original, err := store.create("charge", actor, ctx)
	if err != nil {
		t.Fatalf("initial create failed: %v", err)
	}

	// Reconnect with the same operation id and same actor/context.
	reconnect, err := store.create("charge", actor, ctx)
	if err != nil {
		t.Fatalf("reconnect create must not error for a repeat of the same operation, got: %v", err)
	}

	if reconnect.ExecutionID != original.ExecutionID {
		t.Fatalf("reconnect must return the same execution id (%s), got %s", original.ExecutionID, reconnect.ExecutionID)
	}
	if len(store.records) != 1 {
		t.Fatalf("reconnect must not create a second execution, have %d, want 1", len(store.records))
	}
}
