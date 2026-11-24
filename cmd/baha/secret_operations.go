package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// targetStateRoot resolves a Target name to its runtime state root. It is a
// package variable so tests can point secret audit writes at a temp root
// without standing up a real Target deployment.
var targetStateRoot = deployment.TargetStateRoot

type secretMutationResult struct {
	Application string `json:"application"`
	Environment string `json:"environment"`
	Key         string `json:"key"`
	Updated     bool   `json:"updated,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
	Preview     bool   `json:"preview,omitempty"`
}

// recordSecretAudit is best-effort: a failure to write the audit must never
// fail the secret operation itself, but is surfaced to stderr. It records who
// (human vs runtime actor), what (exact key, never the value), where (target /
// application / environment), and the result (success / denial / failure) for
// the secret set/delete/list seam. No secret value or plaintext credential is
// ever written to the audit log.
func recordSecretAudit(ctx context.Context, resolved resolvedApplication, operation, resource, outcome string) {
	root, rootErr := targetStateRoot(resolved.Target.Name)
	if rootErr != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not resolve target state root for secret audit:", rootErr)
		return
	}
	event := evidence.NewAuditEvent(ctx, resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, operation, outcome)
	event.Resource = resource
	event.ApplicationID = resolved.Manifest.ApplicationID
	event.DeploymentID = resolved.DeploymentIdentity.DeploymentID
	event.Capability = "app.secret"
	// A present AuthorizationDecision takes precedence over any principal: a
	// verified principal can coexist with a policy denial (authorizeMCPOperation
	// stores denied machine decisions in this context), so a present decision
	// must never fall back to principal-allow. Only when no decision is present
	// does the verified principal stand in.
	if decision, ok := operatorauth.AuthorizationDecisionFromContext(ctx); ok {
		event.Actor.Subject = decision.Actor.Subject
		event.Actor.Assurance = decision.Actor.Assurance
		event.Actor.Methods = append([]string(nil), decision.Actor.Methods...)
		if decision.Allowed {
			event.AuthorizationResult = "allow"
		} else {
			event.AuthorizationResult = "deny"
		}
	} else if principal, ok := operatorauth.PrincipalFromContext(ctx); ok {
		event.Actor.Identity = principal.Subject
		event.Actor.Issuer = principal.Issuer
		event.Actor.Subject = principal.Subject
		event.Actor.Assurance = principal.Assurance
		event.Actor.Methods = append([]string(nil), principal.Methods...)
		event.AuthorizationResult = "allow"
	} else {
		// No verified operator on the enforcement seam. A succeeded local
		// workload operation attributes the runtime actor (not the
		// cli/local-operator person default — trusted-local is not a person),
		// and the AuthorizationResult matches the actual outcome: a success is
		// an allow, not a deny. A failure with no decision is the one case that
		// is correctly denied.
		event.Actor.Identity = "trusted-local"
		if strings.EqualFold(outcome, "success") {
			event.AuthorizationResult = "allow"
		} else {
			event.AuthorizationResult = "deny"
		}
	}
	if err := evidence.Append(root, event); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not record secret audit:", err)
	}
}

func classifySecretOutcome(err error) string {
	if err == nil {
		return "success"
	}
	classified := machine.Classify(err)
	switch classified.Code {
	case machine.ErrorAuthenticationFailed, machine.ErrorPolicyDenied, machine.ErrorOwnershipAmbiguous, machine.ErrorCapabilityMissing, machine.ErrorApprovalRequired:
		return "denial"
	default:
		return "failure"
	}
}

func authorizeApplicationOperation(ctx context.Context, id string, resolved resolvedApplication) error {
	return authorizeMCPOperation(ctx, id, resolved.Target.Name, resolved.Manifest.Environment, resolved.Manifest.ApplicationID, resolved.ManifestPath)
}

// authorizeHumanReveal gates any path that would return a managed secret value
// to a human operator (CLI --reveal, app creds --reveal). It first requires a
// verified human identity, then applies the registered secret.reveal operation's
// normal target, environment, membership, and permission checks. Workload reads
// that do not reveal keep using their existing operation and are unaffected.
func authorizeHumanReveal(ctx context.Context, target, environment, appName string) error {
	if err := operatorauth.RequireHumanOperator(ctx); err != nil {
		// Reveal denied at the human-operator seam: audit the denial so the
		// audit captures denied reveals, not only successful ones.
		recordRevealDenied(ctx, target, environment, appName)
		return err
	}
	if err := authorizeMCPOperation(ctx, "secret.reveal", target, environment, appName, ""); err != nil {
		// Reveal failed after the human operator was verified (policy,
		// membership, permissions, runtime): audit the failure too.
		recordRevealFailed(ctx, target, environment, appName, err)
		return err
	}
	// Authorization decision recorded here is distinct from the reveal outcome:
	// this records that a human operator was authorized to reveal, not that
	// material was released. Consumers record the actual success/failure at the
	// release point (see recordRevealSuccess).
	recordRevealAuthorized(ctx, target, environment, appName)
	return nil
}

// recordRevealAuthorized records the authorization decision at the
// secret.reveal seam: a human operator was verified and authorized to reveal.
// It is intentionally named "authorized" (not "success") to distinguish the
// authorization decision from the actual material-release outcome.
func recordRevealAuthorized(ctx context.Context, target, environment, appName string) {
	recordRevealForTarget(ctx, target, resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: target},
		Manifest: application.Manifest{Environment: environment, Name: appName},
	}, "authorized", "", "")
}

// recordRevealDenied records a denied human reveal (failed human-operator
// check) against a synthesized resolved target.
func recordRevealDenied(ctx context.Context, target, environment, appName string) {
	recordRevealForTarget(ctx, target, resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: target},
		Manifest: application.Manifest{Environment: environment, Name: appName},
	}, "denied", "", "")
}

// recordRevealFailed records a failed human reveal (post-operator failure) with
// a failure reason in the event detail.
//
// Fail-closed audit precision (0428-277): a failed reveal must never pair
// outcome=failed with a spurious AuthorizationResult=allow. The upstream
// recordRevealAuthorized records that the human was authorized to reveal, but
// no material was released when the read/parse failed, so reusing that
// upstream "authorized/allow" decision alongside a failed release would
// produce an inconsistent audit event. recordRevealFailed passes "deny" as a
// narrow override to the shared builder (recordRevealForTarget), which still
// applies the authenticated actor attribution below.
func recordRevealFailed(ctx context.Context, target, environment, appName string, cause error) {
	reason := "reveal failed"
	if cause != nil {
		reason = cause.Error()
	}
	recordRevealForTarget(ctx, target, resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: target},
		Manifest: application.Manifest{Environment: environment, Name: appName},
	}, "failed", reason, "deny")
}

// recordRevealSuccess records a successful human reveal.
func recordRevealSuccess(ctx context.Context, target, environment, appName string) {
	recordRevealForTarget(ctx, target, resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: target},
		Manifest: application.Manifest{Environment: environment, Name: appName},
	}, "success", "", "")
}

// recordRevealForTarget resolves the target state root and records a reveal
// audit with the given outcome. detail is recorded on failed outcomes so the
// failure reason is captured; it is ignored for the denied/success outcomes.
// authorizationResultOverride, when non-empty, forces the recorded
// AuthorizationResult — recordRevealFailed passes "deny" here so a failed
// reveal never carries a spurious allow, while the actor attribution below
// still records the authenticated actor.
func recordRevealForTarget(ctx context.Context, target string, resolved resolvedApplication, outcome, detail, authorizationResultOverride string) {
	root, rootErr := targetStateRoot(resolved.Target.Name)
	if rootErr != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not resolve target state root for reveal audit:", rootErr)
		return
	}
	event := evidence.NewAuditEvent(ctx, resolved.Target.Name, resolved.Manifest.Name, resolved.Manifest.Environment, "secret.reveal", outcome)
	event.Capability = "app.secret"
	event.ApplicationID = resolved.Manifest.ApplicationID
	event.DeploymentID = resolved.DeploymentIdentity.DeploymentID
	if detail != "" {
		event.Detail = detail
	}
	// Authenticated actor attribution always runs first, so a failed reveal
	// keeps the verified actor instead of falling back to cli/local-operator.
	if decision, ok := operatorauth.AuthorizationDecisionFromContext(ctx); ok {
		event.Actor.Subject = decision.Actor.Subject
		event.Actor.Assurance = decision.Actor.Assurance
		event.Actor.Methods = append([]string(nil), decision.Actor.Methods...)
	} else if principal, ok := operatorauth.PrincipalFromContext(ctx); ok {
		event.Actor.Identity = principal.Subject
		event.Actor.Issuer = principal.Issuer
		event.Actor.Subject = principal.Subject
		event.Actor.Assurance = principal.Assurance
		event.Actor.Methods = append([]string(nil), principal.Methods...)
	} else {
		event.Actor.Identity = "trusted-local"
	}
	// Fail-closed override (0428-277): recordRevealFailed passes "deny" so a
	// failed reveal never carries a spurious allow. Absent the override the AR
	// is derived from the upstream authorization decision, falling back to the
	// principal (allowed) or a deny on a failed outcome.
	if authorizationResultOverride != "" {
		event.AuthorizationResult = authorizationResultOverride
	} else if decision, ok := operatorauth.AuthorizationDecisionFromContext(ctx); ok {
		if decision.Allowed {
			event.AuthorizationResult = "allow"
		} else {
			event.AuthorizationResult = "deny"
		}
	} else if _, ok := operatorauth.PrincipalFromContext(ctx); ok {
		event.AuthorizationResult = "allow"
	} else if strings.EqualFold(outcome, "success") {
		event.AuthorizationResult = "allow"
	} else {
		event.AuthorizationResult = "deny"
	}
	if err := evidence.Append(root, event); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not record reveal audit:", err)
	}
}
func listApplicationSecrets(ctx context.Context, resolved resolvedApplication) ([]applicationsecret.Metadata, error) {
	if err := authorizeApplicationOperation(ctx, "secret.list", resolved); err != nil {
		recordSecretAudit(ctx, resolved, "secret.list", "", classifySecretOutcome(err))
		return nil, err
	}
	service, err := resolvedApplicationSecretService(ctx, resolved)
	if err != nil {
		recordSecretAudit(ctx, resolved, "secret.list", "", classifySecretOutcome(err))
		return nil, err
	}
	items, err := service.List(ctx, resolved.Manifest.Name)
	if err != nil {
		recordSecretAudit(ctx, resolved, "secret.list", "", classifySecretOutcome(err))
		return nil, err
	}
	configured := make([]applicationsecret.Metadata, 0, len(items))
	for _, item := range items {
		if item.Present {
			configured = append(configured, item)
		}
	}
	recordSecretAudit(ctx, resolved, "secret.list", "", classifySecretOutcome(nil))
	return configured, nil
}

// Input is read only after shared authorization and secret-scope validation.
// The callback keeps protected bytes out of transport DTOs and audit metadata.
func setApplicationSecret(ctx context.Context, resolved resolvedApplication, key string, input func() ([]byte, error)) (secretMutationResult, error) {
	if err := authorizeApplicationOperation(ctx, "secret.set", resolved); err != nil {
		recordSecretAudit(ctx, resolved, "secret.set", key, classifySecretOutcome(err))
		return secretMutationResult{}, err
	}
	service, err := resolvedApplicationSecretService(ctx, resolved)
	if err != nil {
		recordSecretAudit(ctx, resolved, "secret.set", key, classifySecretOutcome(err))
		return secretMutationResult{}, err
	}
	value, err := input()
	if err != nil {
		recordSecretAudit(ctx, resolved, "secret.set", key, classifySecretOutcome(err))
		return secretMutationResult{}, err
	}
	defer zeroBytes(value)
	if err := service.Set(ctx, resolved.Manifest.Name, key, value); err != nil {
		recordSecretAudit(ctx, resolved, "secret.set", key, classifySecretOutcome(err))
		return secretMutationResult{}, err
	}
	recordSecretAudit(ctx, resolved, "secret.set", key, classifySecretOutcome(nil))
	return secretMutationResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Key: key, Updated: true}, nil
}
func deleteApplicationSecret(ctx context.Context, resolved resolvedApplication, key string, confirmed bool) (secretMutationResult, error) {
	if err := authorizeApplicationOperation(ctx, "secret.delete", resolved); err != nil {
		recordSecretAudit(ctx, resolved, "secret.delete", key, classifySecretOutcome(err))
		return secretMutationResult{}, err
	}
	service, err := resolvedApplicationSecretService(ctx, resolved)
	if err != nil {
		recordSecretAudit(ctx, resolved, "secret.delete", key, classifySecretOutcome(err))
		return secretMutationResult{}, err
	}
	items, err := service.List(ctx, resolved.Manifest.Name)
	if err != nil {
		recordSecretAudit(ctx, resolved, "secret.delete", key, classifySecretOutcome(err))
		return secretMutationResult{}, err
	}
	found := false
	for _, item := range items {
		if item.Name == key && item.Present {
			found = true
		}
	}
	if !found {
		recordSecretAudit(ctx, resolved, "secret.delete", key, classifySecretOutcome(openbao.ErrApplicationSecretNotFound))
		return secretMutationResult{}, openbao.ErrApplicationSecretNotFound
	}
	result := secretMutationResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Key: key, Preview: !confirmed}
	if confirmed {
		if err := service.Delete(ctx, resolved.Manifest.Name, key); err != nil {
			recordSecretAudit(ctx, resolved, "secret.delete", key, classifySecretOutcome(err))
			return result, err
		}
		result.Deleted = true
	}
	recordSecretAudit(ctx, resolved, "secret.delete", key, classifySecretOutcome(nil))
	return result, nil
}
