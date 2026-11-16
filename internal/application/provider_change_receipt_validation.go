package application

// Validate read projections of persisted receipts without inferring execution.
func ValidateProviderChangeReceiptMetadata(result ProviderChangeResult, materials []ProviderMaterial) error {
	if result.Version != ProviderImpactVersion || result.PlanID == "" {
		return &ProviderChangeError{Code: "invalid-impact-result"}
	}
	switch result.Status {
	case "complete", "partial", "failed":
	default:
		return &ProviderChangeError{Code: "invalid-impact-result"}
	}
	if err := validateScopeStrings(result); err != nil {
		return err
	}
	for _, outcome := range result.Outcomes {
		if err := validateExecutionOutcome(outcome); err != nil {
			return err
		}
	}
	return validateImpactObservation(ProviderImpactObservation{Providers: map[string]ProviderImpactFact{"retained": {Realization: ProviderRealization{InstanceID: "retained"}, Materials: materials}}})
}
