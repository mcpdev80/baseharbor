package main

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// appChangePreview renders authoritative impact facts only. It does not grant
// consent, invent mitigations, or execute any provider operation.
func appChangePreview(plan application.ProviderImpactPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Provider change preview | Target: %s | complete: %t | destructive: %t\n", plan.Target, plan.Complete, plan.Destructive)
	fmt.Fprintf(&b, "Existing consent authority requires %d confirmation(s)\n", plan.ConfirmationsRequired)
	for _, impact := range plan.Impacts {
		fmt.Fprintf(&b, "\n%s: %s | scenario: %s | selection: %s\n", impact.InstanceID, impact.Operation, impact.Scenario, impact.Selection)
		fmt.Fprintf(&b, "Before: %s | after: ", impact.Before.State)
		if impact.After == nil {
			b.WriteString("no proposed runtime realization\n")
		} else {
			fmt.Fprintf(&b, "%s (proposed, not verified)\n", impact.After.State)
		}
		for _, dependent := range impact.Dependents {
			fmt.Fprintf(&b, "Dependent: %s\n", dependent)
		}
		for _, f := range impact.Functions {
			fmt.Fprintf(&b, "Consumer: %s | function: %s | outcome: %s | required: %t\n", f.Consumer, f.Function, f.Outcome, f.Required)
			if f.ValidUntil != "" {
				fmt.Fprintf(&b, "Validity until: %s\n", f.ValidUntil)
			}
			if f.Transition != nil {
				fmt.Fprintf(&b, "Consumer action: %s | verified: %t\n", f.Transition.ID, f.Transition.Verified)
			}
		}
		for _, material := range impact.Materials {
			fmt.Fprintf(&b, "Material: %s | kind: %s | disposition: %s | verified: %t\n",
				material.Before.ID, material.Before.Kind, material.After.Disposition, material.After.Verified)
			if material.Before.Location != nil {
				appChangeLocation(&b, "Before location", *material.Before.Location)
			}
			if material.After.Destination != nil {
				appChangeLocation(&b, "Proposed destination", *material.After.Destination)
			}
			for _, recovery := range material.Before.Recovery {
				appChangeLocation(&b, "Observed recovery", recovery)
			}
		}
		for _, location := range impact.Before.Locations {
			appChangeLocation(&b, "Observed location", location)
		}
		for _, effect := range impact.Effects {
			fmt.Fprintf(&b, "Operational effect: %s | before: %s | after: %s\n", effect.Aspect, effect.Before, effect.After)
		}
		for _, prerequisite := range impact.Prerequisites {
			fmt.Fprintf(&b, "Required action: %s | %s | supported: %t | verified: %t\n",
				prerequisite.Kind, prerequisite.Subject, prerequisite.Supported, prerequisite.Verified)
		}
		for _, dependency := range impact.ContinuationDependencies {
			fmt.Fprintf(&b, "Continuation dependency: %s\n", dependency.Realization.InstanceID)
		}
		for _, notice := range impact.Notices {
			fmt.Fprintf(&b, "Plan notice: %s\n", notice)
		}
	}
	if !plan.Complete {
		b.WriteString("\nImpact information is incomplete; no unobserved outcome is asserted.\n")
	}
	return b.String()
}

// ResourceLocation fields originate from the authoritative observation. Keep
// access/secret payloads, tokens and evidence out of these human views.
func appChangeLocation(b *strings.Builder, label string, location application.ResourceLocation) {
	fmt.Fprintf(b, "%s: %s | %s | %s | %s\n", label, location.Owner, location.Target, location.Kind, location.Reference)
}
