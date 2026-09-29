package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
)

var appNewInput io.Reader = os.Stdin

type appNewOptions struct {
	Name               string
	Directory          string
	Environment        string
	Stack              string
	Capabilities       []capability.Kind
	Secrets            []string
	Output             cliOutputFormat
	EmitBackstage      bool
	BackstageOwner     string
	BackstageLifecycle string
}

func appNewCommand() *cli.Command {
	return &cli.Command{
		Name:    "new",
		Summary: "Create a new ecosystem-native application from a BaseHarbor contract",
		Usage:   "baha app new [NAME] [--directory PARENT] [--stack go|nextjs|python|quarkus | --stack-profile NAME] [--emit-backstage --backstage-owner OWNER [--backstage-lifecycle LIFECYCLE]] [-e ENV|--environment ENV] [--http] [--sql] [--cache] [--s3] [--secrets] [--require-secret NAME]... [--telemetry] [--all] [-o json|--output json]",
		Long:    "Creates a normal ecosystem-native source repository plus baseharbor.yaml. Development integration is authoring-time only: generated applications use standard ecosystem libraries and do not depend on a BaseHarbor application framework.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) == 0 {
				if noInput(ctx) || !readerIsTerminal(appNewInput) {
					return usageError("interactive app new requires a terminal", "Provide NAME and --directory <parent> with deterministic flags for CI/non-TTY use.")
				}
				return runAppNewWizard(ctx, out, errOut)
			}
			options, err := parseAppNewOptions(args)
			if err != nil {
				return err
			}
			if strings.TrimSpace(options.Name) == "" {
				return usageError("application NAME is required for deterministic app new", "Use 'baha app new' interactively or pass a name explicitly.")
			}
			root, err := resolveNewApplicationRoot(options.Name, options.Directory)
			if err != nil {
				return err
			}
			registry, err := referenceDevelopmentRegistry()
			if err != nil {
				return err
			}
			var adapterID string
			var profile *development.StackProfile
			if strings.TrimSpace(options.StackProfile) != "" {
				if options.StackExplicit {
					return usageError("--stack and --stack-profile cannot be combined", "Select either a built-in adapter stack or one reusable Stack Profile.")
				}
				catalog, err := development.LoadProfileCatalog(".", builtinDevelopmentProfiles(registry))
				if err != nil {
					return err
				}
				resolved, err := development.ResolveStackProfile(options.StackProfile, development.ProfileMap(catalog))
				if err != nil {
					return err
				}
				profile = &resolved.Profile
			} else {
				adapterID, err = developmentAdapterID(options.Stack)
				if err != nil {
					return err
				}
			}
			result, err := development.CreateApplication(root, development.NewApplicationRequest{
				Name:               options.Name,
				Environment:        options.Environment,
				Adapter:            adapterID,
				Profile:            profile,
				Capabilities:       options.Capabilities,
				Secrets:            options.Secrets,
				EmitBackstage:      options.EmitBackstage,
				BackstageOwner:     options.BackstageOwner,
				BackstageLifecycle: options.BackstageLifecycle,
			}, registry)
			if err != nil {
				return err
			}

			if options.Output == outputJSON {
				return writeJSON(out, struct {
					ContractVersion string                      `json:"contract_version"`
					Application     string                      `json:"application"`
					Environment     string                      `json:"environment"`
					Profile         development.StackProfile    `json:"profile"`
					DevelopmentPlan development.DevelopmentPlan `json:"development_plan"`
					Files           []string                    `json:"files"`
					Satisfied       bool                        `json:"satisfied"`
				}{
					ContractVersion: development.NewApplicationResultVersion,
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
		case "--emit-backstage":
			options.EmitBackstage = true
		case "--all":
			for _, kind := range []capability.Kind{capability.ExposureHTTP, capability.SQL, capability.KeyValue, capability.ObjectStorageS3, capability.Secrets, capability.TelemetryOTLP} {
				addCapability(kind)
			}
		case "--environment", "-e", "--directory", "--stack", "--stack-profile", "--require-secret", "--backstage-owner", "--backstage-lifecycle", "--output", "-o":
			if i+1 >= len(args) {
				return appNewOptions{}, usageError(arg+" requires a value", "Run 'baha app new --help' for usage.")
			}
			i++
			value := strings.TrimSpace(args[i])
			switch arg {
			case "--environment", "-e":
				options.Environment = value
			case "--directory":
				options.Directory = value
			case "--stack":
				options.Stack = value
				options.StackExplicit = true
			case "--stack-profile":
				options.StackProfile = value
			case "--backstage-owner":
				options.BackstageOwner = value
				options.EmitBackstage = true
			case "--backstage-lifecycle":
				options.BackstageLifecycle = value
				options.EmitBackstage = true
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
	if options.EmitBackstage && strings.TrimSpace(options.BackstageOwner) == "" {
		return appNewOptions{}, usageError("--emit-backstage requires --backstage-owner", "Provide the Backstage owner explicitly; BaseHarbor never infers portal ownership.")
	}
	return options, nil
}

func developmentCapabilityKinds(values []string) ([]capability.Kind, error) {
	if len(values) == 0 {
		return []capability.Kind{capability.ExposureHTTP}, nil
	}
	seen := map[capability.Kind]struct{}{}
	result := make([]capability.Kind, 0, len(values))
	for _, value := range values {
		var kind capability.Kind
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "http", "exposure.http":
			kind = capability.ExposureHTTP
		case "sql", "database.sql":
			kind = capability.SQL
		case "cache", "cache.key-value":
			kind = capability.KeyValue
		case "s3", "object-storage.s3":
			kind = capability.ObjectStorageS3
		case "secrets":
			kind = capability.Secrets
		case "telemetry", "otlp", "telemetry.otlp":
			kind = capability.TelemetryOTLP
		default:
			return nil, usageError("unsupported greenfield capability "+value, "Use exposure.http, database.sql, cache.key-value, object-storage.s3, secrets or telemetry.otlp.")
		}
		if _, exists := seen[kind]; exists {
			continue
		}
		seen[kind] = struct{}{}
		result = append(result, kind)
	}
	return result, nil
}

func developmentAdapterID(stack string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(stack))
	if name == "" {
		name = "go"
	}
	if name == "next.js" {
		name = "nextjs"
	}
	id := name
	if !strings.HasPrefix(id, "development/") {
		id = "development/" + id
	}
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		return "", err
	}
	if _, err := registry.Resolve(id); err != nil {
		return "", usageError("development stack "+stack+" is not available", "Use 'baha stack list' to see registered stacks and profiles.")
	}
	return id, nil
}
