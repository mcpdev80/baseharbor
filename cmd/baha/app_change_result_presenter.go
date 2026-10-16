package main

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// appChangeResultPresenter renders recorded outcomes, never planned After
// values as executed state. The domain authority supplies all dispositions.
func appChangeResultPresenter(plan application.ProviderImpactPlan, result application.ProviderChangeResult) (string, error) {
	if result.Version != application.ProviderImpactVersion || result.PlanID != plan.PlanID {
		return "", &application.ProviderChangeError{Code: "stale-impact-result"}
	}
	verified, err := application.RecordProviderChangeResult(plan, result.Outcomes)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Provider change result: %s | Target: %s\n", verified.Status, plan.Target)
	for _, out := range verified.Outcomes {
		fmt.Fprintf(&b, "\n%s: %s | status: %s | verified: %t\n", out.InstanceID, out.Operation, out.Status, out.Verified)
		if out.ErrorCode != "" {
			fmt.Fprintf(&b, "Result code: %s\n", out.ErrorCode)
		}
		if out.After != nil {
			fmt.Fprintf(&b, "Observed state: %s | verified: %t\n", out.After.State, out.After.Verified)
			appChangeSafeAccess(&b, "Actual provider access", out.After.Access)
			for _, location := range out.After.Locations {
				appChangeLocation(&b, "Observed location", location)
			}
		} else {
			b.WriteString("No verified resulting runtime location asserted\n")
		}
		for _, material := range out.Materials {
			fmt.Fprintf(&b, "Actual material: %s | disposition: %s | verified: %t\n",
				material.MaterialID, material.Disposition, material.Verified)
			appChangeSafeAccess(&b, "Actual material access", material.Access)
			if material.Location != nil {
				appChangeLocation(&b, "Actual location", *material.Location)
			}
			for _, recovery := range material.Recovery {
				appChangeLocation(&b, "Observed recovery", recovery)
			}
		}
		for _, function := range out.Functions {
			fmt.Fprintf(&b, "Actual function: %s/%s | %s | verified: %t\n",
				function.Consumer, function.Function, function.Outcome, function.Verified)
			if function.Behavior != "" { fmt.Fprintf(&b, "Actual function behavior: %s\n", function.Behavior) }
			if function.ValidUntil != "" {
				fmt.Fprintf(&b, "Validity until: %s\n", function.ValidUntil)
			}
		}
		for _, effect := range out.Effects {
			fmt.Fprintf(&b, "Observed operational effect: %s | before: %s | after: %s\n", effect.Aspect, effect.Before, effect.After)
		}
	}
	pending, err := application.PendingProviderChanges(plan, verified)
	if err != nil {
		return "", err
	}
	if len(pending) > 0 {
		b.WriteString("\nUnfinished changes (replan and obtain fresh consent before retry):\n")
		for _, change := range pending {
			fmt.Fprintf(&b, "%s: %s\n", change.InstanceID, change.Operation)
		}
	} else {
		b.WriteString("\nNo unfinished provider operations reported.\n")
	}
	return b.String(), nil
}
