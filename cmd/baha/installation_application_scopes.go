package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type installationApplicationScopeKey struct{}
type runtimeDestroyAcknowledgementKey struct{}

type installationApplicationScope struct {
	Scope    application.ProviderImpactScope
	Consent  application.ProviderChangeConsent
	Children map[string]string
}

// Delegation is limited to the exact child plans displayed inside an already
// acknowledged installation plan. Generic approval cannot mint a child grant.
func withInstallationApplicationScopes(ctx context.Context, plan runtimeDestroyImpact) (context.Context, error) {
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return ctx, err
	}
	if consent == nil {
		consent, _ = ctx.Value(runtimeDestroyAcknowledgementKey{}).(*application.ProviderChangeConsent)
	}
	if !plan.All || plan.Scope.Owner.Owner != "installation.destroy" || !plan.Scope.Destructive || consent == nil {
		return ctx, classifyProviderChangeConsent(&application.ProviderChangeError{Code: "impact-confirmation-required"})
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, *consent); err != nil {
		return ctx, runtimeDestroyImpactError(plan, err)
	}
	proof := installationApplicationScope{Scope: plan.Scope, Consent: *consent, Children: map[string]string{}}
	for _, child := range plan.Applications {
		if !child.Scope.Destructive || child.Scope.Owner.Owner != "application-deployment" || proof.Children[child.Scope.Owner.Subject] != "" {
			return ctx, runtimeMutationOwnerError()
		}
		proof.Children[child.Scope.Owner.Subject] = child.Scope.PlanID
	}
	return context.WithValue(ctx, installationApplicationScopeKey{}, proof), nil
}

func delegatedInstallationApplicationConsent(ctx context.Context, current applicationDestructionPlan, reset bool) (*application.ProviderChangeConsent, bool, error) {
	proof, present := ctx.Value(installationApplicationScopeKey{}).(installationApplicationScope)
	if !present {
		return nil, false, nil
	}
	if err := application.ValidateProviderImpactScopeConsent(proof.Scope, proof.Consent); err != nil {
		return nil, true, err
	}
	if !reset || !current.Scope.Destructive || proof.Children[current.Scope.Owner.Subject] != current.Scope.PlanID {
		return nil, true, &application.ProviderChangeError{Code: "stale-impact-consent"}
	}
	return &application.ProviderChangeConsent{PlanID: current.Scope.PlanID, First: proof.Consent.First, Second: proof.Consent.Second, DeleteData: proof.Consent.DeleteData}, true, nil
}
