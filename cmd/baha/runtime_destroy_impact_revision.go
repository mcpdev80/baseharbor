package main

import "github.com/mcpdev80/baseharbor/internal/deployment"

func runtimeDestroyImpactRevision(p runtimeDestroyImpact, owners map[string]string, targets []deployment.ResolvedTarget) string {
	// Child scope IDs bind material contents, owners and semantic consequences.
	// Target consequences already enter each canonical child PlanID. Their
	// displayed certificate countdowns must not enter the outer digest again.
	revisionPlan := p
	revisionPlan.Applications = make([]applicationDestructionPlan, len(p.Applications))
	for i, child := range p.Applications {
		scope := child.Scope
		scope.Targets = nil
		revisionPlan.Applications[i] = applicationDestructionPlan{Scope: scope}
	}
	return runtimeMutationRevision(struct {
		Plan    runtimeDestroyImpact
		Owners  map[string]string
		Targets []deployment.ResolvedTarget
	}{revisionPlan, owners, targets})
}
