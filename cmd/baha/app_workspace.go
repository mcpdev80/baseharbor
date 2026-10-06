package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func appWorkspaceCommand() *cli.Command {
	return &cli.Command{
		Name:    "workspace",
		Summary: "Manage local multi-repository workspace mappings",
		Usage:   "baha app workspace [<init|map|show|resolve|status|update> [options]]",
		Long:    "With no subcommand, opens the guided multi-repository workspace setup. Portable source identity is kept in .baseharbor/sources.yaml while developer-local checkout paths stay in XDG configuration and never become Application Intent.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha app workspace does not accept positional arguments", "Run 'baha app workspace' for guided setup or choose init, map, show, resolve, status or update.")
			}
			if noInput(ctx) || !readerIsTerminal(appWorkspaceInput) {
				return usageError("interactive app workspace requires a terminal", "Use 'baha app workspace init' and 'baha app workspace map' for deterministic non-interactive setup.")
			}
			return runAppWorkspaceWizard(ctx, out)
		},
		Children: []*cli.Command{
			appWorkspaceInitCommand(),
			appWorkspaceMapCommand(),
			appWorkspaceShowCommand(),
			appWorkspaceResolveCommand(),
			appWorkspaceStatusCommand(),
			appWorkspaceUpdateCommand(),
		},
	}
}

func resolveWorkspaceManifest(path string) (string, application.Manifest, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		manifest, err := application.LoadManifestFile(path)
		if err != nil {
			return "", application.Manifest{}, err
		}
		abs, err := filepath.Abs(path)
		return abs, manifest, err
	}
	manifestPath, err := application.FindRepositoryManifest(path)
	if err != nil {
		return "", application.Manifest{}, err
	}
	manifest, err := application.LoadManifestFile(manifestPath)
	return manifestPath, manifest, err
}

func appWorkspaceInitCommand() *cli.Command {
	return &cli.Command{
		Name:    "init",
		Summary: "Create versioned source identity metadata for a multi-repository application",
		Usage:   "baha app workspace init [--manifest PATH] --source ID=REPOSITORY [--source ...] [--oci ID=IMAGE] [--component COMPONENT=SOURCE[@SUBPATH]]... [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "workspace init")
			if err != nil {
				return err
			}
			args = filtered
			manifestArg := "."
			var sources []development.SourceDefinition
			var components []development.ComponentSource
			for i := 0; i < len(args); i++ {
				arg := args[i]
				switch arg {
				case "--manifest", "--source", "--oci", "--component":
					if i+1 >= len(args) {
						return usageError(arg+" requires a value", "Run 'baha app workspace init --help'.")
					}
					i++
					value := strings.TrimSpace(args[i])
					switch arg {
					case "--manifest":
						manifestArg = value
					case "--source":
						id, identity, ok := strings.Cut(value, "=")
						if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(identity) == "" {
							return usageError("--source requires ID=REPOSITORY", "Example: --source api=https://github.com/acme/api.git")
						}
						sources = append(sources, development.SourceDefinition{ID: strings.TrimSpace(id), Type: development.SourceRepository, Repository: strings.TrimSpace(identity)})
					case "--oci":
						id, image, ok := strings.Cut(value, "=")
						if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(image) == "" {
							return usageError("--oci requires ID=IMAGE", "Example: --oci worker=ghcr.io/acme/worker@sha256:...")
						}
						sources = append(sources, development.SourceDefinition{ID: strings.TrimSpace(id), Type: development.SourceOCI, Image: strings.TrimSpace(image)})
					case "--component":
						component, mapping, ok := strings.Cut(value, "=")
						if !ok || strings.TrimSpace(component) == "" || strings.TrimSpace(mapping) == "" {
							return usageError("--component requires COMPONENT=SOURCE[@SUBPATH]", "Example: --component api=backend@services/api")
						}
						source, subPath, hasSubPath := strings.Cut(mapping, "@")
						entry := development.ComponentSource{Component: strings.TrimSpace(component), Source: strings.TrimSpace(source)}
						if hasSubPath {
							entry.SubPath = strings.TrimSpace(subPath)
						}
						components = append(components, entry)
					}
				default:
					return unknownOptionUsage("baha app workspace init", arg, "--manifest", "--source", "--oci", "--component")
				}
			}
			result, err := initializeApplicationWorkspace(ctx, machineWorkspaceInitInput{Manifest: manifestArg, Sources: sources, Components: components})
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			path := result.SourceModelPath
			fmt.Fprintf(out, "source model: %s\n", path)
			fmt.Fprintln(out, "local checkout paths remain separate; map repository sources with 'baha app workspace map SOURCE PATH'")
			return nil
		},
	}
}

func appWorkspaceMapCommand() *cli.Command {
	return &cli.Command{
		Name:    "map",
		Summary: "Map one repository source identity to an existing local checkout/worktree",
		Usage:   "baha app workspace map SOURCE PATH [--manifest PATH] [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "workspace map")
			if err != nil {
				return err
			}
			args = filtered
			manifestArg := "."
			var positional []string
			for i := 0; i < len(args); i++ {
				if args[i] == "--manifest" {
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace map --help'.")
					}
					i++
					manifestArg = args[i]
					continue
				}
				if strings.HasPrefix(args[i], "-") {
					return unknownOptionUsage("baha app workspace map", args[i], "--manifest")
				}
				positional = append(positional, args[i])
			}
			if len(positional) != 2 {
				return usageError("workspace map requires SOURCE and PATH", "Example: baha app workspace map api-source ~/dev/api")
			}
			result, err := mapApplicationWorkspace(ctx, manifestArg, positional[0], positional[1])
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			path, sourceModelPath := result.MappingPath, result.SourceModelPath
			sourceID := strings.TrimSpace(positional[0])
			if sourceModelPath != "" {
				fmt.Fprintf(out, "source model: %s\n", sourceModelPath)
			}
			fmt.Fprintf(out, "mapped %s -> %s\n", sourceID, displayUserPath(positional[1]))
			fmt.Fprintf(out, "workspace state: %s\n", path)
			return nil
		},
	}
}

