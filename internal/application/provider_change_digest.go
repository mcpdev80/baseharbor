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
	}{stableValidityImpact(p), registry, facts, consumers, union.Required, union.ReleasedBindings, union.ConsumerActivity, ancillary})
}

// Countdown samples change with observation time, while authority and absolute
// validity do not. Expiry still changes the function outcome and consent digest.
// Copy projections so calculating the digest never edits the displayed result.
func stableValidityImpact(p ProviderImpactPlan) ProviderImpactPlan {
	stable := p
	stable.Impacts = append([]ProviderImpact(nil), p.Impacts...)
	for i, impact := range p.Impacts {
		stable.Impacts[i].Functions = append([]FunctionConsequence(nil), impact.Functions...)
		for j := range stable.Impacts[i].Functions {
			stable.Impacts[i].Functions[j].RemainingValiditySeconds = nil
		}
		stable.Impacts[i].Materials = append([]MaterialConsequence(nil), impact.Materials...)
		for j := range stable.Impacts[i].Materials {
			stable.Impacts[i].Materials[j].RemainingValiditySeconds = nil
		}
	}
	return stable
}
