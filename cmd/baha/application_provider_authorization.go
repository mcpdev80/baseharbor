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
	if err != nil || plan.ProviderPlan.Impact == nil {
		return err
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
	return nil
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
	// The existing read-only lifecycle resolver loads the canonical identity;
	// revalidation must not initialize or allocate a new source identity.
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
