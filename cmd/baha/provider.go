package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	providerauthoring "github.com/mcpdev80/baseharbor/internal/provider/authoring"
)

func providerCommand() *cli.Command {
	return &cli.Command{
		Name:    "provider",
		Summary: "Author and validate Capability Provider extensions",
		Usage:   "baha provider <command> [options]",
		Long:    "Provider authoring reuses the same baseharbor.provider/v1 descriptor and capability conformance model used by BaseHarbor Core.",
		Children: []*cli.Command{
			providerInitCommand(),
			providerTestCommand(),
		},
	}
}

func providerInitCommand() *cli.Command {
	return &cli.Command{
		Name:    "init",
		Summary: "Create a minimal Capability Provider authoring skeleton",
		Usage:   "baha provider init ID [--path DIR] [-o json|--output json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			id, root, format, err := parseProviderInitArgs(args)
			if err != nil {
				return err
			}
			result, err := providerauthoring.Init(root, id)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Created provider scaffold: %s\n", result.Root)
			for _, file := range result.Files {
				fmt.Fprintf(out, "  %s\n", file)
			}
			fmt.Fprintln(out, "Next: baha provider test "+result.Root)
			return nil
		},
	}
}

func providerTestCommand() *cli.Command {
	return &cli.Command{
		Name:    "test",
		Summary: "Run provider contract conformance checks",
		Usage:   "baha provider test [PATH] [-o json|--output json]",
		Long:    "Loads provider.yaml and runs the existing transport-independent provider contract conformance checks. Runtime lifecycle conformance extends the same report model.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "provider test")
			if err != nil {
				return err
			}
			if len(filtered) > 1 {
				return usageError("baha provider test accepts at most one PATH", "Example: baha provider test .")
			}
			root := "."
			if len(filtered) == 1 {
				root = filtered[0]
			}
			descriptor, err := providerauthoring.Load(root)
			if err != nil {
				return err
			}
			report := providerauthoring.Check(descriptor)
			if format == outputJSON {
				if err := writeJSON(out, report); err != nil {
					return err
				}
			} else {
				printProviderConformance(out, report)
			}
			if report.Status != capability.ConformancePass {
				return fmt.Errorf("provider conformance failed")
			}
			return nil
		},
	}
}

func parseProviderInitArgs(args []string) (string, string, cliOutputFormat, error) {
	var id, root string
	format := outputHuman
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			format = outputJSON
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) || args[i+1] != "json" {
				return "", "", "", usageError(arg+" requires json", "Use -o json.")
			}
			i++
			format = outputJSON
		case arg == "--path":
			if i+1 >= len(args) {
				return "", "", "", usageError("--path requires DIR", "Example: --path ./my-provider")
			}
			i++
			root = args[i]
		case strings.HasPrefix(arg, "--path="):
			root = strings.TrimPrefix(arg, "--path=")
		case strings.HasPrefix(arg, "-"):
			return "", "", "", usageError("unknown provider init option "+arg, "Run 'baha provider init --help' for usage.")
		default:
			if id != "" {
				return "", "", "", usageError("baha provider init accepts one provider ID", "Example: baha provider init example/postgresql")
			}
			id = arg
		}
	}
	if strings.TrimSpace(id) == "" {
		return "", "", "", usageError("provider ID is required", "Use namespace/name, for example example/postgresql.")
	}
	if strings.TrimSpace(root) == "" {
		parts := strings.Split(id, "/")
		root = parts[len(parts)-1]
	}
	return id, root, format, nil
}

func printProviderConformance(out io.Writer, report capability.ConformanceReport) {
	fmt.Fprintf(out, "Provider: %s\n", report.ProviderID)
	fmt.Fprintf(out, "Protocol: %s\n", report.Protocol)
	fmt.Fprintf(out, "Status: %s\n", strings.ToUpper(string(report.Status)))
	for _, check := range report.Checks {
		fmt.Fprintf(out, "  %-28s %s", check.Name, strings.ToUpper(string(check.Status)))
		if check.Message != "" {
			fmt.Fprintf(out, " - %s", check.Message)
		}
		fmt.Fprintln(out)
	}
}
