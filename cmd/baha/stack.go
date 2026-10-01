package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
	"go.yaml.in/yaml/v3"
)

func stackCommand() *cli.Command {
	return &cli.Command{
		Name:    "stack",
		Summary: "Discover and manage reusable development Stack Profiles",
		Usage:   "baha stack list|show|create",
		Children: []*cli.Command{
			stackListCommand(),
			stackShowCommand(),
			stackCreateCommand(),
		},
	}
}

func stackCatalog() (development.Registry, development.ProfileCatalogEntries, error) {
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		return development.Registry{}, nil, err
	}
	catalog, err := effectiveDevelopmentProfileCatalog(".", registry)
	if err != nil {
		return development.Registry{}, nil, err
	}
	return registry, catalog, nil
}

func stackListCommand() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Summary: "List built-in, user and repository Stack Profiles",
		Usage:   "baha stack list [-o json|--output json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			format, rest, err := parseStackOutput(args)
			if err != nil {
				return err
			}
			if len(rest) != 0 {
				return usageError("stack list does not accept positional arguments", "Use 'baha stack list'.")
			}
			_, catalog, err := stackCatalog()
			if err != nil {
				return err
			}
			names := sortedProfileNames(catalog)
			if format == outputJSON {
				entries := make([]development.ProfileEntry, 0, len(names))
				for _, name := range names {
					entries = append(entries, catalog[name])
				}
				return writeJSON(out, entries)
			}
			if len(names) == 0 {
				fmt.Fprintln(out, "No Stack Profiles found.")
				return nil
			}
			for _, name := range names {
				entry := catalog[name]
				fmt.Fprintf(out, "%-20s %-12s", name, entry.Scope)
				if entry.Path != "" {
					fmt.Fprintf(out, " %s", displayUserPath(entry.Path))
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}
}

func stackShowCommand() *cli.Command {
	return &cli.Command{
		Name:    "show",
		Summary: "Show a resolved Stack Profile",
		Usage:   "baha stack show NAME [-o json|--output json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			format, rest, err := parseStackOutput(args)
			if err != nil {
				return err
			}
			if len(rest) != 1 {
				return usageError("stack show requires exactly one NAME", "Use 'baha stack show NAME'.")
			}
			_, catalog, err := stackCatalog()
			if err != nil {
				return err
			}
			entry, ok := catalog[rest[0]]
			if !ok {
				return fmt.Errorf("stack profile %q was not found", rest[0])
			}
			resolved, err := development.ResolveStackProfile(rest[0], development.ProfileMap(catalog))
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, map[string]any{
					"scope":   entry.Scope,
					"path":    entry.Path,
					"sources": resolved.Sources,
					"profile": resolved.Profile,
				})
			}
			data, err := yaml.Marshal(resolved.Profile)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Scope: %s\n", entry.Scope)
			if entry.Path != "" {
				fmt.Fprintf(out, "Path: %s\n", displayUserPath(entry.Path))
			}
			fmt.Fprintln(out, "Resolved profile:")
			fmt.Fprint(out, string(data))
			return nil
		},
	}
}

func stackCreateCommand() *cli.Command {
	return &cli.Command{
		Name:    "create",
		Summary: "Create a reusable Stack Profile",
		Usage:   "baha stack create [NAME --component ID:ROLE:STACK ...] [--extends NAME] [--scope user|repository] [-o json|--output json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			registry, catalog, err := stackCatalog()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				if noInput(ctx) || !readerIsTerminal(appNewInput) {
					return usageError("interactive stack create requires a terminal", "Provide NAME and --component ID:ROLE:STACK for deterministic use.")
				}
				selection, err := guidedCreateStackProfile(bufio.NewReader(appNewInput), out, catalog, registry)
				if err != nil {
					return err
				}
				if selection.RawProfile == nil {
					return fmt.Errorf("stack creation did not produce a reusable profile")
				}
				path, err := development.SaveProfile(*selection.RawProfile, selection.SaveScope, ".")
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "Created stack %s (%s)\n", selection.RawProfile.Metadata.Name, selection.SaveScope)
				fmt.Fprintf(out, "Path: %s\n", displayUserPath(path))
				return nil
			}
			options, err := parseStackCreateOptions(args)
			if err != nil {
				return err
			}
			if _, exists := catalog[options.Profile.Metadata.Name]; exists {
				return fmt.Errorf("stack profile %q already exists", options.Profile.Metadata.Name)
			}
			temp := development.ProfileCatalog{}
			for name, entry := range catalog {
				temp[name] = entry.Profile
			}
			temp[options.Profile.Metadata.Name] = options.Profile
			if _, err := development.ResolveStackProfile(options.Profile.Metadata.Name, temp); err != nil {
				return err
			}
			path, err := development.SaveProfile(options.Profile, options.Scope, ".")
			if err != nil {
				return err
			}
			if options.Output == outputJSON {
				return writeJSON(out, map[string]any{
					"name":  options.Profile.Metadata.Name,
					"scope": options.Scope,
					"path":  path,
				})
			}
			fmt.Fprintf(out, "Created stack %s (%s)\n", options.Profile.Metadata.Name, options.Scope)
			fmt.Fprintf(out, "Path: %s\n", displayUserPath(path))
			return nil
		},
	}
}

