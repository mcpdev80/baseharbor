package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machinereadmodels"
)

// appVersionsCommand is the aggregate readable version view across recorded
// deployments (and, with --all-targets, across every configured Target). It is
// the primary UI for release/component versions: one row per deployment with
// the wanted, applied and observed reference dimensions kept in three strictly
// separate cells. Read-only: no registry query, no mutation.
func appVersionsCommand() *cli.Command {
	return &cli.Command{
		Name:    "versions",
		Summary: "Show readable wanted/applied/observed versions for recorded deployments",
		Usage:   "baha app versions [--all-targets] [-o json|--output json]",
		Long:    "Renders each deployment's three independent reference dimensions (wanted, applied, observed) as separate cells sourced from the recorded stage versions. A missing dimension shows UNCLEAR instead of a blank cell. --all-targets widens the listing to every configured Target through the existing display seam.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app versions")
			if err != nil {
				return err
			}
			allTargets := false
			for _, arg := range filtered {
				switch arg {
				case "--all-targets":
					allTargets = true
				default:
					return unknownOptionUsage("baha app versions", arg, "--all-targets")
				}
			}
			result, err := collectMachineApplicationVersions(ctx, machineApplicationVersionsInput{AllTargets: allTargets})
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			if len(result.Releases) == 0 {
				fmt.Fprintln(out, "No deployments registered.")
				return nil
			}
			for i, view := range result.Releases {
				if i > 0 {
					fmt.Fprintln(out)
				}
				fmt.Fprintf(out, "%s/%s (%s)\n", view.Identity.Application, view.Identity.Environment, view.Identity.Target)
				fmt.Fprintf(out, "  wanted=%s  applied=%s  observed=%s\n",
					stageCellSummary(view.Stages.Wanted),
					stageCellSummary(view.Stages.Applied),
					stageCellSummary(view.Stages.Observed),
				)
			}
			for _, warning := range result.Warnings {
				fmt.Fprintf(errOut, "[WARN] %s\n", warning)
			}
			return nil
		},
	}
}

type machineApplicationVersionsInput struct {
	AllTargets bool   `json:"all_targets,omitempty"`
	Target     string `json:"target,omitempty" jsonschema:"optional BaseHarbor target; defaults to the effective target"`
}

func collectMachineApplicationVersions(ctx context.Context, input machineApplicationVersionsInput) (machinereadmodels.ApplicationVersionsResult, error) {
	var (
		items    []deployment.DeploymentRecord
		warnings []error
		err      error
	)
	if input.AllTargets {
		items, warnings, err = deployment.ListAllDeploymentsForDisplay()
	} else {
		targetName := strings.TrimSpace(input.Target)
		if targetName == "" {
			target, resolveErr := effectiveTarget(ctx)
			if resolveErr != nil {
				return machinereadmodels.ApplicationVersionsResult{}, resolveErr
			}
			targetName = target.Name
		}
		items, warnings, err = deployment.ListDeploymentsForDisplay(targetName)
	}
	if err != nil {
		return machinereadmodels.ApplicationVersionsResult{}, err
	}
	result := machinereadmodels.ApplicationVersionsResult{
		ContractVersion: machinereadmodels.ReleaseViewContractVersion,
		Releases:        machinereadmodels.ReleaseViewsFromRecords(items),
	}
	for _, warning := range warnings {
		result.Warnings = append(result.Warnings, warning.Error())
	}
	return result, nil
}

func stageCellSummary(cell *machinereadmodels.ViewStageCell) string {
	if cell == nil || cell.MissingData {
		return machinereadmodels.UnclearSummary
	}
	if cell.Stale {
		return cell.Reference + " [stale]"
	}
	return cell.Reference
}
