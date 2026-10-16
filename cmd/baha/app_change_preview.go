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
		fmt.Fprintf(&b, "Before: %s | verified: %t | after: ", impact.Before.State, impact.Before.Verified)
		appChangeSafeAccess(&b, "Before access", impact.Before.Access)
		if impact.After == nil {
			b.WriteString("no proposed runtime realization\n")
		} else {
			fmt.Fprintf(&b, "%s (proposed, not verified)\n", impact.After.State)
			appChangeSafeAccess(&b, "Proposed access (unverified)", impact.After.Access)
		}
		for _, dependent := range impact.Dependents {
			fmt.Fprintf(&b, "Dependent: %s\n", dependent)
		}
		for _, f := range impact.Functions {
			fmt.Fprintf(&b, "Consumer: %s | function: %s | outcome: %s | required: %t\n", f.Consumer, f.Function, f.Outcome, f.Required)
			if f.Behavior != "" { fmt.Fprintf(&b, "Observed function behavior: %s\n", f.Behavior) }
			if f.RemainingValiditySeconds != nil { fmt.Fprintf(&b, "Remaining function validity (seconds): %d\n", *f.RemainingValiditySeconds) }
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
			if material.Meaning != "" { fmt.Fprintf(&b, "Authoritative material meaning: %s\n", material.Meaning) }
			if material.Before.ValidFrom != "" { fmt.Fprintf(&b, "Material valid from: %s\n", material.Before.ValidFrom) }
			if material.Before.ExpiresAt != "" { fmt.Fprintf(&b, "Material expires at: %s\n", material.Before.ExpiresAt) }
			if material.RemainingValiditySeconds != nil { fmt.Fprintf(&b, "Remaining material validity (seconds): %d\n", *material.RemainingValiditySeconds) }
			appChangeSafeAccess(&b, "Before material access", material.Before.Access)
			appChangeSafeAccess(&b, "Proposed material access (unverified)", material.After.AccessAfter)
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
			if prerequisite.Consequence != "" { fmt.Fprintf(&b, "Prerequisite consequence: %s\n", prerequisite.Consequence) }
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

// appChangeSafeAccess emits structural, non-secret security properties only.
// Never print methods/endpoints, references, names, identities, or credentials.
func appChangeSafeAccess(b *strings.Builder, label string, access *application.ProviderAccess) {
	if access == nil {
		fmt.Fprintf(b, "%s: unknown/not reported\n", label)
		return
	}
	s := access.Security
	fmt.Fprintf(b, "%s: reported | identity configured: %t | credential references: %d | trust references: %d | authorization entries: %d | secret references: %d | renewable: %t | rotatable: %t | revocable: %t\n",
		label, s.Identity != nil, len(s.Credentials), len(s.Trust), len(s.Authorization), len(s.Secrets),
		s.Lifecycle.Renewable, s.Lifecycle.Rotatable, s.Lifecycle.Revocable)
}
