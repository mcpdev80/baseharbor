package application

import "fmt"

type ProviderChangeError struct{ Code string }

func (e *ProviderChangeError) Error() string { return "provider change: " + e.Code }

// ValidateProviderChangeConsent is shared by CLI, JSON and MCP authorities.
// Complete=false never vetoes an informed developer's choice. Lack of an
// alternative requires two stages; ordinary consent cannot imply data deletion.
func ValidateProviderChangeConsent(current ProviderImpactPlan, consent ProviderChangeConsent) error {
	if current.Version != ProviderImpactVersion || current.PlanID == "" || current.ConfirmationsRequired < 1 || current.ConfirmationsRequired > 2 {
		return &ProviderChangeError{Code: "invalid-impact-plan"}
	}
	if consent.PlanID != current.PlanID {
		return &ProviderChangeError{Code: "stale-impact-consent"}
	}
	if !consent.First || (current.ConfirmationsRequired == 2 && !consent.Second) {
		return &ProviderChangeError{Code: "impact-confirmation-required"}
	}
	if current.Destructive && !consent.DeleteData {
		return &ProviderChangeError{Code: "data-deletion-consent-required"}
	}
	return nil
}

// PendingProviderChanges returns only unverified/unfinished operations. It does
// not schedule or mutate; the existing lifecycle replans and obtains fresh
// consent if the resulting plan changes. Applied outcomes are not replayed.
func PendingProviderChanges(plan ProviderImpactPlan, result ProviderChangeResult) ([]ProviderChangeRequest, error) {
	if result.PlanID != plan.PlanID {
		return nil, &ProviderChangeError{Code: "stale-impact-result"}
	}
	outcomes := map[string]string{}
	for _, out := range result.Outcomes {
		key := out.InstanceID + "/" + out.Operation
		if _, duplicate := outcomes[key]; duplicate {
			return nil, fmt.Errorf("duplicate provider outcome")
		}
		outcomes[key] = out.Status
	}
	pending := []ProviderChangeRequest{}
	for _, impact := range plan.Impacts {
		if outcomes[impact.InstanceID+"/"+impact.Operation] != "applied" {
			r := ProviderChangeRequest{InstanceID: impact.InstanceID, Operation: impact.Operation, Selection: impact.Selection}
			if impact.After != nil && impact.After.InstanceID != impact.InstanceID {
				r.ReplacementID = impact.After.InstanceID
			}
			for _, m := range impact.Materials {
				r.Materials = append(r.Materials, m.After)
			}
			pending = append(pending, r)
		}
	}
	return pending, nil
}
