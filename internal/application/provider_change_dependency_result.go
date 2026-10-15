package application

import (
	"reflect"
	"sort"
)

// ProviderDependencyEvidence is a projection of the existing Target graph.
// Plans contain the observed prerequisite identities; execution supplies a
// fresh observation of those same identities and edges. It is not another owner.
type ProviderDependencyEvidence struct {
	Realization  ProviderRealization `json:"realization"`
	Dependencies []string            `json:"dependencies"`
}

func describeContinuationDependencies(s ResolutionSnapshot, observation ProviderImpactObservation, request ProviderChangeRequest) []ProviderDependencyEvidence {
	roots := []string{}
	if request.ReplacementID != "" {
		roots = append(roots, request.ReplacementID)
	}
	for _, action := range observation.ConsumerActions {
		if containsString(request.ConsumerActions, action.ID) {
			roots = append(roots, action.Dependencies...)
		}
	}
	instances, _ := indexResolutionInstances(s)
	visited := map[string]bool{}
	result := []ProviderDependencyEvidence{}
	var visit func(string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		instance, exists := instances[id]
		if !exists {
			return
		} // Existing plan validation exposes unknown nodes.
		realization, _ := observedRealization(instance, s, observation)
		deps := appendDependencies(s.Facts[id].Dependencies)
		if deps != nil {
			sort.Strings(deps)
		}
		result = append(result, ProviderDependencyEvidence{Realization: realization, Dependencies: deps})
		for _, dependency := range deps {
			visit(dependency)
		}
	}
	for _, root := range roots {
		visit(root)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Realization.InstanceID < result[j].Realization.InstanceID })
	return result
}

func continuationRequiresDependencyEvidence(impact ProviderImpact) bool {
	if impact.After != nil && impact.After.InstanceID != impact.InstanceID {
		return true
	}
	for _, function := range impact.Functions {
		if function.Transition != nil && len(function.Transition.Dependencies) != 0 {
			return true
		}
	}
	return false
}

func continuationDependencyOutcomesMatch(actual, expected []ProviderDependencyEvidence, aggregate []ProviderImpact) bool {
	if len(actual) != len(expected) {
		return false
	}
	byID := map[string]ProviderDependencyEvidence{}
	for _, node := range actual {
		id := node.Realization.InstanceID
		if _, duplicate := byID[id]; duplicate {
			return false
		}
		byID[id] = node
	}
	for _, wanted := range expected {
		node, exists := byID[wanted.Realization.InstanceID]
		got, before := node.Realization, wanted.Realization
		if !exists || !got.Supported || !got.Verified || got.Evidence == "" || got.State != ProviderRunning || got.InstanceID != before.InstanceID || got.Kind != before.Kind || got.Target != before.Target || got.Owner != before.Owner || got.Ownership != before.Ownership || got.Scope != before.Scope || got.Ancillary != before.Ancillary || !recoveryPreserved(before.Locations, got.Locations) || !sameAccess(before.Access, got.Access) || node.Dependencies == nil || wanted.Dependencies == nil {
			return false
		}
		deps := append([]string{}, node.Dependencies...)
		sort.Strings(deps)
		if !reflect.DeepEqual(deps, wanted.Dependencies) {
			return false
		}
		for _, change := range aggregate {
			if change.InstanceID == got.InstanceID && interruptsProvider(change.Operation) {
				return false
			}
		}
	}
	return true
}

// ObserveProviderContinuationDependencies collects fresh result evidence from
// the same authoritative registry/ancillary owners. New edges or changed owners
// remain visible and cannot match the approved graph without a fresh replan.
func ObserveProviderContinuationDependencies(impact ProviderImpact, snapshot ResolutionSnapshot, observation ProviderImpactObservation) ([]ProviderDependencyEvidence, error) {
	if err := snapshot.Registry.Validate(); err != nil {
		return nil, err
	}
	if err := validateImpactObservation(observation); err != nil {
		return nil, err
	}
	if err := validateObservationOwners(snapshot, observation); err != nil {
		return nil, err
	}
	instances, err := indexResolutionInstances(snapshot)
	if err != nil {
		return nil, err
	}
	result := []ProviderDependencyEvidence{}
	for _, expected := range impact.ContinuationDependencies {
		instance, exists := instances[expected.Realization.InstanceID]
		if !exists {
			return nil, &ProviderChangeError{Code: "continuation-dependency-missing"}
		}
		actual, _ := observedRealization(instance, snapshot, observation)
		deps := appendDependencies(snapshot.Facts[instance.ID].Dependencies)
		if deps != nil {
			sort.Strings(deps)
		}
		result = append(result, ProviderDependencyEvidence{Realization: actual, Dependencies: deps})
	}
	return result, nil
}
