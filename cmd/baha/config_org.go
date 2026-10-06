package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

type organizationView struct {
	ContractVersion string                `json:"contract_version"`
	State           orgconfig.ActiveState `json:"state"`
	Effective       orgconfig.Effective   `json:"effective"`
}

type organizationCheckView struct {
	ContractVersion string                 `json:"contract_version"`
	Status          orgconfig.UpdateStatus `json:"status"`
	Available       orgconfig.Config       `json:"available"`
}

func configOrganizationCommand() *cli.Command {
	return &cli.Command{
		Name:    "organization",
		Summary: "Configure versioned organization/platform defaults and references",
		Usage:   "baha config organization set|show|check|update [options]",
		Long:    "Organization configuration composes existing BaseHarbor targets, providers, stack profiles, trust and policy references. It never changes portable Application Intent and never stores plaintext credentials or private keys.",
		Children: []*cli.Command{
			{
				Name:    "set",
				Summary: "Resolve and activate an organization configuration source",
				Usage:   "baha config organization set --source oci|git|local|system [--location LOCATION] [--requested TAG|REF] [--environment ENV] [-o json]",
				Run:     runOrganizationSet,
			},
			{
				Name:    "show",
				Summary: "Show the active organization configuration and effective defaults",
				Usage:   "baha config organization show [--environment ENV] [--preferences FILE] [-o json]",
				Run:     runOrganizationShow,
			},
			{
				Name:    "check",
				Summary: "Resolve the configured source without changing the active organization configuration",
				Usage:   "baha config organization check [-o json]",
				Run:     runOrganizationCheck,
			},
			{
				Name:    "update",
				Summary: "Explicitly activate the currently configured source at its newly resolved immutable version",
				Usage:   "baha config organization update --yes [--environment ENV] [-o json]",
				Run:     runOrganizationUpdate,
			},
		},
	}
}

func runOrganizationSet(ctx context.Context, args []string, out, errOut io.Writer) error {
	source, environment, format, err := parseOrganizationSetArgs(args)
	if err != nil {
		return err
	}
	state, err := orgconfig.Activate(ctx, source)
	if err != nil {
		return err
	}
	return writeOrganizationView(out, state, environment, format)
}

func runOrganizationShow(ctx context.Context, args []string, out, errOut io.Writer) error {
	args, preferences, err := readOrganizationPreferences(args)
	if err != nil {
		return err
	}
	environment, format, err := parseOrganizationReadArgs(args, "config organization show", false)
	if err != nil {
		return err
	}
	state, err := orgconfig.LoadActive()
	if err != nil {
		return err
	}
	return writeOrganizationView(out, state, environment, format, preferences...)
}

func runOrganizationCheck(ctx context.Context, args []string, out, errOut io.Writer) error {
	_, format, err := parseOrganizationReadArgs(args, "config organization check", false)
	if err != nil {
		return err
	}
	status, available, err := orgconfig.Check(ctx)
	if err != nil {
		return err
	}
	view := organizationCheckView{ContractVersion: orgconfig.ContractVersion, Status: status, Available: available}
	if format == outputJSON {
		return writeJSON(out, view)
	}
	fmt.Fprintf(out, "Organization source: %s %s\n", status.Current.Source.Kind, status.Current.Source.Location)
	fmt.Fprintf(out, "Current:   %s\n", organizationResolutionIdentity(status.Current))
	fmt.Fprintf(out, "Available: %s\n", organizationResolutionIdentity(status.Available))
	if status.Changed {
		fmt.Fprintln(out, "Update available. Active configuration is unchanged; run 'baha config organization update --yes' to activate it.")
	} else {
		fmt.Fprintln(out, "Organization configuration is current.")
	}
	return nil
}

func runOrganizationUpdate(ctx context.Context, args []string, out, errOut io.Writer) error {
	environment, format, err := parseOrganizationReadArgs(args, "config organization update", true)
	if err != nil {
		return err
	}
	state, err := orgconfig.Refresh(ctx)
	if err != nil {
		return err
	}
	return writeOrganizationView(out, state, environment, format)
}

