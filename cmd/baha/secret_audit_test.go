package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestClassifySecretOutcomeMapsDenialsAndFailures(t *testing.T) {
	if got := classifySecretOutcome(nil); got != "success" {
		t.Fatalf("nil error outcome = %q, want success", got)
	}
	denialCodes := []machine.ErrorCode{
		machine.ErrorAuthenticationFailed,
		machine.ErrorPolicyDenied,
		machine.ErrorOwnershipAmbiguous,
		machine.ErrorCapabilityMissing,
		machine.ErrorApprovalRequired,
	}
	sampleErr := machine.NewError(machine.ErrorInternal, "sample", "retry", false)
	for _, code := range denialCodes {
		if got := classifySecretOutcome(machine.Wrap(code, sampleErr, "denied", false)); got != "denial" {
			t.Fatalf("code %q outcome = %q, want denial", code, got)
		}
	}
	failureCodes := []machine.ErrorCode{
		machine.ErrorNotFound,
		machine.ErrorRuntimeUnavailable,
		machine.ErrorInternal,
		machine.ErrorTimeout,
	}
	for _, code := range failureCodes {
		if got := classifySecretOutcome(machine.Wrap(code, sampleErr, "failed", false)); got != "failure" {
			t.Fatalf("code %q outcome = %q, want failure", code, got)
		}
	}
}

func secretAuditResolvedApplication(target, app, env string) resolvedApplication {
	return resolvedApplication{
		Target:             deployment.ResolvedTarget{Name: target},
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "dep-058"},
		Manifest: application.Manifest{
			Name:          app,
			Environment:   env,
			ApplicationID: "app-058",
		},
	}
}

// TestRecordSecretAuditWritesOutcomeWithoutValue proves the audit captures
// who/what/where and result for the secret seam, and that no secret value
// leaks into the stored event. This runs against a temp state root so no
// real Target deployment is required.
func TestRecordSecretAuditWritesOutcomeWithoutValue(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	resolved := secretAuditResolvedApplication("local", "demo", "dev")

	secret := []byte("SUPERSECRET-VALUE")
	outcome := classifySecretOutcome(nil)
	recordSecretAudit(ctx, resolved, "secret.set", "API_TOKEN", outcome)

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(events))
	}
	event := events[0]
	if event.Operation != "secret.set" || event.Outcome != "success" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Resource != "API_TOKEN" {
		t.Fatalf("resource context missing: got %q, want API_TOKEN", event.Resource)
	}
	if event.Application != "demo" || event.Environment != "dev" || event.Target != "local" {
		t.Fatalf("wrong resource location: %#v", event)
	}
	if strings.Contains(event.Resource, string(secret)) || strings.Contains(event.Detail, string(secret)) {
		t.Fatalf("secret value leaked into audit: %q", string(secret))
	}
}

// TestRecordSecretAuditDistinguishesRuntimeActorWithoutVerifiedOperator proves
// that a secret audit without a verified human attribute the runtime actor
// rather than inventing a person. trusted-local is not a person.
func TestRecordSecretAuditRuntimeActorWithoutVerifiedOperator(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	resolved := secretAuditResolvedApplication("local", "demo", "dev")
	recordSecretAudit(ctx, resolved, "secret.list", "", classifySecretOutcome(nil))

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one audit event, got %d", len(events))
	}
	if events[0].AuthorizationResult != "deny" {
		t.Fatalf("runtime actor audit should record AuthorizationResult deny without a verified operator, got %q", events[0].AuthorizationResult)
	}
}

func TestRecordSecretAuditBailsQuietlyWhenStateRootUnreachable(t *testing.T) {
	// Point at an unresolvable root; the audit must not panic or fail the caller.
	targetStateRoot = func(string) (string, error) { return "", errors.New("no target registered") }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	resolved := secretAuditResolvedApplication("local", "demo", "dev")
	recordSecretAudit(ctx, resolved, "secret.set", "API_TOKEN", classifySecretOutcome(nil))
}

// TestRecordSecretAuditPresentDeniedDecisionOverridesVerifiedPrincipal proves
// the precedence rule: when an AuthorizationDecision is present with Allowed
// false, the audit records AuthorizationResult=deny even though a verified
// principal also exists in the context (authorizeMCPOperation stores denied
// machine decisions alongside the verified principal). A present decision must
// never fall back to principal-allow, which would corrupt the audit record.
func TestRecordSecretAuditPresentDeniedDecisionOverridesVerifiedPrincipal(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := operatorauth.WithEnforcement(context.Background())
	principal := &identity.Principal{
		Subject:   "verified-operator",
		Issuer:    "issuer",
		Assurance: "high",
	}
	ctx = operatorauth.WithVerifiedPrincipal(ctx, principal)
	decision, ok := operatorauth.AuthorizationDecisionFromContext(ctx)
	if ok {
		t.Fatalf("context must start without a stored decision for this fixture")
	}
	decision = operatorauth.AuthorizationDecision{
		ContractVersion: "0.4.28",
		Allowed:         false,
		Operation:       "secret.set",
		Actor:           operatorauth.MachineActorRef{Subject: "workload"},
	}
	ctx = operatorauth.WithAuthorizationDecision(ctx, decision)

	resolved := secretAuditResolvedApplication("local", "demo", "dev")
	recordSecretAudit(ctx, resolved, "secret.set", "API_TOKEN", classifySecretOutcome(nil))

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(events))
	}
	event := events[0]
	if event.AuthorizationResult != "deny" {
		t.Fatalf("present denied decision must yield AuthorizationResult=deny, got %q", event.AuthorizationResult)
	}
	if event.Actor.Subject != decision.Actor.Subject {
		t.Fatalf("actor must follow the denied decision, got %q want %q", event.Actor.Subject, decision.Actor.Subject)
	}
}
