package operatorauth

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestRequireHumanOperatorRuntimeIdentityFailsClosed proves the human-authorization
// seam: a reveal must fail when only the runtime/trusted-local identity is
// present (no verified operator), and must succeed with a verified operator
// principal. This is the core (a) acceptance for the secret slice.
func TestRequireHumanOperatorRuntimeIdentityFailsClosed(t *testing.T) {
	// (a1) No principal at all (the trusted-local / local-runtime path that
	// authorizes ordinary application operations) must fail closed.
	ctx := WithEnforcement(context.Background())
	if err := RequireHumanOperator(ctx); err == nil {
		t.Fatal("RequireHumanOperator allowed a reveal with no operator principal")
	} else {
		classified := machine.Classify(err)
		if classified.Code != machine.ErrorAuthenticationFailed {
			t.Fatalf("runtime-identity reveal denial code = %q, want %q", classified.Code, machine.ErrorAuthenticationFailed)
		}
	}

	// (a2) An unverified principal must also fail closed.
	unverified := WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "unverified"})
	if err := RequireHumanOperator(unverified); err == nil {
		t.Fatal("RequireHumanOperator allowed a reveal with an unverified principal")
	}

	// (a3) A verified operator principal must succeed.
	verified := WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "operator", IssuerVerified: true})
	if err := RequireHumanOperator(verified); err != nil {
		t.Fatalf("RequireHumanOperator rejected a verified operator: %v", err)
	}
}

// TestRequireHumanOperatorRecordsRevealDecisionUnderDistinctOp ensures the human-
// authorization decision is traced under the secret.reveal machine op id.
func TestRequireHumanOperatorRecordsRevealDecisionUnderDistinctOp(t *testing.T) {
	ctx := WithEnforcement(context.Background())
	_ = RequireHumanOperator(ctx)
	decision, ok := AuthorizationDecisionFromContext(ctx)
	if !ok {
		t.Fatal("no authorization decision was recorded for a reveal")
	}
	if decision.Operation != "secret.reveal" {
		t.Fatalf("recorded reveal operation = %q, want secret.reveal", decision.Operation)
	}
}

// TestRequireHumanOperatorScopedToRevealKeepsOtherOperationsUnchanged ensures the
// gate is scoped to secret.reveal: ordinary local-runtime operations (apply,
// app.environment) still authorize in local/dev, so workload reads stay
// independently allowed.
func TestRequireHumanOperatorScopedToRevealKeepsOtherOperationsUnchanged(t *testing.T) {
	operation, ok := machine.OperationByID("apply")
	if !ok {
		t.Fatal("apply operation is not registered")
	}
	ctx := WithEnforcement(context.Background())
	dev, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: operation,
		Context:   OperationContext{Application: "demo", Environment: "dev", Target: "local"},
	})
	if err != nil {
		t.Fatalf("apply in dev should still authorize via trusted-local: %v", err)
	}
	if !dev.Allowed || dev.Actor.Mode != "trusted-local" {
		t.Fatalf("apply decision changed: %#v", dev)
	}
}
