package main

import (
	"context"
	"io"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/application"
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
		return err
	}
	if err := revalidateApplicationProviderIntent(ctx, resolved); err != nil {
		return err
	}
	current, err := buildResolvedApplicationPlan(ctx, resolved)
	if err != nil {
		return err
	}
	if current.ProviderPlan.Impact == nil || current.ProviderPlan.Impact.PlanID != plan.ProviderPlan.Impact.PlanID {
		return providerImpactConsentError(*plan.ProviderPlan.Impact, &application.ProviderChangeError{Code: "stale-impact-consent"})
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
