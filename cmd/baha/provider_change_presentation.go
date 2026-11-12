package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func printProviderImpactForCLI(ctx context.Context, out io.Writer, plan application.ProviderImpactPlan) error {
	if cli.OutputOptionsFromContext(ctx).Verbose {
		return printProviderImpact(out, plan)
	}
	fmt.Fprintf(out, "\nProvider changes · Core Target %s\nPlan: %s\n", plan.Target, plan.PlanID)
	fmt.Fprintf(out, "%d provider action(s); %d confirmation(s) required.\n", len(plan.Impacts), plan.ConfirmationsRequired)
	if plan.Destructive {
		fmt.Fprintln(out, "Permanent data deletion: requested; separate explicit authorization is required.")
	} else {
		fmt.Fprintln(out, "Permanent data deletion: not requested by this plan.")
	}
	if !plan.Complete {
		fmt.Fprintln(out, "Achtung: inventory is incomplete. Unlisted material and functional continuity are not verified.")
	}
	for _, impact := range plan.Impacts {
		fmt.Fprintf(out, "\n  %s  %s\n", strings.ToUpper(impact.Operation), impact.InstanceID)
		after := "removed"
		if impact.After != nil {
			after = string(impact.After.State)
		}
		fmt.Fprintf(out, "    State: %s -> %s\n", impact.Before.State, after)
		for _, function := range impact.Functions {
			need := "optional"
			if function.Required {
				need = "required"
			}
			fmt.Fprintf(out, "    Function: %s -> %s (%s; consumer: %s)\n", function.Function, function.Outcome, need, function.Consumer)
			if function.Behavior != "" {
				fmt.Fprintf(out, "      %s\n", function.Behavior)
			}
			if function.ValidUntil != "" {
				fmt.Fprintf(out, "      Valid until: %s\n", function.ValidUntil)
			}
		}
		for _, material := range impact.Materials {
			fmt.Fprintf(out, "    Material: %s -> %s\n", material.Before.ID, material.After.Disposition)
			if material.Before.Location != nil {
				fmt.Fprintf(out, "      Location: %s (%s)\n", material.Before.Location.Reference, material.Before.Location.Target)
			}
			if material.Meaning != "" {
				fmt.Fprintf(out, "      %s\n", material.Meaning)
			}
			if material.After.Destination != nil {
				fmt.Fprintf(out, "      Destination: %s (%s)\n", material.After.Destination.Reference, material.After.Destination.Target)
			}
		}
		for _, effect := range impact.Effects {
			fmt.Fprintf(out, "    Effect %s: %s -> %s\n", effect.Aspect, effect.Before, effect.After)
		}
		for _, prerequisite := range impact.Prerequisites {
			fmt.Fprintf(out, "    Prerequisite %s: supported=%t verified=%t; %s\n", prerequisite.Subject, prerequisite.Supported, prerequisite.Verified, prerequisite.Consequence)
		}
		for _, notice := range impact.Notices {
			fmt.Fprintf(out, "    Note: %s\n", notice)
		}
	}
	fmt.Fprintln(out, "\nUse --verbose for the full structured provider plan.")
	return nil
}
