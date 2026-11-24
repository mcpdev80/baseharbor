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
	}, "success", "", "")

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
// so the audit captures failed reveals (not only denied or successful ones) and
// never records a spurious allow for a failed release (0428-277).
//
// Fail-closed proof: an ALLOWED operatorauth.AuthorizationDecision is installed
// in the context (WithAuthorizationDecision, the same upstream state a
// successful authorizeHumanReveal leaves behind). Under the original bug the
// recorder reused that stored "allow" decision and paired it with
// outcome=failed; this test fails in that case. It must go RED if recordRevealFailed
// reverts to recordRevealForTarget while the upstream decision stays allowed.
func TestAuthorizeHumanRevealFailedRecordsRevealAudit(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := operatorauth.WithAuthorizationDecision(context.Background(), operatorauth.AuthorizationDecision{
		ContractVersion: "2026-08-01",
		Allowed:         true,
		Operation:       "secret.reveal",
		Safety:          machine.SafetyDestructive,
		Actor:           operatorauth.MachineActorRef{Mode: "local-runtime", Subject: "trusted-local"},
	})

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
		t.Fatalf("failed reveal must record AuthorizationResult=deny (fail-closed even under an upstream allow), got %q", event.AuthorizationResult)
	}
	if strings.EqualFold(event.AuthorizationResult, "allow") {
		t.Fatalf("failed reveal must never record a spurious AuthorizationResult=allow, got %q", event.AuthorizationResult)
	}
	// RC2: the authenticated actor from the seeded upstream decision is still
	// attributed on a failed reveal — a failed release must not fall back to
	// cli/local-operator. This assertion is what goes RED if the fix is
	// reverted to a builder that drops the decision's actor.
	if event.Actor.Subject != "trusted-local" {
		t.Fatalf("failed reveal must preserve authenticated actor Subject, got %q", event.Actor.Subject)
	}
}

// TestAuthorizeHumanRevealSeamRecordsAuthorizationNotSuccess proves the
// premature-success fix at the seam: authorizeHumanReveal records the
// authorization decision (outcome "authorized") at the seam, NOT the
// material-release outcome ("success"). The deny path (already covered by
// TestAuthorizeHumanRevealDeniedRecordsRevealAudit) confirms the seam still
// audits at the operator check. The success path is now
// recordRevealAuthorized, never recordRevealSuccess, so a later read/parse
// failure can never see a spurious "success" audit.
func TestAuthorizeHumanRevealSeamRecordsAuthorizationNotSuccess(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	// The seam's post-authorization call is recordRevealAuthorized, which
	// records the "authorized" outcome — never "success".
	ctx := context.Background()
	recordRevealAuthorized(ctx, "local", "dev", "demo")

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(events))
	}
	event := events[0]
	if event.Operation != "secret.reveal" {
		t.Fatalf("expected secret.reveal operation, got %#v", event)
	}
	if event.Outcome == "success" {
		t.Fatalf("seam must never record the success (release) outcome, got %q", event.Outcome)
	}
	if event.Outcome != "authorized" {
		t.Fatalf("seam must record the authorization decision outcome authorized, got %q", event.Outcome)
	}
}

// TestAuthorizeHumanRevealPostAuthReadFailureRecordsNoSuccess proves that when
// the post-authorization read/parse fails, the consumer records a failure and
// the audit contains NO "success" event — the failed reveal never released
// material. The seam records "authorized"; the consumer's failed read records
// "failed"; neither is a "success".
func TestAuthorizeHumanRevealPostAuthReadFailureRecordsNoSuccess(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	// The seam records the authorization decision (not success).
	recordRevealAuthorized(ctx, "local", "dev", "demo")
	// The consumer's read/parse fails -> record the failure.
	recordRevealFailed(ctx, "local", "dev", "demo", errors.New("invalid application environment contract"))

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Operation == "secret.reveal" && e.Outcome == "success" {
			t.Fatalf("post-authorization read failure must not record a success reveal, got %#v", e)
		}
	}
	foundAuthorized, foundFailure := false, false
	for _, e := range events {
		if e.Operation != "secret.reveal" {
			continue
		}
		switch e.Outcome {
		case "authorized":
			foundAuthorized = true
		case "failed":
			foundFailure = true
		}
	}
	if !foundAuthorized || !foundFailure {
		t.Fatalf("expected both an authorized (seam) and a failed (read failure) reveal, got events: %#v", events)
	}
}

