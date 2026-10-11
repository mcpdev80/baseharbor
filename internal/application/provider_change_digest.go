package application

import (
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Sampling noise in RAM/container counters cannot make an unchanged mutation
// plan stale. Decisions still bind to actual consumers, ownership, binding,
// dependency graph and all displayed consequences/locations/access.
func impactDigestInput(p ProviderImpactPlan, registry capability.Registry, s ResolutionSnapshot, union ProviderPlan) ([]byte, error) {
	type structure struct {
		Target       string
		State        ProviderState
		Core         CoreRequirement
		Dependencies []string
	}
	facts := map[string]structure{}
	for id, f := range s.Facts {
		facts[id] = structure{f.Target, f.State, f.Core, f.Dependencies}
	}
	type consumer struct {
		ApplicationID string
		Requirements  []RequirementResolution
	}
	consumers := map[string]consumer{}
	for key, fp := range union.Consumers {
		consumers[key] = consumer{fp.ApplicationID, fp.Requirements}
	}
	return json.Marshal(struct {
		Plan      ProviderImpactPlan
		Registry  capability.Registry
		Facts     map[string]structure
		Consumers map[string]consumer
		Required  []string
		Released  []capability.ProviderBinding
		Activity  map[string]bool
	}{p, registry, facts, consumers, union.Required, union.ReleasedBindings, union.ConsumerActivity})
}
