package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// The domain plan determines acknowledgement requirements. Presentation does
// not decide whether the developer may accept unavailable functionality.
func acknowledgeProviderImpact(ctx context.Context, in io.Reader, out io.Writer, plan application.ProviderImpactPlan, supplied *application.ProviderChangeConsent) error {
	if out == nil {
		out = io.Discard
	}
	if err := printProviderImpact(out, plan); err != nil {
		return err
	}
	if supplied != nil {
		return providerImpactConsentError(plan, application.ValidateProviderChangeConsent(plan, *supplied))
	}
	// --yes and ordinary apply approval deliberately do not imply either
	// impact acknowledgement, even when the plan is incomplete.
	if noInput(ctx) || in == nil || !readerIsTerminal(in) {
		return providerImpactConsentError(plan, application.ValidateProviderChangeConsent(plan, application.ProviderChangeConsent{PlanID: plan.PlanID}))
	}
	consent, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, plan)
	if err != nil {
		return err
	}
	return providerImpactConsentError(plan, application.ValidateProviderChangeConsent(plan, consent))
}

func printProviderImpact(out io.Writer, plan application.ProviderImpactPlan) error {
	if plan.Attention != "" {
		fmt.Fprintln(out, "Achtung: review the concrete consequences and required acknowledgements below.")
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(plan)
}

func providerImpactConsentError(plan application.ProviderImpactPlan, err error) error {
	classified := classifyProviderChangeConsent(err)
	var machineError *machine.Error
	if errors.As(classified, &machineError) {
		machineError.Details = plan
	}
	return classified
}

func readProviderImpactAcknowledgements(reader *bufio.Reader, out io.Writer, plan application.ProviderImpactPlan) (application.ProviderChangeConsent, error) {
	consent := application.ProviderChangeConsent{PlanID: plan.PlanID}
	for stage := 1; stage <= plan.ConfirmationsRequired; stage++ {
		label := "Confirm the displayed provider changes"
		if plan.Destructive {
			label = "Achtung: confirm the displayed changes and permanent data deletion"
		} else if plan.ConfirmationsRequired == 2 {
			label = "Achtung: confirm the displayed outcome without a working alternative"
		}
		fmt.Fprintf(out, "%s (%d/%d, plan %s)? [y/N]: ", label, stage, plan.ConfirmationsRequired, plan.PlanID)
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return consent, err
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return consent, nil
		}
		if stage == 1 {
			consent.First = true
		} else {
			consent.Second = true
		}
	}
	// The two consequence acknowledgements do not grant data deletion. Collect
	// that separate decision only after every required plan stage was accepted.
	if plan.Destructive {
		fmt.Fprintf(out, "Achtung: separately authorize permanent deletion of the displayed owned data (plan %s)? [y/N]: ", plan.PlanID)
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return consent, err
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		consent.DeleteData = answer == "y" || answer == "yes"
	}
	return consent, nil
}

func classifyProviderChangeConsent(err error) error {
	if err == nil {
		return nil
	}
	var change *application.ProviderChangeError
	if !errors.As(err, &change) {
		return err
	}
	code := machine.ErrorApprovalRequired
	if change.Code == "stale-impact-consent" {
		code = machine.ErrorConflict
	} else if change.Code == "invalid-impact-plan" {
		code = machine.ErrorValidationFailed
	} else if strings.Contains(change.Code, "ownership") || strings.HasPrefix(change.Code, "foreign-") || change.Code == "external-provider-runtime-not-owned" {
		code = machine.ErrorOwnershipAmbiguous
	} else if change.Code == "unsupported-change-operation" {
		code = machine.ErrorUnsupported
	} else if change.Code == "provider-instance-missing" || change.Code == "replacement-provider-missing" {
		code = machine.ErrorNotFound
	}
	return &machine.Error{Code: code, CauseCode: change.Code, Message: err.Error(), Next: "Review the current concrete provider impact plan and explicitly acknowledge its required stages; data deletion requires separate explicit consent.", Cause: err}
}