type stackCreateOptions struct {
	Profile development.StackProfile
	Scope   development.ProfileScope
	Output  cliOutputFormat
}

func parseStackCreateOptions(args []string) (stackCreateOptions, error) {
	options := stackCreateOptions{
		Profile: development.StackProfile{
			APIVersion: development.StackProfileAPIVersion,
			Kind:       development.StackProfileKind,
		},
		Scope: development.ProfileScopeUser,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--extends", "--component", "--capability", "--scope", "--output", "-o":
			if i+1 >= len(args) {
				return stackCreateOptions{}, usageError(arg+" requires a value", "Run 'baha stack create --help' for usage.")
			}
			i++
			value := strings.TrimSpace(args[i])
			switch arg {
			case "--extends":
				options.Profile.Extends = append(options.Profile.Extends, value)
			case "--component":
				component, err := parseStackComponent(value)
				if err != nil {
					return stackCreateOptions{}, err
				}
				options.Profile.Components = append(options.Profile.Components, component)
			case "--capability":
				preference, err := parseStackCapabilityPreference(value)
				if err != nil {
					return stackCreateOptions{}, err
				}
				options.Profile.Capabilities = append(options.Profile.Capabilities, preference)
			case "--scope":
				switch value {
				case "user":
					options.Scope = development.ProfileScopeUser
				case "repository", "repo", "team":
					options.Scope = development.ProfileScopeRepository
				default:
					return stackCreateOptions{}, fmt.Errorf("unsupported stack scope %q", value)
				}
			case "--output", "-o":
				if value != "json" {
					return stackCreateOptions{}, usageError("unsupported output format "+value, "Use --output json.")
				}
				options.Output = outputJSON
			}
		default:
			if strings.HasPrefix(arg, "-") {
				return stackCreateOptions{}, usageError("unknown stack create option "+arg, "Run 'baha stack create --help' for usage.")
			}
			if options.Profile.Metadata.Name != "" {
				return stackCreateOptions{}, usageError("stack create accepts exactly one NAME", "Use 'baha stack create NAME ...'.")
			}
			options.Profile.Metadata.Name = arg
		}
	}
	if strings.TrimSpace(options.Profile.Metadata.Name) == "" {
		return stackCreateOptions{}, usageError("stack profile NAME is required", "Use 'baha stack create NAME ...'.")
	}
	if len(options.Profile.Components) == 0 && len(options.Profile.Extends) == 0 {
		return stackCreateOptions{}, usageError("stack profile needs --component or --extends", "Example: baha stack create team-api --component app:application:go")
	}
	return options, nil
}

func parseStackComponent(value string) (development.Component, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return development.Component{}, usageError("invalid --component "+value, "Use ID:ROLE:STACK, for example api:backend:go.")
	}
	id := slugifyAppName(parts[0])
	role := strings.TrimSpace(parts[1])
	adapter, err := developmentAdapterID(parts[2])
	if err != nil {
		return development.Component{}, err
	}
	if id == "" || role == "" {
		return development.Component{}, fmt.Errorf("component id and role are required")
	}
	return development.Component{ID: id, Role: role, Adapter: adapter}, nil
}

func parseStackOutput(args []string) (cliOutputFormat, []string, error) {
	format := outputHuman
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 >= len(args) || args[i+1] != "json" {
				return "", nil, usageError(args[i]+" requires json", "Use --output json.")
			}
			format = outputJSON
			i++
		default:
			rest = append(rest, args[i])
		}
	}
	return format, rest, nil
}

func parseStackCapabilityPreference(value string) (development.CapabilityPreference, error) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return development.CapabilityPreference{}, usageError("invalid --capability "+value, "Use KIND=COMPONENT[,COMPONENT], for example database.sql=api.")
	}
	kinds, err := developmentCapabilityKinds([]string{strings.TrimSpace(parts[0])})
	if err != nil {
		return development.CapabilityPreference{}, err
	}
	components := splitWizardItems(parts[1])
	if len(components) == 0 {
		return development.CapabilityPreference{}, fmt.Errorf("capability placement requires at least one component")
	}
	for i := range components {
		components[i] = slugifyAppName(components[i])
		if components[i] == "" {
			return development.CapabilityPreference{}, fmt.Errorf("invalid capability component")
		}
	}
	return development.CapabilityPreference{Capability: kinds[0], Components: components}, nil
}
