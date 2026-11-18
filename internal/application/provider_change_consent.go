package application

import "fmt"

// ProviderImpactConsentLabels is the minimal i18n surface for the CLI's
// destructive / consent-gated paths. The regression these labels fix is the
// rendered CLI string: every label below is the English catalog value that the
// CLI must print instead of a hardcoded German fallback. The wire model value
// ProviderImpactPlan.Attention ("Achtung") stays language-neutral and is
// untouched here — this is a presentation-layer catalog only.
type ProviderImpactConsentLabels struct {
	ControlPlaneStopImpact               string
	NodeDisconnectImpact                 string
	TargetDefinitionChangeImpact         string
	ApplicationStopImpact                string
	RuntimeDestroyImpactDestructive      string
	RuntimeDestroyImpactNonDestructive   string
	ProviderImpactLead                   string
	ProviderDestructiveConfirm           string
	ProviderTwoStageConfirm              string
	ProviderDestructiveDataConsentPrompt string
	ApplicationRestoreImpact             string
	ExternalProviderRemovalImpact        string
}

// ProviderImpactConsentCatalog returns the shipped English consent labels.
func ProviderImpactConsentCatalog() ProviderImpactConsentLabels {
	return ProviderImpactConsentLabels{
		ControlPlaneStopImpact:               "Warning: review this selected Core stop, affected consumers and retained recovery locations.",
		NodeDisconnectImpact:                 "Warning: review node access loss and all known Core/local recovery owners.",
		TargetDefinitionChangeImpact:         "Warning: review this Target access change, retained data and actual recovery location.",
		ApplicationStopImpact:                "Warning: review the current application stop, affected consumers and retained recovery locations.",
		RuntimeDestroyImpactDestructive:      "Warning: permanent destruction requires two current-plan confirmations and separate data-deletion consent.",
		RuntimeDestroyImpactNonDestructive:   "Warning: selected runtime removal requires two current-plan confirmations; persistent data and protected owners remain.",
		ProviderImpactLead:                   "Warning: review the concrete consequences and required acknowledgements below.",
		ProviderDestructiveConfirm:           "Warning: confirm the displayed changes and permanent data deletion",
		ProviderTwoStageConfirm:              "Warning: confirm the displayed outcome without a working alternative",
		ProviderDestructiveDataConsentPrompt: "Warning: separately authorize permanent deletion of the displayed owned data (plan %s)? [y/N]: ",
		ApplicationRestoreImpact:             "Warning: restore can overwrite newer owned data. Confirm this exact archive/owner plan twice and separately authorize data deletion; ordinary approval supplies none of these decisions.",
		ExternalProviderRemovalImpact:        "Warning: review all Target bindings and the unknown live access consequences.",
	}
}

type ProviderChangeError struct{ Code string }

func (e *ProviderChangeError) Error() string { return "provider change: " + e.Code }

// ValidateProviderChangeConsent is shared by CLI, JSON and MCP authorities.
// Complete=false never vetoes an informed developer's choice. Lack of an
// alternative requires both acknowledgement fields; ordinary consent cannot
// imply data deletion.
func ValidateProviderChangeConsent(current ProviderImpactPlan, consent ProviderChangeConsent) error {
	if err := validateProviderConsentRequirements(current.Version, current.PlanID, current.ConfirmationsRequired); err != nil {
		return err
	}
	return validateProviderConsent(current.PlanID, current.ConfirmationsRequired, current.Destructive, consent)
}

func validateProviderConsentRequirements(version, planID string, confirmations int) error {
	if version != ProviderImpactVersion || planID == "" || confirmations < 1 || confirmations > 2 {
		return &ProviderChangeError{Code: "invalid-impact-plan"}
	}
	return nil
}

func validateProviderConsent(planID string, confirmations int, destructive bool, consent ProviderChangeConsent) error {
	if consent.PlanID != planID {
		return &ProviderChangeError{Code: "stale-impact-consent"}
	}
	if !consent.First || (confirmations == 2 && !consent.Second) {
		return &ProviderChangeError{Code: "impact-confirmation-required"}
	}
	if destructive && !consent.DeleteData {
		return &ProviderChangeError{Code: "data-deletion-consent-required"}
	}
	return nil
}

// PendingProviderChanges returns only unverified/unfinished operations. It does
// not schedule or mutate; the existing lifecycle replans and obtains fresh
// consent if the resulting plan changes. Applied outcomes are not replayed.
func PendingProviderChanges(plan ProviderImpactPlan, result ProviderChangeResult) ([]ProviderChangeRequest, error) {
	if result.Version != ProviderImpactVersion || result.PlanID != plan.PlanID {
		return nil, &ProviderChangeError{Code: "stale-impact-result"}
	}
	validated, err := RecordProviderChangeResult(plan, result.Outcomes)
	if err != nil {
		return nil, err
	}
	outcomes := map[string]string{}
	actual := map[string]ProviderExecutionOutcome{}
	for _, out := range validated.Outcomes {
		key := out.InstanceID + "/" + out.Operation
		if _, duplicate := outcomes[key]; duplicate {
			return nil, fmt.Errorf("duplicate provider outcome")
		}
		outcomes[key] = out.Status
		actual[key] = out
	}
	pending := []ProviderChangeRequest{}
	for _, impact := range plan.Impacts {
		if outcomes[impact.InstanceID+"/"+impact.Operation] != "applied" {
			r := ProviderChangeRequest{InstanceID: impact.InstanceID, Operation: impact.Operation, Selection: impact.Selection, Consumers: append([]string{}, impact.RequestedConsumers...)}
			for _, f := range impact.Functions {
				if f.Transition != nil && !containsString(r.ConsumerActions, f.Transition.ID) {
					r.ConsumerActions = append(r.ConsumerActions, f.Transition.ID)
				}
			}
			if impact.After != nil && impact.After.InstanceID != impact.InstanceID {
				r.ReplacementID = impact.After.InstanceID
			}
			for _, m := range impact.Materials {
				done := false
				for _, got := range actual[impact.InstanceID+"/"+impact.Operation].Materials {
					if materialOutcomeMatches(got, m, plan.Impacts) {
						done = true
					}
				}
				if !done {
					r.Materials = append(r.Materials, m.After)
				}
			}
			pending = append(pending, r)
		}
	}
	return pending, nil
}
