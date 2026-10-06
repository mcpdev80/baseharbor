package operatorauth

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestAuthorizeMachineOperationTrustedLocalAndManagedFailClosed(t *testing.T) {
	operation, ok := machine.OperationByID("apply")
	if !ok {
		t.Fatal("apply operation is not registered")
	}

	devCtx := WithEnforcement(context.Background())
	dev, err := AuthorizeMachineOperation(devCtx, AuthorizationRequest{
		Operation: operation,
		Context:   OperationContext{Application: "demo", Environment: "dev", Target: "local"},
	})
	if err != nil {
		t.Fatalf("dev authorization failed: %v", err)
	}
	if !dev.Allowed || dev.Actor.Mode != "trusted-local" || dev.Actor.Subject != "trusted-local" {
		t.Fatalf("unexpected dev decision: %#v", dev)
	}
	if dev.Safety != machine.SafetyMutating || !dev.PolicyRequired {
		t.Fatalf("machine safety metadata was not preserved: %#v", dev)
	}
	storedDev, ok := AuthorizationDecisionFromContext(devCtx)
	if !ok || storedDev.Actor.Subject != "trusted-local" || !storedDev.Allowed {
		t.Fatalf("trusted-local decision was not retained: %#v", storedDev)
	}

	managedCtx := WithEnforcement(context.Background())
	managed, err := AuthorizeMachineOperation(managedCtx, AuthorizationRequest{
		Operation: operation,
		Context:   OperationContext{Application: "demo", Environment: "prod", Target: "prod-eu"},
	})
	if err == nil {
		t.Fatal("managed authorization unexpectedly succeeded without principal")
	}
	if managed.Allowed || managed.ReasonCode != "operator_authentication_required" {
		t.Fatalf("managed decision did not fail closed: %#v", managed)
	}
	classified := machine.Classify(err)
	if classified.Code != machine.ErrorAuthenticationFailed {
		t.Fatalf("managed denial code = %q", classified.Code)
	}
	storedManaged, ok := AuthorizationDecisionFromContext(managedCtx)
	if !ok || storedManaged.Allowed || storedManaged.ReasonCode != "operator_authentication_required" {
		t.Fatalf("managed denial decision was not retained: %#v", storedManaged)
	}
}

func TestAuthorizeMachineOperationReturnsSecretSafeStableActor(t *testing.T) {
	operation, ok := machine.OperationByID("destroy")
	if !ok {
		t.Fatal("destroy operation is not registered")
	}

	ctx := WithEnforcement(context.Background())
	setPrincipal(ctx, &identity.Principal{
		Issuer:    "https://issuer.example",
		Subject:   "user-123",
		Audience:  []string{"baseharbor"},
		Assurance: "urn:mfa",
		Methods:   []string{"pwd", "otp"},
	})

	decision, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{
		Operation: operation,
		Context:   OperationContext{Application: "demo", Environment: "prod", Target: "prod-eu"},
	})
	if err != nil {
		t.Fatalf("authorization failed: %v", err)
	}
	if !decision.Allowed || decision.Actor.Mode != "authenticated" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.Actor.Issuer != "https://issuer.example" || decision.Actor.Subject != "user-123" {
		t.Fatalf("stable actor fields missing: %#v", decision.Actor)
	}
	if decision.Safety != machine.SafetyDestructive || !decision.PolicyRequired || !decision.ConfirmationRequired {
		t.Fatalf("operation metadata mismatch: %#v", decision)
	}
	stored, ok := AuthorizationDecisionFromContext(ctx)
	if !ok || stored.Actor.Subject != "user-123" || stored.Actor.Issuer != "https://issuer.example" {
		t.Fatalf("authenticated decision was not retained: %#v", stored)
	}
}

func TestVerifiedActorIsPreservedForDevelopmentEnvironment(t *testing.T) {
	operation, _ := machine.OperationByID("apply")
	ctx := WithVerifiedPrincipal(context.Background(), &identity.Principal{Issuer: "https://issuer.example", Subject: "operator-a"})
	decision, err := AuthorizeMachineOperation(ctx, AuthorizationRequest{Operation: operation, Context: OperationContext{Environment: "dev", Target: "local"}})
	if err != nil || decision.Actor.Mode != "authenticated" || decision.Actor.Subject != "operator-a" {
		t.Fatal("verified HTTP/MCP actor collapsed into shared trusted-local identity", err)
	}
}
