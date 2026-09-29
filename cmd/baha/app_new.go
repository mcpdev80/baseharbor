package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/development/goadapter"
)

type appNewOptions struct {
	Name         string
	Environment  string
	Stack        string
	Capabilities []capability.Kind
	Secrets      []string
	Output       cliOutputFormat
}

func appNewCommand() *cli.Command {
	return &cli.Command{
		Name:    "new",
		Summary: "Create a new ecosystem-native application from a BaseHarbor contract",
		Usage:   "baha app new [NAME] [--stack go] [-e ENV|--environment ENV] [--http] [--sql] [--cache] [--s3] [--secrets] [--require-secret NAME]... [--telemetry] [--all] [-o json|--output json]",
		Long:    "Creates a normal ecosystem-native source repository plus baseharbor.yaml. Development integration is authoring-time only: generated applications use standard ecosystem libraries and do not depend on a BaseHarbor application framework.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			options, err := parseAppNewOptions(args)
			if err != nil {
				return err
			}
			if options.Name == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				options.Name = filepath.Base(cwd)
			}
			adapterID, err := developmentAdapterID(options.Stack)
			if err != nil {
				return err
			}
			registry, err := development.NewRegistry(goadapter.Adapter{})
			if err != nil {
				return err
			}
			result, err := development.CreateApplication(".", development.NewApplicationRequest{
				Name:         options.Name,
				Environment:  options.Environment,
				Adapter:      adapterID,
				Capabilities: options.Capabilities,
				Secrets:      options.Secrets,
			}, registry)
			if err != nil {
				return err
			}

			if options.Output == outputJSON {
				return writeJSON(out, struct {
					Application     string                      `json:"application"`
					Environment     string                      `json:"environment"`
					Profile         development.StackProfile    `json:"profile"`
					DevelopmentPlan development.DevelopmentPlan `json:"development_plan"`
					Files           []string                    `json:"files"`
					Satisfied       bool                        `json:"satisfied"`
				}{
					Application:     result.Manifest.Name,
					Environment:     result.Manifest.Environment,
					Profile:         result.Profile,
					DevelopmentPlan: result.Plan,
					Files:           result.FilePaths,
					Satisfied:       result.Validation.Satisfied,
				})
			}
			fmt.Fprintf(out, "created %s (%s) with %s\n", result.Manifest.Name, result.Manifest.Environment, adapterID)
			for _, path := range result.FilePaths {
				fmt.Fprintf(out, "  %s\n", path)
			}
			fmt.Fprintln(out, "validation: SATISFIED")
			fmt.Fprintln(out, "next: review the generated source, then run 'baha up'")
			return nil
		},
	}
}

func parseAppNewOptions(args []string) (appNewOptions, error) {
	options := appNewOptions{Environment: "dev", Stack: "go", Output: outputHuman}
	seenCapability := map[capability.Kind]bool{}
	addCapability := func(kind capability.Kind) {
		if !seenCapability[kind] {
			seenCapability[kind] = true
			options.Capabilities = append(options.Capabilities, kind)
		}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--http":
			addCapability(capability.ExposureHTTP)
		case "--sql":
			addCapability(capability.SQL)
		case "--cache":
			addCapability(capability.KeyValue)
		case "--s3":
			addCapability(capability.ObjectStorageS3)
		case "--secrets":
			addCapability(capability.Secrets)
		case "--telemetry":
			addCapability(capability.TelemetryOTLP)
		case "--all":
			for _, kind := range []capability.Kind{capability.ExposureHTTP, capability.SQL, capability.KeyValue, capability.ObjectStorageS3, capability.Secrets, capability.TelemetryOTLP} {
				addCapability(kind)
			}
		case "--environment", "-e", "--stack", "--require-secret", "--output", "-o":
			if i+1 >= len(args) {
				return appNewOptions{}, usageError(arg+" requires a value", "Run 'baha app new --help' for usage.")
			}
			i++
			value := strings.TrimSpace(args[i])
			switch arg {
			case "--environment", "-e":
				options.Environment = value
			case "--stack":
				options.Stack = value
			case "--require-secret":
				if value == "" {
					return appNewOptions{}, usageError("--require-secret requires a non-empty name", "Pass the environment variable name expected by the application.")
				}
				options.Secrets = append(options.Secrets, value)
				addCapability(capability.Secrets)
			case "--output", "-o":
				if value != "json" {
					return appNewOptions{}, usageError("unsupported output format "+value, "Use --output json.")
				}
				options.Output = outputJSON
			}
		default:
			if strings.HasPrefix(arg, "-") {
				return appNewOptions{}, usageError("unknown app new option "+arg, "Run 'baha app new --help' for usage.")
			}
			if options.Name != "" {
				return appNewOptions{}, usageError("baha app new accepts at most one application name", "Run 'baha app new --help' for usage.")
			}
			options.Name = arg
		}
	}
	if len(options.Capabilities) == 0 {
		addCapability(capability.ExposureHTTP)
	}
	return options, nil
}

func developmentAdapterID(stack string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(stack)) {
	case "", "go", goadapter.AdapterID:
		return goadapter.AdapterID, nil
	default:
		return "", usageError("development stack "+stack+" is not available yet", "Use --stack go.")
	}
}
