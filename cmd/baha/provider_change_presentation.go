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
	fmt.Fprintf(out, "\nInstallation on %s\n", plan.Target)
	seen := map[string]bool{}
	line := func(text string) {
		if !seen[text] {
			fmt.Fprintln(out, "  "+text)
			seen[text] = true
		}
	}
	for _, impact := range plan.Impacts {
		action := providerActionLabel(impact.Operation)
		if len(impact.Functions) == 0 {
			line(action + " " + impact.InstanceID)
		}
		for _, function := range impact.Functions {
			name := providerFunctionLabel(function.Function)
			line(action + " " + name)
			if function.Outcome == "unavailable" || function.Outcome == "unknown" {
				line("Warning: " + name + " will be " + function.Outcome + " for " + function.Consumer)
			}
			if function.ValidUntil != "" {
				line(name + " remains valid until " + function.ValidUntil)
			}
		}
		for _, material := range impact.Materials {
			if material.After.Disposition != "retained" {
				line("Data: " + material.Before.ID + " -> " + material.After.Disposition)
				if material.Meaning != "" {
					line(material.Meaning)
				}
				if material.After.Destination != nil {
					line("Destination: " + material.After.Destination.Reference)
				}
			}
		}
		for _, effect := range impact.Effects {
			line(effect.Aspect + ": " + effect.Before + " -> " + effect.After)
		}
		for _, prerequisite := range impact.Prerequisites {
			if !prerequisite.Supported || !prerequisite.Verified {
				line("Warning: " + prerequisite.Subject + ": " + prerequisite.Consequence)
			}
		}
	}
	if plan.Destructive {
		fmt.Fprintln(out, "\nWarning: the listed data will be permanently deleted.")
	} else {
		fmt.Fprintln(out, "\nNo data deletion is planned.")
	}
	if !plan.Complete {
		fmt.Fprintln(out, "Some existing resources could not be fully checked.")
	}
	fmt.Fprintln(out, "Details: --verbose")
	return nil
}

func providerActionLabel(operation string) string {
	switch operation {
	case "add":
		return "Set up"
	case "update", "rebind":
		return "Update"
	case "resume":
		return "Start"
	case "stop":
		return "Stop"
	case "restart":
		return "Restart"
	case "replace":
		return "Replace"
	case "destroy", "remove-runtime":
		return "Remove"
	case "unbind", "disable":
		return "Disconnect"
	default:
		return strings.ToUpper(operation)
	}
}

func providerFunctionLabel(function string) string {
	switch function {
	case "database.sql":
		return "SQL"
	case "cache.key-value":
		return "Cache"
	case "store.key-value", "database.key-value":
		return "Persistent key-value storage"
	case "object-storage.s3":
		return "Object storage"
	case "database.document":
		return "Document database"
	case "messaging.queue", "messaging.pubsub", "messaging.stream":
		return "Messaging"
	case "identity.oidc":
		return "Identity / login"
	case "secrets", "secrets.kv":
		return "Secrets"
	case "exposure.http":
		return "HTTP access"
	case "telemetry.otlp":
		return "Telemetry"
	case "metrics":
		return "Metrics"
	case "logs":
		return "Logs"
	default:
		return function
	}
}

// A resumed deployment can need source consent without another provider-change
// challenge. Show requested services without claiming they are already running.
func printApplicationServiceSummary(out io.Writer, target string, contract application.PortableContract) {
	fmt.Fprintf(out, "\nApplication deployment on %s\nServices requested by %s:\n", target, contract.Application)
	seen := map[string]bool{}
	for _, requirement := range contract.Capabilities {
		label := providerFunctionLabel(string(requirement.Kind))
		if !seen[label] {
			fmt.Fprintln(out, "  "+label)
			seen[label] = true
		}
	}
	if contract.Secrets.Managed {
		fmt.Fprintln(out, "  Secrets")
	}
}