func writeOrganizationView(out io.Writer, state orgconfig.ActiveState, environment string, format cliOutputFormat, preferences ...orgconfig.PreferenceLayer) error {
	effective, err := orgconfig.ResolveEffective(state, environment, preferences...)
	if err != nil {
		return err
	}
	view := organizationView{ContractVersion: orgconfig.ContractVersion, State: state, Effective: effective}
	if format == outputJSON {
		return writeJSON(out, view)
	}
	fmt.Fprintf(out, "Organization: %s\n", effective.Organization)
	fmt.Fprintf(out, "Environment:  %s\n", effective.Environment)
	fmt.Fprintf(out, "Source:       %s %s\n", state.Resolution.Source.Kind, state.Resolution.Source.Location)
	fmt.Fprintf(out, "Resolved:     %s\n", organizationResolutionIdentity(state.Resolution))
	if effective.Target != nil {
		fmt.Fprintf(out, "Target:       %s (%s)\n", effective.Target.Value, effective.Target.Source)
	}
	if effective.Stack != nil {
		fmt.Fprintf(out, "Stack:        %s (%s)\n", effective.Stack.Value, effective.Stack.Source)
	}
	for capability, provider := range effective.Providers {
		fmt.Fprintf(out, "Provider:     %s -> %s", capability, provider.Provider)
		if provider.Reference != "" {
			fmt.Fprintf(out, " -> %s", provider.Reference)
		}
		if provider.Scope != "" {
			fmt.Fprintf(out, " [%s]", provider.Scope)
		}
		fmt.Fprintf(out, " (%s)\n", provider.Source)
	}
	for _, policy := range effective.Policies {
		mode := "default"
		if policy.Mandatory {
			mode = "mandatory"
		}
		fmt.Fprintf(out, "Policy:       %s -> %s [%s] (%s)\n", policy.Policy, policy.Reference, mode, policy.Source)
	}
	return nil
}

func readOrganizationPreferences(args []string) ([]string, []orgconfig.PreferenceLayer, error) {
	remaining := make([]string, 0, len(args))
	var preferences []orgconfig.PreferenceLayer
	seen := false
	for i := 0; i < len(args); i++ {
		if args[i] != "--preferences" {
			remaining = append(remaining, args[i])
			continue
		}
		if seen || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
			return nil, nil, usageError("--preferences requires one file", "Provide one JSON array of lower-trust preference layers.")
		}
		seen = true
		i++
		file, err := os.Open(args[i])
		if err != nil {
			return nil, nil, fmt.Errorf("read organization preferences: %w", err)
		}
		decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&preferences)
		if err == nil {
			var extra any
			if trailing := decoder.Decode(&extra); trailing != io.EOF {
				err = fmt.Errorf("preferences require exactly one JSON array")
			}
		}
		_ = file.Close()
		if err != nil {
			return nil, nil, usageError("invalid preference file", "Use the versioned organization preference-layer JSON contract without unknown fields or trailing data.")
		}
	}
	return remaining, preferences, nil
}

func organizationResolutionIdentity(r orgconfig.Resolution) string {
	if value := strings.TrimSpace(r.ResolvedDigest); value != "" {
		if revision := strings.TrimSpace(r.ResolvedRevision); revision != "" {
			return value + " / " + revision
		}
		return value
	}
	if value := strings.TrimSpace(r.ResolvedRevision); value != "" {
		return value
	}
	return "<unresolved>"
}

func parseOrganizationSetArgs(args []string) (orgconfig.Source, string, cliOutputFormat, error) {
	var source orgconfig.Source
	environment := "dev"
	format := outputHuman
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source", "--location", "--requested", "--environment", "-o", "--output":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return source, "", "", usageError(args[i]+" requires a value", "Run 'baha config organization set --help' for usage.")
			}
			key, value := args[i], strings.TrimSpace(args[i+1])
			i++
			switch key {
			case "--source":
				source.Kind = orgconfig.SourceKind(strings.ToLower(value))
			case "--location":
				source.Location = value
			case "--requested":
				source.Requested = value
			case "--environment":
				environment = value
			case "-o", "--output":
				if value != "json" && value != "human" {
					return source, "", "", usageError("unsupported output format "+value, "Use human or json.")
				}
				format = cliOutputFormat(value)
			}
		case "--json":
			format = outputJSON
		default:
			return source, "", "", unknownOptionUsage("baha config organization set", args[i], "--source", "--location", "--requested", "--environment", "--json", "-o", "--output")
		}
	}
	if source.Kind == "" {
		return source, "", "", usageError("--source is required", "Use --source oci|git|local|system.")
	}
	if source.Kind != orgconfig.SourceSystem && strings.TrimSpace(source.Location) == "" {
		return source, "", "", usageError("--location is required for this organization source", "Provide an OCI repository, Git repository, local directory/file, or use --source system.")
	}
	return source, environment, format, nil
}

func parseOrganizationReadArgs(args []string, command string, requireApproval bool) (string, cliOutputFormat, error) {
	environment := "dev"
	format := outputHuman
	approved := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes":
			approved = true
		case "--environment", "-o", "--output":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", "", usageError(args[i]+" requires a value", "Run 'baha "+command+" --help' for usage.")
			}
			key, value := args[i], strings.TrimSpace(args[i+1])
			i++
			if key == "--environment" {
				environment = value
			} else {
				if value != "json" && value != "human" {
					return "", "", usageError("unsupported output format "+value, "Use human or json.")
				}
				format = cliOutputFormat(value)
			}
		case "--json":
			format = outputJSON
		default:
			return "", "", unknownOptionUsage("baha "+command, args[i], "--environment", "--yes", "--json", "-o", "--output")
		}
	}
	if requireApproval && !approved {
		return "", "", usageError("organization update requires explicit approval", "Review 'baha config organization check' and rerun with --yes.")
	}
	return environment, format, nil
}
