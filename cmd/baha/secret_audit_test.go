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
// that a secret audit without a verified human operator attributes the runtime
// actor (trusted-local, not the cli/local-operator person default) and, because
// the operation succeeded, records AuthorizationResult=allow. trusted-local is
// not a person, and a succeeded local workload operation is an allow, not a
// deny.
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
	event := events[0]
	if event.Actor.Identity != "trusted-local" {
		t.Fatalf("runtime actor audit must attribute the runtime actor trusted-local, got %q", event.Actor.Identity)
	}
	if event.AuthorizationResult != "allow" {
		t.Fatalf("succeeded runtime op must record AuthorizationResult=allow, got %q", event.AuthorizationResult)
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

// TestRecordSecretAuditRuntimeSuccessIsAllowNotPerson proves the precedence and
// attribution fix: a local runtime operation that succeeded (no verified human,
// no decision, outcome=success) must attribute the runtime actor (trusted-local)
// rather than the cli/local-operator person default, and record
// AuthorizationResult=allow to match the actual success outcome.
func TestRecordSecretAuditRuntimeSuccessIsAllowNotPerson(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
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
	if event.Actor.Identity != "trusted-local" {
		t.Fatalf("succeeded runtime op must attribute runtime actor trusted-local, got %q", event.Actor.Identity)
	}
	if event.Actor.Subject == "local-operator" || event.Actor.Identity == "local-operator" {
		t.Fatalf("runtime op must not attribute the cli/local-operator person default, got %+v", event.Actor)
	}
	if event.AuthorizationResult != "allow" {
		t.Fatalf("succeeded runtime op must record AuthorizationResult=allow, got %q", event.AuthorizationResult)
	}
}

// TestRecordSecretAuditRuntimeFailureIsDenyWithRuntimeActor proves that a
// failed local runtime operation (no verified human, no decision,
// outcome=failure) attributes the runtime actor and records AuthorizationResult
// =deny, matching the actual failure outcome.
func TestRecordSecretAuditRuntimeFailureIsDenyWithRuntimeActor(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	resolved := secretAuditResolvedApplication("local", "demo", "dev")
	recordSecretAudit(ctx, resolved, "secret.delete", "API_TOKEN", classifySecretOutcome(errors.New("boom")))

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(events))
	}
	event := events[0]
	if event.Actor.Identity != "trusted-local" {
		t.Fatalf("failed runtime op must attribute runtime actor trusted-local, got %q", event.Actor.Identity)
	}
	if event.AuthorizationResult != "deny" {
		t.Fatalf("failed runtime op must record AuthorizationResult=deny, got %q", event.AuthorizationResult)
	}
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

// TestAuthorizeHumanRevealSuccessRecordsRevealAudit proves the success path of
// the reveal audit: a human reveal that succeeds records a reveal audit event
// with operation "secret.reveal", the success outcome, and AuthorizationResult
// =allow, against a temp state root.
func TestAuthorizeHumanRevealSuccessRecordsRevealAudit(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	principal := &identity.Principal{Subject: "operator", Issuer: "issuer", IssuerVerified: true}
	ctx := operatorauth.WithVerifiedPrincipal(context.Background(), principal)

	// The success helper records against the temp state root.
	recordRevealForTarget(ctx, "local", resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: "local"},
		Manifest: application.Manifest{Name: "demo", Environment: "dev"},
	}, "success", "")

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one reveal audit event, got %d", len(events))
	}
	event := events[0]
	if event.Operation != "secret.reveal" || event.Outcome != "success" {
		t.Fatalf("success reveal must record operation secret.reveal / outcome success, got %#v", event)
	}
	if event.AuthorizationResult != "allow" {
		t.Fatalf("success reveal must record AuthorizationResult=allow, got %q", event.AuthorizationResult)
	}
}

// TestAuthorizeHumanRevealDeniedRecordsRevealAudit proves that when the human
// operator check fails (no verified operator), the denied reveal is audited
// with operation "secret.reveal" and the denied outcome, so the audit captures
// denied reveals, not only successful ones.
func TestAuthorizeHumanRevealDeniedRecordsRevealAudit(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()

	err := authorizeHumanReveal(ctx, "local", "dev", "demo")
	if err == nil {
		t.Fatalf("secret.reveal must fail when no human operator is present")
	}

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one reveal audit event for the denial, got %d", len(events))
	}
	event := events[0]
	if event.Operation != "secret.reveal" || event.Outcome != "denied" {
		t.Fatalf("denied reveal must record operation secret.reveal / outcome denied, got %#v", event)
	}
	if event.AuthorizationResult != "deny" {
		t.Fatalf("denied reveal must record AuthorizationResult=deny, got %q", event.AuthorizationResult)
	}
}

// TestAuthorizeHumanRevealFailedRecordsRevealAudit proves the failed-reveal
// helper records a reveal audit event with operation "secret.reveal", the
// failed outcome, a failure reason in the detail, and AuthorizationResult=deny,
// so the audit captures failed reveals (not only denied or successful ones).
func TestAuthorizeHumanRevealFailedRecordsRevealAudit(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()

	cause := errors.New("policy denied")
	recordRevealFailed(ctx, "local", "dev", "demo", cause)

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one reveal audit event for the failure, got %d", len(events))
	}
	event := events[0]
	if event.Operation != "secret.reveal" || event.Outcome != "failed" {
		t.Fatalf("failed reveal must record operation secret.reveal / outcome failed, got %#v", event)
	}
	if !strings.Contains(event.Detail, "policy denied") {
		t.Fatalf("failed reveal must record the failure reason in detail, got %q", event.Detail)
	}
	if event.AuthorizationResult != "deny" {
		t.Fatalf("failed reveal must record AuthorizationResult=deny, got %q", event.AuthorizationResult)
	}
}
