package application

import (
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"sort"
)

// Sampling noise in RAM/container counters cannot make an unchanged mutation
// plan stale. Decisions still bind to actual consumers, ownership, binding,
// dependency graph and all displayed consequences/locations/access.
func impactDigestInput(p ProviderImpactPlan, registry capability.Registry, s ResolutionSnapshot, union ProviderPlan) ([]byte, error) {
	type structure struct {
		Target         string
		State          ProviderState
		Core           CoreRequirement
		Dependencies   []string
		Ancillary      bool
		OwnerReference string
	}
	facts := map[string]structure{}
	for id, f := range s.Facts {
		facts[id] = structure{f.Target, f.State, f.Core, f.Dependencies, f.Ancillary, f.OwnerReference}
	}
	type consumer struct {
		ApplicationID string
		Requirements  []RequirementResolution
	}
	consumers := map[string]consumer{}
	for key, fp := range union.Consumers {
		consumers[key] = consumer{fp.ApplicationID, fp.Requirements}
	}
	ancillary := append([]capability.ProviderInstance(nil), s.Ancillary...)
	sort.Slice(ancillary, func(i, j int) bool { return ancillary[i].ID < ancillary[j].ID })
	return json.Marshal(struct {
		Plan      ProviderImpactPlan
		Registry  capability.Registry
		Facts     map[string]structure
		Consumers map[string]consumer
		Required  []string
		Released  []capability.ProviderBinding
		Activity  map[string]bool
		Ancillary []capability.ProviderInstance
	}{p, registry, facts, consumers, union.Required, union.ReleasedBindings, union.ConsumerActivity, ancillary})
}
