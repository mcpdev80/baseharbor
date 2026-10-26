package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"

	"github.com/mcpdev80/baseharbor/internal/cli"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func appInspectCommand() *cli.Command {
	return &cli.Command{
		Name:    "inspect",
		Summary: "Inspect a repository without changing it",
		Usage:   "baha app inspect [PATH] [--verbose] [-o json|--output json|--json]",
		Long:    "Analyzes a local repository/path or remote Git URL read-only. Human output summarizes source-neutral workload evidence and detected capability intent; --verbose adds source provenance and detailed evidence. -o json, --output json and --json emit the complete shared machine-readable result.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			root, format, verbose, err := parseAppInspectArgs(args)
			if err != nil {
				return err
			}
			result, err := inspectRepositorySource(ctx, root)
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
			printRepositoryInspection(out, result, verbose)
			return nil
		},
	}
}

func parseAppInspectArgs(args []string) (string, cliOutputFormat, bool, error) {
	verbose := false
	filteredArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--verbose" {
			verbose = true
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}
	filtered, format, err := parseReadOutputArgs(filteredArgs, "app inspect")
	if err != nil {
		return "", "", false, err
	}
	root := "."
	if len(filtered) > 1 {
		return "", "", false, usageError("baha app inspect accepts at most one PATH", "Example: baha app inspect . --verbose")
	}
	if len(filtered) == 1 {
		if strings.HasPrefix(filtered[0], "-") {
			return "", "", false, usageError("unknown option "+filtered[0], "Run 'baha app inspect --help' for usage.")
		}
		root = filtered[0]
	}
	return root, format, verbose, nil
}
func printRepositoryInspection(out io.Writer, result repositoryinspect.Result, verbose bool) {
	fmt.Fprintf(out, "Repository inspection: %s\n", result.Root)
	fmt.Fprintf(out, "Application: %s\n", result.Application)
	if result.ExistingManifest != "" {
		fmt.Fprintf(out, "Existing BaseHarbor manifest: %s\n", result.ExistingManifest)
		if selection, err := application.ResolveRepositoryEnvironment(result.Root, ""); err == nil {
			if target, targetErr := effectiveTarget(context.Background()); targetErr == nil {
				auth := collectOperatorAuthObservation(context.Background(), target.Name, selection.Manifest.Environment)
				fmt.Fprintf(out, "Operator access: %s · %s · %s", auth.Mode, auth.Status, auth.Session)
				if auth.Provider != "" {
					fmt.Fprintf(out, " · %s", auth.Provider)
				}
				fmt.Fprintln(out)
			}
		}
	}

	fmt.Fprintf(out, "Workload source resolution: %s (%s)\n", result.WorkloadSourceResolution.State, result.WorkloadSourceResolution.Reason)
	if len(result.WorkloadSourceCandidates) > 0 {
		if result.SelectedWorkloadSource != nil {
			if len(result.WorkloadSourceCandidates) > 1 {
				fmt.Fprintf(out, "Workload source: %s (%s; selected from %d candidates)\n", result.SelectedWorkloadSource.Kind, result.SelectedWorkloadSource.Path, len(result.WorkloadSourceCandidates))
			} else {
				fmt.Fprintf(out, "Workload source: %s (%s)\n", result.SelectedWorkloadSource.Kind, result.SelectedWorkloadSource.Path)
			}
		} else {
			fmt.Fprintf(out, "Workload source: %d candidates (explicit selection required)\n", len(result.WorkloadSourceCandidates))
		}
		if verbose || result.SelectedWorkloadSource == nil || len(result.WorkloadSourceCandidates) > 1 {
			fmt.Fprintln(out, "Workload source candidates:")
			for _, candidate := range result.WorkloadSourceCandidates {
				fmt.Fprintf(out, "  - %s: %s\n", candidate.Kind, candidate.Path)
				if candidate.Kind == repositoryinspect.WorkloadSourceCompose {
					if analysis, err := repositoryinspect.AnalyzeComposeFile(result.Root, candidate.Path); err == nil {
						if len(analysis.WorkloadServices) > 0 {
							fmt.Fprintf(out, "      workload: %s\n", strings.Join(analysis.WorkloadServices, ", "))
						}
						if len(analysis.InfrastructureServices) > 0 {
							fmt.Fprintf(out, "      infrastructure: %s\n", strings.Join(analysis.InfrastructureServices, ", "))
						}
						if len(analysis.AmbiguousServices) > 0 {
							fmt.Fprintf(out, "      ambiguous: %s\n", strings.Join(analysis.AmbiguousServices, ", "))
						}
					}
				}
			}
		}
	}
	if result.WorkloadEvidence != nil && len(result.WorkloadEvidence.Components) > 0 {
		fmt.Fprintln(out, "Workload components:")
		for _, component := range result.WorkloadEvidence.Components {
			detail := ""
			if component.InfrastructureClass != "" {
				detail = " [" + component.InfrastructureClass + "]"
			}
			fmt.Fprintf(out, "  - %s%s\n", component.ID, detail)
			if verbose {
				for _, source := range component.Source {
					fmt.Fprintf(out, "      %s %s %s\n", source.Kind, source.Path, source.Resource)
				}
			}
		}
		if verbose {
			fmt.Fprintf(out, "Workload fingerprint: %s\n", result.WorkloadEvidence.Fingerprint)
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

	printInspectionSummary(out, result)
	if verbose {
		printInspectionFindings(out, result, repositoryinspect.ConfidenceDetected, "Detected evidence")
		printInspectionFindings(out, result, repositoryinspect.ConfidenceSuggested, "Suggested evidence")
		printInspectionFindings(out, result, repositoryinspect.ConfidencePossible, "Possible evidence")
		printInspectionReconciliation(out, result)
	}

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

func printInspectionSummary(out io.Writer, result repositoryinspect.Result) {
	if len(result.Findings) == 0 {
		return
	}
	fmt.Fprintln(out, "\nCapabilities:")
	for _, finding := range result.Findings {
		label := inspectionFindingLabel(finding)
		detail := string(finding.Confidence)
		if finding.Protocol != "" {
			detail += ", " + finding.Protocol
		}
		if finding.Name != "" {
			detail += ", " + finding.Name
		}
		if len(finding.Operations) > 0 {
			values := make([]string, 0, len(finding.Operations))
			for _, operation := range finding.Operations {
				values = append(values, string(operation))
			}
			detail += ", operations: " + strings.Join(values, ", ")
		}
		fmt.Fprintf(out, "  %-18s %s\n", label, detail)
	}
}

func inspectionFindingLabel(finding repositoryinspect.Finding) string {
	switch finding.Service {
	case "sql":
		return "SQL Database"
	case "cache":
		return "Cache"
	case "object-storage":
		return "Object Storage"
	case "observability":
		if finding.Capability == "metrics" {
			return "Metrics"
		}
		if finding.Capability == "logs" {
			return "Logs"
		}
		return "Observability"
	case "secrets":
		return "Secrets"
	case "identity":
		return "Identity"
	case "messaging":
		return "Messaging"
	case "vector":
		return "Vector"
	}
	if finding.Capability == "runtime-api" {
		return "Runtime API"
	}
	return finding.Capability
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
