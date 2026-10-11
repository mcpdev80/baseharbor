package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	"sort"
)

// Walks the already-resolved dependency graph for consequence attribution only.
// It performs no provider selection and changes no union or binding.
func dependencyPath(s ResolutionSnapshot, root, target string, visiting map[string]bool) []string {
	if root == target {
		return []string{root}
	}
	if visiting[root] {
		return nil
	}
	visiting[root] = true
	defer delete(visiting, root)
	deps := append([]string(nil), s.Facts[root].Dependencies...)
	sort.Strings(deps)
	for _, dep := range deps {
		if path := dependencyPath(s, dep, target, visiting); path != nil {
			return append([]string{root}, path...)
		}
	}
	return nil
}

func observedFunctionUses(s ResolutionSnapshot, union ProviderPlan, actual []ProviderFunctionUse) []ProviderFunctionUse {
	result := append([]ProviderFunctionUse(nil), actual...)
	for _, b := range s.Registry.Bindings {
		consumer := "application/" + b.ApplicationID + "/" + b.Environment
		for key, fp := range union.Consumers {
			if fp.ApplicationID == b.ApplicationID {
				consumer = key
				break
			}
		}
		found := false
		for _, use := range result {
			if use.InstanceID == b.ProviderInstanceID && use.Consumer == consumer {
				found = true
				break
			}
		}
		if !found {
			required := false
			if fp, active := union.Consumers[consumer]; active {
				for _, req := range fp.Requirements {
					if req.InstanceID == b.ProviderInstanceID && req.Requirement.Kind == b.Resource.Kind && req.Requirement.Name == b.Resource.Name {
						required = true
					}
				}
			}
			result = append(result, ProviderFunctionUse{Consumer: consumer, InstanceID: b.ProviderInstanceID, Function: string(b.Resource.Kind), Required: required, Evidence: "authoritative provider binding"})
		}
	}
	// Newly resolved and independently enabled consumers may have no persisted
	// binding yet. Attribute their real requested requirements to the same union.
	for key, fp := range union.Consumers {
		for _, req := range fp.Requirements {
			if req.InstanceID == "" {
				continue
			}
			found := false
			for _, use := range result {
				if use.Consumer == key && use.InstanceID == req.InstanceID {
					found = true
					break
				}
			}
			if !found {
				result = append(result, ProviderFunctionUse{Consumer: key, InstanceID: req.InstanceID, Function: string(req.Requirement.Kind), Required: true, Evidence: "resolved consumer requirement"})
			}
		}
	}
	return result
}

func verifiedFunctionContinuation(o ProviderImpactObservation, use ProviderFunctionUse, to string) bool {
	for _, c := range o.Continuations {
		if c.Consumer == use.Consumer && c.Function == use.Function && c.From == use.InstanceID && c.To == to && c.Supported && c.Verified && c.Evidence != "" {
			return true
		}
	}
	return false
}

func knownProviderFunction(p capability.Provider, name string) bool {
	for _, kind := range p.Capabilities {
		if string(kind) == name {
			return true
		}
	}
	d, err := capability.ReferenceIntegration(p.Kind)
	if err != nil {
		return false
	}
	c, ok := d.ChangeContract()
	return ok && containsString(c.Functions, name)
}
