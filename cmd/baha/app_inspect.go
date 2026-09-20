package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func appInspectCommand() *cli.Command {
	return &cli.Command{
		Name:    "inspect",
		Summary: "Inspect a repository without changing it",
		Usage:   "baha app inspect [PATH] [-o json|--output json|--json]",
		Long:    "Analyzes repository evidence read-only and reports deterministic capability findings as detected, suggested or possible. -o json, --output json and the compatibility alias --json emit the shared machine-readable result used by future API/Web UI/Operator adapters.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			root, format, err := parseAppInspectArgs(args)
			if err != nil {
				return err
			}
			result, err := repositoryinspect.Inspect(ctx, root)
			if err != nil {
				return err
			}
			if format == outputJSON {
				data, err := repositoryinspect.MarshalJSONResult(result)
				if err != nil {
					return err
				}
				fmt.Fprintln(out, string(data))
				return nil
			}
			printRepositoryInspection(out, result)
			return nil
		},
	}
}

func parseAppInspectArgs(args []string) (string, cliOutputFormat, error) {
	filtered, format, err := parseReadOutputArgs(args, "app inspect")
	if err != nil {
		return "", "", err
	}
	root := "."
	if len(filtered) > 1 {
		return "", "", usageError("baha app inspect accepts at most one PATH", "Example: baha app inspect . -o json")
	}
	if len(filtered) == 1 {
		if strings.HasPrefix(filtered[0], "-") {
			return "", "", usageError("unknown option "+filtered[0], "Run 'baha app inspect --help' for usage.")
		}
		root = filtered[0]
	}
	return root, format, nil
}
func printRepositoryInspection(out io.Writer, result repositoryinspect.Result) {
	fmt.Fprintf(out, "Repository inspection: %s\n", result.Root)
	fmt.Fprintf(out, "Application: %s\n", result.Application)
	if result.ExistingManifest != "" {
		fmt.Fprintf(out, "Existing BaseHarbor manifest: %s\n", result.ExistingManifest)
	}

	if len(result.ComposeCandidates) == 1 {
		fmt.Fprintf(out, "Compose: %s\n", result.ComposeCandidates[0])
	} else if len(result.ComposeCandidates) > 1 {
		fmt.Fprintf(out, "Compose: %d candidates (no automatic selection)\n", len(result.ComposeCandidates))
		for _, path := range result.ComposeCandidates {
			fmt.Fprintf(out, "  - %s\n", path)
		}
	}
	if len(result.WorkloadServices) > 0 {
		fmt.Fprintf(out, "Workload services: %s\n", strings.Join(result.WorkloadServices, ", "))
	}
	if len(result.Artifacts) > 0 {
		fmt.Fprintln(out, "Artifacts:")
		for _, artifact := range result.Artifacts {
			fmt.Fprintf(out, "  - %s: %s\n", artifact.Kind, artifact.Path)
		}
	}

	printInspectionFindings(out, result, repositoryinspect.ConfidenceDetected, "Detected")
	printInspectionFindings(out, result, repositoryinspect.ConfidenceSuggested, "Suggested")
	printInspectionFindings(out, result, repositoryinspect.ConfidencePossible, "Possible")
	printInspectionReconciliation(out, result)

	if len(result.RequiredSecrets) > 0 {
		fmt.Fprintln(out, "\nRequired secrets from BaseHarbor contract:")
		for _, name := range result.RequiredSecrets {
			fmt.Fprintf(out, "  - %s\n", name)
		}
	}
	if len(result.SecretCandidates) > 0 {
		fmt.Fprintln(out, "\nPotential secret inputs from repository evidence:")
		for _, name := range result.SecretCandidates {
			fmt.Fprintf(out, "  - %s (%s)\n", name, result.SecretSources[name])
		}
	}
	if len(result.Ports) > 0 {
		fmt.Fprintln(out, "\nPublished ports:")
		for _, port := range result.Ports {
			if port.Service != "" {
				fmt.Fprintf(out, "  - %s: %s (%s)\n", port.Service, port.Value, port.Path)
			} else {
				fmt.Fprintf(out, "  - %s (%s)\n", port.Value, port.Path)
			}
		}
	}
	if len(result.HealthChecks) > 0 {
		fmt.Fprintln(out, "\nHealth checks:")
		for _, evidence := range result.HealthChecks {
			fmt.Fprintf(out, "  - %s: %s\n", evidence.Path, evidence.Detail)
		}
	}
	fmt.Fprintln(out, "\nNo changes were made.")
}

func printInspectionReconciliation(out io.Writer, result repositoryinspect.Result) {
	if len(result.Reconciliation) == 0 {
		return
	}
	fmt.Fprintln(out, "\nContract reconciliation:")
	for _, item := range result.Reconciliation {
		label := item.Capability
		if item.Name != "" {
			label += "/" + item.Name
		}
		direction := ""
		if item.Direction != "" {
			direction = " " + string(item.Direction)
		}
		operations := ""
		if len(item.Operations) > 0 {
			values := make([]string, 0, len(item.Operations))
			for _, operation := range item.Operations {
				values = append(values, string(operation))
			}
			operations = " [" + strings.Join(values, ", ") + "]"
		}
		fmt.Fprintf(out, "  - %-10s %s%s%s\n", strings.ToUpper(string(item.State)), label, direction, operations)
	}
	fmt.Fprintln(out, "  Stale means current inspection found no supporting evidence; it never removes declared intent.")
	fmt.Fprintln(out, "  Detected runtime operations are evidence only and never grant authorization.")
}

func printInspectionFindings(out io.Writer, result repositoryinspect.Result, confidence repositoryinspect.Confidence, title string) {
	var findings []repositoryinspect.Finding
	for _, finding := range result.Findings {
		if finding.Confidence == confidence {
			findings = append(findings, finding)
		}
	}
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s:\n", title)
	for _, finding := range findings {
		label := finding.Capability
		if finding.Name != "" {
			label += "/" + finding.Name
		}
		fmt.Fprintf(out, "  - %s\n", label)
		for _, evidence := range finding.Evidence {
			fmt.Fprintf(out, "      %s: %s (%s)\n", evidence.Kind, evidence.Detail, evidence.Path)
		}
	}
}
