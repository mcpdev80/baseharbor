package application

import (
	"encoding/json"
	"fmt"
	"sort"
)

// RecordProviderChangeResult projects verified EXECUTION receipts, never the
// proposed After fields. Missing/failed steps remain individually pending.
// Actual unexpected dispositions are retained and reported as partial failure.
func RecordProviderChangeResult(plan ProviderImpactPlan, outcomes []ProviderExecutionOutcome) (ProviderChangeResult, error) {
	result := ProviderChangeResult{Version: ProviderImpactVersion, PlanID: plan.PlanID, Status: "partial", Outcomes: []ProviderExecutionOutcome{}}
	if plan.Version != ProviderImpactVersion || plan.PlanID == "" {
		return result, &ProviderChangeError{Code: "invalid-impact-plan"}
	}
	proposed := map[string]ProviderImpact{}
	for _, i := range plan.Impacts {
		proposed[i.InstanceID+"/"+i.Operation] = i
	}
	actual := map[string]ProviderExecutionOutcome{}
	for _, out := range outcomes {
		key := out.InstanceID + "/" + out.Operation
		imp, exists := proposed[key]
		if !exists {
			return result, &ProviderChangeError{Code: "unplanned-provider-outcome"}
		}
		if _, duplicate := actual[key]; duplicate {
			return result, fmt.Errorf("duplicate provider execution outcome")
		}
		if err := validateExecutionOutcome(out); err != nil {
			return result, err
		}
		if out.Status == "applied" {
			if !out.Verified || out.Evidence == "" {
				return result, &ProviderChangeError{Code: "execution-verification-required"}
			}
			// Partial/unexpected effects must stay visible instead of being called a
			// successful execution of the proposed plan.
			if continuationRequiresDependencyEvidence(imp) && len(imp.ContinuationDependencies) == 0 || !continuationDependencyOutcomesMatch(out.ContinuationDependencies, imp.ContinuationDependencies, plan.Impacts) {
				out.Status = "failed"
				out.ErrorCode = "continuation-dependency-unverified"
			} else if !outcomeMatchesImpact(out, imp, plan.Impacts) {
				out.Status = "failed"
				out.ErrorCode = "actual-disposition-differs"
			}
		}
		actual[key] = out
	}
	applied, failed := 0, 0
	for _, imp := range plan.Impacts {
		out, exists := actual[imp.InstanceID+"/"+imp.Operation]
		if !exists {
			out = ProviderExecutionOutcome{InstanceID: imp.InstanceID, Operation: imp.Operation, Status: "pending", Materials: []MaterialOutcome{}}
		}
		result.Outcomes = append(result.Outcomes, out)
		if out.Status == "applied" {
			applied++
		}
		if out.Status == "failed" {
			failed++
		}
	}
	sort.Slice(result.Outcomes, func(i, j int) bool { return result.Outcomes[i].InstanceID < result.Outcomes[j].InstanceID })
	if plan.Complete && applied == len(plan.Impacts) {
		result.Status = "complete"
	} else if failed > 0 && failed == len(plan.Impacts) {
		result.Status = "failed"
	}
	return result, nil
}

func validateExecutionOutcome(out ProviderExecutionOutcome) error {
	switch out.Status {
	case "applied", "failed", "pending":
	default:
		return fmt.Errorf("invalid provider execution status")
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return err
	}
	if err := validateImpactStrings(tree); err != nil {
		return err
	}
	for _, dependency := range out.ContinuationDependencies {
		realization := dependency.Realization
		if err := validateImpactObservation(ProviderImpactObservation{Providers: map[string]ProviderImpactFact{realization.InstanceID: {Realization: realization}}}); err != nil {
			return err
		}
	}
	if out.After != nil {
		r := out.After
		if r.InstanceID == "" || r.Target == "" || r.Kind == "" {
			return fmt.Errorf("actual realization requires identity, Target and kind")
		}
		if r.Verified && r.Evidence == "" {
			return fmt.Errorf("actual verified realization requires evidence")
		}
		for _, l := range r.Locations {
			if err := validateLocation(l); err != nil {
				return err
			}
		}
		if err := validateProviderAccess(r.Access); err != nil {
			return err
		}
	}
	for _, f := range out.Functions {
		if f.Transition != nil {
			if err := validateConsumerTransition(*f.Transition); err != nil {
				return err
			}
		}
		if f.Consumer == "" || f.InstanceID == "" || f.Function == "" {
			return fmt.Errorf("actual function outcome requires consumer, root and function")
		}
		switch f.Outcome {
		case "remains", "changed", "unavailable", "unknown":
		default:
			return fmt.Errorf("invalid actual function outcome")
		}
		if f.Verified && f.Evidence == "" {
			return fmt.Errorf("verified function outcome requires evidence")
		}
		if err := validateExpiry(f.ValidUntil); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, m := range out.Materials {
		if m.MaterialID == "" || seen[m.MaterialID] {
			return fmt.Errorf("missing/duplicate actual material")
		}
		seen[m.MaterialID] = true
		d := MaterialDisposition{MaterialID: m.MaterialID, Disposition: m.Disposition, Destination: m.Location, AccessAfter: m.Access, Verified: m.Verified, Evidence: m.Evidence}
		if err := validateDisposition(d); err != nil {
			return err
		}
		for _, l := range m.Recovery {
			if err := validateLocation(l); err != nil {
				return err
			}
		}
	}
	return nil
}

func outcomeMatchesImpact(out ProviderExecutionOutcome, imp ProviderImpact, aggregate []ProviderImpact) bool {
	if imp.After == nil {
		if out.After != nil {
			return false
		}
	} else {
		if out.After == nil || !out.After.Verified || out.After.Evidence == "" || out.After.InstanceID != imp.After.InstanceID || out.After.Kind != imp.After.Kind || out.After.Target != imp.After.Target || out.After.State != imp.After.State || out.After.Ownership != imp.After.Ownership || out.After.Scope != imp.After.Scope || out.After.Owner != imp.After.Owner || !recoveryPreserved(imp.After.Locations, out.After.Locations) || !sameAccess(imp.After.Access, out.After.Access) {
			return false
		}
	}
	if !effectOutcomesMatch(out.Effects, imp.Effects) {
		return false
	}
	if !functionOutcomesMatch(out.Functions, imp.Functions) {
		return false
	}
	actual := map[string]MaterialOutcome{}
	for _, m := range out.Materials {
		actual[m.MaterialID] = m
	}
	if len(actual) != len(imp.Materials) {
		return false
	}
	for _, m := range imp.Materials {
		got, exists := actual[m.Before.ID]
		if !exists || !materialOutcomeMatches(got, m, aggregate) {
			return false
		}
	}
	return true
}

// Final receipts and retry exclusion use the same actual material checks.
// An acknowledged unknown disposition permits execution, never completion.
func materialOutcomeMatches(got MaterialOutcome, expected MaterialConsequence, aggregate []ProviderImpact) bool {
	return got.MaterialID == expected.Before.ID && got.Disposition != "unknown" && got.Verified && got.Evidence != "" &&
		got.Disposition == expected.After.Disposition && sameLocation(got.Location, expected.After.Destination) &&
		sameAccess(got.Access, expected.After.AccessAfter) && recoveryPreserved(expectedMaterialRecovery(expected, aggregate), got.Recovery)
}

func sameLocation(a, b *ResourceLocation) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func recoveryPreserved(before, after []ResourceLocation) bool {
	for _, expected := range before {
		found := false
		for _, actual := range after {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func sameAccess(a, b *ProviderAccess) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