// TestAuthorizeHumanRevealSuccessfulReleaseRecordsSuccess proves that a
// successful reveal release records a "success" audit — this is recorded by
// the consumer at the release point (recordRevealSuccess), not by the seam.
func TestAuthorizeHumanRevealSuccessfulReleaseRecordsSuccess(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	// The seam records "authorized" (authorization decision), not success.
	recordRevealAuthorized(ctx, "local", "dev", "demo")
	// The consumer's read/parse succeeds -> records the release success.
	recordRevealSuccess(ctx, "local", "dev", "demo")

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAuditOperation(events, "secret.reveal") {
		t.Fatal("expected a secret.reveal audit event")
	}
	foundAuthorized, foundSuccess := false, false
	for _, e := range events {
		if e.Operation != "secret.reveal" {
			continue
		}
		if e.Outcome == "authorized" {
			foundAuthorized = true
		}
		if e.Outcome == "success" {
			foundSuccess = true
		}
	}
	if !foundAuthorized {
		t.Fatalf("expected the seam to record an authorized decision, got events: %#v", events)
	}
	if !foundSuccess {
		t.Fatalf("successful reveal release must record a success outcome, got events: %#v", events)
	}
}

// TestRecordEnvDestroyAuditRuntimeSuccessIsAllowNotPerson proves the runtime
// actor attribution fix for env.destroy: a succeeded local runtime env-destroy
// (no verified human, no decision, outcome=success) must attribute the runtime
// actor (trusted-local, not the cli/local-operator person default) and record
// AuthorizationResult=allow to match the success outcome.
func TestRecordEnvDestroyAuditRuntimeSuccessIsAllowNotPerson(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	recordEnvDestroyAudit(ctx, "local", "demo", "succeeded", "success")

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(events))
	}
	event := events[0]
	if event.Actor.Identity != "trusted-local" {
		t.Fatalf("succeeded runtime env-destroy must attribute runtime actor trusted-local, got %q", event.Actor.Identity)
	}
	if event.Actor.Subject == "local-operator" || event.Actor.Identity == "local-operator" {
		t.Fatalf("runtime env-destroy must not attribute the cli/local-operator person default, got %+v", event.Actor)
	}
	if event.AuthorizationResult != "allow" {
		t.Fatalf("succeeded runtime env-destroy must record AuthorizationResult=allow, got %q", event.AuthorizationResult)
	}
}

// TestRecordEnvDestroyAuditRuntimeFailureIsDenyWithRuntimeActor proves that a
// failed local runtime env-destroy (no verified human, no decision,
// outcome=failure) attributes the runtime actor and records
// AuthorizationResult=deny, matching the failure outcome.
func TestRecordEnvDestroyAuditRuntimeFailureIsDenyWithRuntimeActor(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	recordEnvDestroyAudit(ctx, "local", "demo", "failed to remove", "failed")

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(events))
	}
	event := events[0]
	if event.Actor.Identity != "trusted-local" {
		t.Fatalf("failed runtime env-destroy must attribute runtime actor trusted-local, got %q", event.Actor.Identity)
	}
	if event.AuthorizationResult != "deny" {
		t.Fatalf("failed runtime env-destroy must record AuthorizationResult=deny, got %q", event.AuthorizationResult)
	}
}

// TestRecordEnvDestroyAuditDeniedIsDenyWithRuntimeActor proves that a denied
// runtime env-destroy (no verified human, no decision, outcome=denied)
// attributes the runtime actor and records AuthorizationResult=deny.
func TestRecordEnvDestroyAuditDeniedIsDenyWithRuntimeActor(t *testing.T) {
	root := t.TempDir()
	targetStateRoot = func(string) (string, error) { return root, nil }
	defer func() { targetStateRoot = deployment.TargetStateRoot }()

	ctx := context.Background()
	recordEnvDestroyAudit(ctx, "local", "demo", "denied by policy", "denied")

	events, err := evidence.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(events))
	}
	event := events[0]
	if event.Actor.Identity != "trusted-local" {
		t.Fatalf("denied runtime env-destroy must attribute runtime actor trusted-local, got %q", event.Actor.Identity)
	}
	if event.AuthorizationResult != "deny" {
		t.Fatalf("denied runtime env-destroy must record AuthorizationResult=deny, got %q", event.AuthorizationResult)
	}
}
