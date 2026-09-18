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
		Usage:   "baha app inspect [PATH] [--json]",
		Long:    "Analyzes repository evidence read-only and reports deterministic capability findings as detected, suggested or possible. --json emits the shared machine-readable result used by future API/Web UI/Operator adapters.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			root, jsonOutput, err := parseAppInspectArgs(args)
			if err != nil {
				return err
			}
			result, err := repositoryinspect.Inspect(ctx, root)
			if err != nil {
				return err
			}
			if jsonOutput {
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

func parseAppInspectArgs(args []string) (string, bool, error) {
	root := "."
	jsonOutput := false
	pathSet := false
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOutput = true
		case strings.HasPrefix(arg, "-"):
			return "", false, usageError("unknown option "+arg, "Run 'baha app inspect --help' for usage.")
		default:
			if pathSet {
				return "", false, usageError("baha app inspect accepts at most one PATH", "Example: baha app inspect . --json")
			}
			root = arg
			pathSet = true
		}
	}
	return root, jsonOutput, nil
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

	if len(result.SecretCandidates) > 0 {
		fmt.Fprintln(out, "\nPotential required secrets:")
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