func appWorkspaceShowCommand() *cli.Command {
	return &cli.Command{
		Name:    "show",
		Summary: "Show canonical source identities and local workspace mappings",
		Usage:   "baha app workspace show [--manifest PATH] [-o json|--output json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			_ = ctx
			manifestArg := "."
			format := outputHuman
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--manifest":
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace show --help'.")
					}
					i++
					manifestArg = args[i]
				case "-o", "--output":
					if i+1 >= len(args) || args[i+1] != "json" {
						return usageError("--output supports only json", "Use --output json.")
					}
					i++
					format = outputJSON
				default:
					return unknownOptionUsage("baha app workspace show", args[i], "--manifest", "--output")
				}
			}
			result, err := inspectApplicationWorkspace(ctx, manifestArg)
			if err != nil {
				return err
			}
			manifestPath := result.Workspace.Manifest
			sourcePath, mappingPath, model, mapping := result.SourceModelPath, result.WorkspacePath, result.Model, result.Workspace
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "application: %s\nmanifest: %s\nsource model: %s\nworkspace: %s\n", result.Workspace.Application, manifestPath, sourcePath, mappingPath)
			for _, source := range model.Sources {
				local := mapping.Sources[source.ID]
				if source.Type == development.SourceOCI {
					fmt.Fprintf(out, "  %s  oci  %s\n", source.ID, source.Image)
				} else if local != "" {
					fmt.Fprintf(out, "  %s  repository  %s  -> %s\n", source.ID, source.Repository, local)
				} else {
					fmt.Fprintf(out, "  %s  repository  %s  -> UNMAPPED\n", source.ID, source.Repository)
				}
			}
			return nil
		},
	}
}

func appWorkspaceResolveCommand() *cli.Command {
	return &cli.Command{
		Name:    "resolve",
		Summary: "Resolve component source identity to local worktrees without using the current working directory",
		Usage:   "baha app workspace resolve [--manifest PATH] [-o json|--output json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			_ = ctx
			manifestArg := "."
			format := outputHuman
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--manifest":
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace resolve --help'.")
					}
					i++
					manifestArg = args[i]
				case "-o", "--output":
					if i+1 >= len(args) || args[i+1] != "json" {
						return usageError("--output supports only json", "Use --output json.")
					}
					i++
					format = outputJSON
				default:
					return unknownOptionUsage("baha app workspace resolve", args[i], "--manifest", "--output")
				}
			}
			manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
			if err != nil {
				return err
			}
			model, _, err := development.LoadSourceModel(manifestPath)
			if err != nil {
				return err
			}
			mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("workspace mapping is missing; map each repository source before resolution")
				}
				return err
			}
			resolved, err := development.ResolveWorkspace(manifestPath, model, mapping)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, resolved)
			}
			fmt.Fprintf(out, "Application: %s\nCanonical manifest: %s\n", resolved.Application, resolved.Manifest)
			for _, component := range resolved.Components {
				if component.Type == development.SourceOCI {
					fmt.Fprintf(out, "  %s -> OCI %s\n", component.Component, component.Image)
				} else {
					fmt.Fprintf(out, "  %s -> %s (%s)\n", component.Component, component.Root, component.Identity)
				}
			}
			return nil
		},
	}
}

type workspaceInspectionResult struct {
	SourceModelPath string                       `json:"source_model_path"`
	WorkspacePath   string                       `json:"workspace_path"`
	Model           development.SourceModel      `json:"source_model"`
	Workspace       development.WorkspaceMapping `json:"workspace"`
}

func inspectApplicationWorkspace(ctx context.Context, manifestArg string) (workspaceInspectionResult, error) {
	manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
	if err != nil {
		return workspaceInspectionResult{}, err
	}
	if err := authorizeMCPOperation(ctx, "workspace.show", "", manifest.Environment, manifest.ApplicationID, manifestPath); err != nil {
		return workspaceInspectionResult{}, err
	}
	model, sourcePath, err := development.LoadSourceModel(manifestPath)
	if err != nil {
		return workspaceInspectionResult{}, err
	}
	mapping, mappingPath, mapErr := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
	if os.IsNotExist(mapErr) {
		mapping = development.WorkspaceMapping{SchemaVersion: development.WorkspaceMappingVersion, Application: manifest.Name, Manifest: manifestPath, Sources: map[string]string{}}
		mappingPath, _ = development.WorkspaceMappingPath(manifestPath, manifest.Name)
	} else if mapErr != nil {
		return workspaceInspectionResult{}, mapErr
	}
	return workspaceInspectionResult{SourceModelPath: sourcePath, WorkspacePath: mappingPath, Model: model, Workspace: mapping}, nil
}
