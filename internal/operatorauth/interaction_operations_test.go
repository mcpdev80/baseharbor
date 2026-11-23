package operatorauth

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Card 0428-063: the guided app logs/exec/urls operation categories get
// registered machine identities so the CLI seams authorize fail-closed against
// the same policy core as every other operation. These tests pin the registry
// semantics, mirroring the 0428-057 secret.reveal precedent.

func TestInteractionOperationsAreRegisteredWithFailClosedSemantics(t *testing.T) {
	// The machine tool boundary of these categories is pinned (excluded) by
	// TestInteractionOperationsFailClosedOnManagedBoundaries; here the safety
	// metadata stays truthful.
	logs, ok := machine.OperationByID("app.logs")
	if !ok {
		t.Fatal("app.logs operation is not registered")
	}
	if logs.Safety != machine.SafetyReadOnly {
		t.Fatalf("app.logs must be read-only, got %s", logs.Safety)
	}
	execOp, ok := machine.OperationByID("app.exec")
	if !ok {
		t.Fatal("app.exec operation is not registered")
	}
	if execOp.Safety != machine.SafetyMutating {
		t.Fatalf("app.exec must be mutating, got %s", execOp.Safety)
	}
	urls, ok := machine.OperationByID("app.urls")
	if !ok {
		t.Fatal("app.urls operation is not registered")
	}
	if urls.Safety != machine.SafetyReadOnly {
		t.Fatalf("app.urls must be read-only, got %s", urls.Safety)
	}
}

func TestInteractionOperationsFailClosedOnManagedBoundaries(t *testing.T) {
	for _, id := range []string{"app.logs", "app.exec", "app.urls"} {
		operation, ok := machine.OperationByID(id)
		if !ok {
			t.Fatalf("%s operation is not registered", id)
		}
		// Guidance categories stay outside the MCP tool boundary by design
		// (streams, host exec, human presentation); typed clients keep using
		// the semantic status/doctor/evidence surfaces.
		if operation.MCPTool != "" {
			t.Fatalf("%s must remain excluded from the MCP tool boundary, got %q", id, operation.MCPTool)
		}
		ctx := WithEnforcement(context.Background())
		decision, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
			Operation: operation,
			Context:   OperationContext{Application: "demo", Environment: "prod", Target: "prod-eu"},
		})
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded on a managed boundary without a principal", id)
		}
		if decision.Allowed || decision.ReasonCode != "operator_authentication_required" {
			t.Fatalf("%s did not fail closed: %#v", id, decision)
		}
		if classified := machine.Classify(err); classified.Code != machine.ErrorAuthenticationFailed {
			t.Fatalf("%s denial code = %q", id, classified.Code)
		}
	}
}
