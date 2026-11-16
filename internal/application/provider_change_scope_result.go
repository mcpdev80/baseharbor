package application

// ProviderScopeOwnerOutcome concerns only the observed local registration
// entry. It never grants authority over the external provider or its material.
type ProviderScopeOwnerOutcome struct {
	Owner       ProviderScopeOwner `json:"owner"`
	Disposition string             `json:"disposition"`
	Verified    bool               `json:"verified"`
	Evidence    string             `json:"evidence,omitempty"`
}

type ProviderTargetChangeResult struct {
	Target string               `json:"target"`
	Result ProviderChangeResult `json:"result"`
}

type ProviderImpactScopeResult struct {
	Version string                       `json:"version"`
	PlanID  string                       `json:"plan_id"`
	Status  string                       `json:"status"`
	Owner   ProviderScopeOwnerOutcome    `json:"owner"`
	Targets []ProviderTargetChangeResult `json:"targets"`
}

// RecordProviderImpactScopeResult verifies each target through the existing
// receipt authority. Removing the global entry alone cannot become success for
// all consumers; missing/unknown target or owner evidence remains partial.
func RecordProviderImpactScopeResult(scope ProviderImpactScope, owner ProviderScopeOwnerOutcome, actual []ProviderTargetChangeResult) (ProviderImpactScopeResult, error) {
	r := ProviderImpactScopeResult{Version: ProviderImpactScopeVersion, PlanID: scope.PlanID, Status: "partial", Owner: owner, Targets: []ProviderTargetChangeResult{}}
	consent := ProviderChangeConsent{PlanID: scope.PlanID, First: true, Second: true, DeleteData: true}
	// Structural validation only: receipts do not acquire or imply consent.
	if err := ValidateProviderImpactScopeConsent(scope, consent); err != nil {
		return r, err
	}
	if owner.Owner != scope.Owner {
		return r, &ProviderChangeError{Code: "impact-scope-owner-outcome-mismatch"}
	}
	if owner.Disposition != "removed" && owner.Disposition != "retained" && owner.Disposition != "unknown" {
		return r, &ProviderChangeError{Code: "invalid-impact-scope-owner-outcome"}
	}
	if owner.Disposition == "removed" && (!owner.Verified || owner.Evidence == "") {
		return r, &ProviderChangeError{Code: "execution-verification-required"}
	}
	if err := validateScopeStrings(owner); err != nil {
		return r, err
	}
	plans := map[string]ProviderImpactPlan{}
	for _, p := range scope.Targets {
		plans[p.Target] = p
	}
	results := map[string]ProviderChangeResult{}
	for _, target := range actual {
		p, exists := plans[target.Target]
		if _, duplicate := results[target.Target]; duplicate || !exists || target.Result.PlanID != p.PlanID || target.Result.Version != ProviderImpactVersion {
			return r, &ProviderChangeError{Code: "impact-scope-target-result-mismatch"}
		}
		verified, err := RecordProviderChangeResult(p, target.Result.Outcomes)
		if err != nil {
			return r, err
		}
		results[target.Target] = verified
	}
	complete := scope.Complete && owner.Disposition == "removed" && owner.Verified && owner.Evidence != ""
	for _, p := range scope.Targets {
		result, exists := results[p.Target]
		if !exists {
			var err error
			result, err = RecordProviderChangeResult(p, nil)
			if err != nil {
				return r, err
			}
		}
		r.Targets = append(r.Targets, ProviderTargetChangeResult{Target: p.Target, Result: result})
		complete = complete && exists && result.Status == "complete"
	}
	if complete {
		r.Status = "complete"
	}
	return r, nil
}
