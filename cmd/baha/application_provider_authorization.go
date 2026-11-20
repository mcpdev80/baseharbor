package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// All apply/up transports reach this existing mutation authority before
// materializing providers. The resolver and consequence domain own the policy;
// this adapter only collects consent and rejects stale observed effects.
func authorizeApplicationProviderChanges(ctx context.Context, resolved resolvedApplication, in io.Reader, out io.Writer) error {
	if err := revalidateApplicationProviderIntent(ctx, resolved); err != nil {
		return err
	}
	plan, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil {
		return err
	}
	if plan.ProviderPlan.Impact == nil {
		return validateApplicationProviderExecution(resolved, nil)
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return err
	}
	if err := acknowledgeProviderImpact(ctx, in, out, *plan.ProviderPlan.Impact, consent); err != nil {
		// Return the exact source identity with the read-only plan challenge so
		// interactive clients can approve both in one review before any apply.
		var challenge *machine.Error
		if errors.As(err, &challenge) && challenge.Code == machine.ErrorApprovalRequired && resolved.FromRepository && resolved.SourceAvailable {
			if _, fingerprintErr := repositoryDesiredStateFingerprint(ctx, resolved); fingerprintErr != nil {
				return fingerprintErr
			}
			submission, _ := candidateSubmissionForResolvedApp(ctx, resolved)
			if digest := delivery.CandidateConsentDigest(submission); digest != "" {
				challenge.Next += " Candidate: --confirm-candidate " + digest
			}
		}
		return err
	}
	if err := revalidateApplicationProviderIntent(ctx, resolved); err != nil {
		return err
	}
	current, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil {
		return err
	}
	if current.ProviderPlan.Impact == nil {
		if out != nil {
			fmt.Fprintln(out, "authorization mismatch: provider-plan-id-after-revalidation")
		}
		return providerImpactConsentError(*plan.ProviderPlan.Impact, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	// Hunt 916 (A-F1/B2/C3): the plan id is the digest of the CONSENT
	// AUTHORITY (provider_change_plan.go, provider_consent_authority.go), so
	// the raw post-consent comparison recognizes an unchanged plan even when
	// a transient observation artifact (unverifiable classification flip of
	// unchanged material, re-derived revision, clock sample) churned the old
	// raw digest. As the second gate, the frozen authority surfaces decide
	// before staleness fires: only display-path differences that never reach
	// the digest stabilize; every genuine authority change keeps the
	// fail-closed stale-impact-consent path.
	if current.ProviderPlan.Impact.PlanID != plan.ProviderPlan.Impact.PlanID {
		if err := application.ValidateProviderImpactRevalidation(*current.ProviderPlan.Impact, *plan.ProviderPlan.Impact); err != nil {
			if out != nil {
				fmt.Fprintln(out, "authorization mismatch: provider-plan-id-after-revalidation")
			}
			return providerImpactConsentError(*plan.ProviderPlan.Impact, err)
		}
	}
	return validateApplicationProviderExecution(resolved, current.ProviderPlan.Impact)
}

// The source transfer proves exact bytes, not application ownership. The
// previously authorized resolution is the owner: never accept a source that
// claims a different application, environment or identity.
func bindAuthoritativeProviderRevalidationIdentity(resolved resolvedApplication, selection application.RepositoryEnvironmentSelection) (application.RepositoryEnvironmentSelection, error) {
	id := resolved.Manifest.ApplicationID
	if err := application.ValidateApplicationID(id); err != nil {
		return selection, classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	if resolved.DeploymentIdentity.ApplicationID != id ||
		selection.Manifest.Name != resolved.Manifest.Name ||
		selection.Manifest.Environment != resolved.Manifest.Environment ||
		(selection.Manifest.ApplicationID != "" && selection.Manifest.ApplicationID != id) {
		return selection, classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	selection.Manifest.ApplicationID = id
	return selection, nil
}

func revalidateApplicationProviderIntent(ctx context.Context, resolved resolvedApplication) error {
	target, err := effectiveTarget(withTargetOverride(ctx, resolved.Target.Name))
	if err != nil {
		return err
	}
	if target != resolved.Target {
		return classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	if !resolved.FromRepository || !resolved.SourceAvailable {
		return nil
	}
	selection, err := application.ResolveRepositoryEnvironment(resolved.RepositoryRoot, resolved.Manifest.Environment)
	if err != nil {
		return err
	}
	// Uploaded workstation source intentionally omits the Core-owned ID.
	// Revalidation must reuse the identity already selected from the
	// authoritative deployment, not look for a second owner in the uploaded
	// source root or initialize a different application.
	selection, err = bindAuthoritativeProviderRevalidationIdentity(resolved, selection)
	if err != nil {
		return err
	}
	selection, err = resolveRepositoryLifecycleIntent(ctx, resolved.Target, selection, "plan")
	if err != nil {
		return err
	}
	current := resolved
	current.Manifest = selection.Manifest
	selection.Manifest, err = resolveAppliedBackendSelection(current)
	if err != nil {
		return err
	}
	if selection.ManifestPath != resolved.ManifestPath || !reflect.DeepEqual(selection.Manifest, resolved.Manifest) {
		return classifyProviderChangeConsent(&application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	return nil
}
