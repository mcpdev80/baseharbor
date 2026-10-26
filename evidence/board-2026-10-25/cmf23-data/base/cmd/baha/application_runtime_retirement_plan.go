package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
)

// Reuse the existing union and consequence model, including scoped detachment.
func buildApplicationRuntimeRetirementPlan(ctx context.Context, original resolvedApplication) (resolvedApplication, application.ProviderPlan, error) {
	withdrawn := original
	withdrawn.Manifest = applicationRuntimeRetirementIntent(original.Manifest)
	snapshot, preferences, err := applicationLifecycleSnapshot(withdrawn, observeApplicationProviderFacts(ctx, withdrawn))
	if err != nil {
		return withdrawn, application.ProviderPlan{}, err
	}
	plan, err := resolveApplicationProviderPlanFromSnapshot(withdrawn, snapshot, preferences)
	if err != nil {
		return withdrawn, plan, err
	}
	requests := applicationProviderChangeRequests(plan)
	seen := map[string]bool{}
	for _, request := range requests {
		seen[request.InstanceID] = true
	}
	for _, binding := range plan.ReleasedBindings {
		if seen[binding.ProviderInstanceID] {
			continue
		}
		requests = append(requests, application.ProviderChangeRequest{InstanceID: binding.ProviderInstanceID, Operation: "unbind", Selection: "explicit", Consumers: []string{"application/" + original.Manifest.ApplicationID + "/" + original.Manifest.Environment}, Materials: []application.MaterialDisposition{}})
		seen[binding.ProviderInstanceID] = true
	}
	impact, err := resolveApplicationProviderImpact(ctx, withdrawn, snapshot, plan, requests)
	if err != nil {
		return withdrawn, plan, err
	}
	plan.Impact = &impact
	return withdrawn, plan, nil
}
