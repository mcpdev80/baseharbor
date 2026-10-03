package main

import (
	"context"
	"errors"
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
		Usage:   "baha app workspace init [--manifest PATH] --source ID=REPOSITORY [--source ...] [--oci ID=IMAGE] [--component COMPONENT=SOURCE[@SUBPATH]]...",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			_ = ctx
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
			manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
			if err != nil {
				return err
			}
			model := development.SourceModel{
				SchemaVersion: development.SourceModelVersion,
				Application:   manifest.Name,
				Sources:       sources,
				Components:    components,
			}
			path, err := development.WriteSourceModel(manifestPath, model)
			if err != nil {
				return err
			}
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
		Usage:   "baha app workspace map SOURCE PATH [--manifest PATH]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			_ = ctx
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
			manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
			if err != nil {
				return err
			}
			model, _, err := development.LoadSourceModel(manifestPath)
			sourceID := strings.TrimSpace(positional[0])
			bootstrap := false
			if err != nil {
				if !errors.Is(err, development.ErrWorkspaceModelMissing) {
					return err
				}
				checkout, absErr := filepath.Abs(strings.TrimSpace(positional[1]))
				if absErr != nil {
					return absErr
				}
				info, statErr := os.Stat(checkout)
				if statErr != nil || !info.IsDir() {
					return usageError("workspace source path is not an existing directory", "Map an existing Git checkout/worktree.")
				}
				repository := strings.TrimSpace(workspaceGitValue(ctx, checkout, "config", "--get", "remote.origin.url"))
				if repository == "" {
					return usageError("cannot bootstrap workspace source without a stable Git origin", "Configure remote.origin.url, or run 'baha app workspace init --source ID=REPOSITORY --component COMPONENT=ID' explicitly.")
				}
				components := application.WorkloadComponentNames(manifest)
				if len(components) != 1 {
					return usageError("cannot infer the first workspace component", "Run 'baha app workspace init --source ID=REPOSITORY --component COMPONENT=ID' explicitly for zero- or multi-component applications.")
				}
				model = development.SourceModel{
					SchemaVersion: development.SourceModelVersion,
					Application:   manifest.Name,
					Sources: []development.SourceDefinition{{
						ID:         sourceID,
						Type:       development.SourceRepository,
						Repository: repository,
						Ref:        strings.TrimSpace(workspaceGitValue(ctx, checkout, "branch", "--show-current")),
					}},
					Components: []development.ComponentSource{{
						Component: components[0],
						Source:    sourceID,
					}},
				}
				bootstrap = true
			}
			found := false
			for _, source := range model.Sources {
				if source.ID == sourceID {
					if source.Type != development.SourceRepository {
						return usageError("OCI sources do not have local workspace mappings", "Map only repository sources.")
					}
					found = true
					break
				}
			}
			if !found {
				return usageError("unknown source "+sourceID, "Run 'baha app workspace show' to inspect source identities.")
			}
			mapping, _, err := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
			if os.IsNotExist(err) {
				mapping = development.WorkspaceMapping{
					SchemaVersion: development.WorkspaceMappingVersion,
					Application:   manifest.Name,
					Manifest:      manifestPath,
					Sources:       map[string]string{},
				}
			} else if err != nil {
				return err
			}
			if mapping.Sources == nil {
				mapping.Sources = map[string]string{}
			}
			mapping.Sources[sourceID] = positional[1]
			var sourceModelPath string
			if bootstrap {
				sourceModelPath, err = development.WriteSourceModel(manifestPath, model)
				if err != nil {
					return err
				}
			}
			path, err := development.SaveWorkspaceMapping(manifestPath, mapping)
			if err != nil {
				if sourceModelPath != "" {
					_ = os.Remove(sourceModelPath)
				}
				return err
			}
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
			manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
			if err != nil {
				return err
			}
			model, sourcePath, err := development.LoadSourceModel(manifestPath)
			if err != nil {
				return err
			}
			mapping, mappingPath, mapErr := development.LoadWorkspaceMapping(manifestPath, manifest.Name)
			if os.IsNotExist(mapErr) {
				mapping = development.WorkspaceMapping{SchemaVersion: development.WorkspaceMappingVersion, Application: manifest.Name, Manifest: manifestPath, Sources: map[string]string{}}
				mappingPath, _ = development.WorkspaceMappingPath(manifestPath, manifest.Name)
			} else if mapErr != nil {
				return mapErr
			}
			result := struct {
				SourceModelPath string                       `json:"source_model_path"`
				WorkspacePath   string                       `json:"workspace_path"`
				Model           development.SourceModel      `json:"source_model"`
				Workspace       development.WorkspaceMapping `json:"workspace"`
			}{sourcePath, mappingPath, model, mapping}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "application: %s\nmanifest: %s\nsource model: %s\nworkspace: %s\n", manifest.Name, manifestPath, sourcePath, mappingPath)
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
