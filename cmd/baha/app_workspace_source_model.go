package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Incremental source-model maintenance (card 0428-059). Sources and component
// mappings are optional unique identities: this surface adds, retargets,
// re-paths and removes them one at a time without recreating the whole model,
// without inventing components, and without deleting local files.

type sourceMutationResult struct {
	SchemaVersion      string                `json:"schema_version"`
	SourceModelPath    string                `json:"source_model_path,omitempty"`
	MappingPath        string                `json:"mapping_path,omitempty"`
	Application        string                `json:"application"`
	Source             string                `json:"source,omitempty"`
	Component          string                `json:"component,omitempty"`
	ComponentSourceRef string                `json:"component_source,omitempty"`
	SubPath            string                `json:"sub_path,omitempty"`
	Type               string                `json:"type,omitempty"`
	Identity           string                `json:"identity,omitempty"`
	Removed            bool                  `json:"removed,omitempty"`
	RemovedMapping     bool                  `json:"removed_mapping,omitempty"`
	DetachedComponents []string              `json:"detached_components,omitempty"`
	Components         []sourceComponentView `json:"components,omitempty"`
	SourceCount        int                   `json:"source_count"`
	ComponentCount     int                   `json:"component_count"`
}

// loadWorkspaceSourceModel reads the workspace model. Callers authorize the
// operation they are about to perform separately so reads cannot grant write
// access to the returned mutable model.
func loadWorkspaceSourceModel(ctx context.Context, manifestArg string) (string, sourceModelContext, error) {
	manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
	if err != nil {
		return "", sourceModelContext{}, err
	}
	model, modelPath, err := development.LoadSourceModel(manifestPath)
	if err != nil {
		return "", sourceModelContext{}, err
	}
	return manifestPath, sourceModelContext{Manifest: manifest, Model: model, ModelPath: modelPath}, nil
}

func authorizeWorkspaceSourceModel(ctx context.Context, operation string, manifestPath string, manifest application.Manifest) error {
	return authorizeMCPOperation(ctx, operation, "", manifest.Environment, manifest.ApplicationID, manifestPath)
}

type sourceModelContext struct {
	Manifest  application.Manifest
	Model     development.SourceModel
	ModelPath string
}

func sourceDefinitionFromFlags(sourceKind, value, option string) (development.SourceDefinition, error) {
	id, identity, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(identity) == "" {
		return development.SourceDefinition{}, usageError(option+" requires ID=VALUE", "Run 'baha app workspace "+sourceKind+" --help'.")
	}
	id = strings.TrimSpace(id)
	identity = strings.TrimSpace(identity)
	switch option {
	case "--local":
		resolved, err := filepath.Abs(identity)
		if err != nil {
			return development.SourceDefinition{}, err
		}
		return development.SourceDefinition{ID: id, Type: development.SourceLocal, Path: resolved}, nil
	case "--oci":
		return development.SourceDefinition{ID: id, Type: development.SourceOCI, Image: identity}, nil
	default:
		return development.SourceDefinition{ID: id, Type: development.SourceRepository, Repository: identity}, nil
	}
}

func sourceDefinitionIdentity(source development.SourceDefinition) string {
	switch source.Type {
	case development.SourceLocal:
		return source.Path
	case development.SourceOCI:
		return source.Image
	default:
		if source.Ref != "" {
			return source.Repository + "@" + source.Ref
		}
		return source.Repository
	}
}

func sourceMutationLines(result sourceMutationResult, source development.SourceDefinition) []string {
	lines := []string{
		fmt.Sprintf("source %s: %s %s", result.Source, result.Type, sourceDefinitionIdentity(source)),
		fmt.Sprintf("source model: %s", result.SourceModelPath),
		fmt.Sprintf("sources: %d components: %d", result.SourceCount, result.ComponentCount),
	}
	if len(result.DetachedComponents) > 0 {
		lines = append(lines, "detached components: "+strings.Join(result.DetachedComponents, ", "))
	}
	return lines
}

func appWorkspaceSourceAddCommand() *cli.Command {
	return &cli.Command{
		Name:    "add",
		Summary: "Add or update one source identity without recreating the source model",
		Usage:   "baha app workspace source add [--manifest PATH] [--source ID=REPOSITORY] [--local ID=PATH] [--oci ID=IMAGE] [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg := "."
			format := outputHuman
			var definitions []development.SourceDefinition
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--manifest":
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace source add --help'.")
					}
					i++
					manifestArg = args[i]
				case "-o", "--output":
					if i+1 >= len(args) {
						return usageError(args[i]+" requires a value", "Use --output json.")
					}
					i++
					if args[i] != "json" {
						return usageError("--output supports only json", "Use --output json.")
					}
					format = outputJSON
				case "--source", "--local", "--oci":
					option := args[i]
					if i+1 >= len(args) {
						return usageError(option+" requires a value", "Run 'baha app workspace source add --help'.")
					}
					i++
					definition, err := sourceDefinitionFromFlags("source add", args[i], option)
					if err != nil {
						return err
					}
					definitions = append(definitions, definition)
				default:
					return unknownOptionUsage("baha app workspace source add", args[i], "--manifest", "--source", "--local", "--oci", "--output")
				}
			}
			if len(definitions) == 0 {
				return usageError("source add requires at least one --source, --local or --oci mapping", "Example: baha app workspace source add --local legacy=/home/dev/legacy-app")
			}
			manifestPath, state, err := loadWorkspaceSourceModel(ctx, manifestArg)
			if err != nil {
				return err
			}
			if err := authorizeWorkspaceSourceModel(ctx, "workspace.init", manifestPath, state.Manifest); err != nil {
				return err
			}
			model := state.Model
			for _, definition := range definitions {
				if err := development.UpsertSourceDefinition(&model, definition); err != nil {
					return err
				}
			}
			// Upsert validates the source shape; WriteSourceModel re-validates
			// the full contract for persisted models.
			path, err := development.WriteSourceModel(manifestPath, model)
			if err != nil {
				return err
			}
			lastID := strings.TrimSpace(definitions[len(definitions)-1].ID)
			var last development.SourceDefinition
			for _, source := range model.Sources {
				if strings.TrimSpace(source.ID) == lastID {
					last = source
					break
				}
			}
			result := sourceMutationResult{
				SchemaVersion: machine.ContractVersion, SourceModelPath: path, Application: model.Application,
				Source: last.ID, Type: string(last.Type), Identity: sourceDefinitionIdentity(last),
				SourceCount: len(model.Sources), ComponentCount: len(model.Components),
			}
			return writeSourceMutationResult(out, format, result, sourceMutationLines(result, last))
		},
	}
}

func appWorkspaceSourceRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Summary: "Remove one source and detach its component mappings without deleting local files",
		Usage:   "baha app workspace source remove SOURCE [--manifest PATH] [--keep-components] [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg := "."
			format := outputHuman
			keepComponents := false
			var positional []string
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--manifest":
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace source remove --help'.")
					}
					i++
					manifestArg = args[i]
				case "-o", "--output":
					if i+1 >= len(args) {
						return usageError(args[i]+" requires a value", "Use --output json.")
					}
					i++
					if args[i] != "json" {
						return usageError("--output supports only json", "Use --output json.")
					}
					format = outputJSON
				case "--keep-components":
					keepComponents = true
				default:
					if strings.HasPrefix(args[i], "-") {
						return unknownOptionUsage("baha app workspace source remove", args[i], "--manifest", "--keep-components", "--output")
					}
					positional = append(positional, args[i])
				}
			}
			if len(positional) != 1 {
				return usageError("source remove requires exactly one SOURCE", "Example: baha app workspace source remove legacy")
			}
			sourceID := strings.TrimSpace(positional[0])
			manifestPath, state, err := loadWorkspaceSourceModel(ctx, manifestArg)
			if err != nil {
				return err
			}
			if err := authorizeWorkspaceSourceModel(ctx, "workspace.init", manifestPath, state.Manifest); err != nil {
				return err
			}
			model := state.Model
			var referenced []string
			for _, component := range model.Components {
				if strings.TrimSpace(component.Source) == sourceID {
					referenced = append(referenced, component.Component)
				}
			}
			if keepComponents && len(referenced) > 0 {
				return usageError("source "+sourceID+" is still referenced by component(s) "+strings.Join(referenced, ", "), "Detach those components first with 'baha app workspace component remove NAME', run without --keep-components to detach them here, or retarget them with 'baha app workspace component add'.")
			}
			sourceModelPath, mappingPath, detached, removedMapping, err := development.SaveRemovableSource(manifestPath, model.Application, model, sourceID)
			if err != nil {
				if errors.Is(err, development.ErrWorkspaceLastSource) {
					return usageError("source remove would leave the application without any source", "Remove or retarget the remaining component mappings first, or keep this source.")
				}
				return err
			}
			result := sourceMutationResult{
				SchemaVersion: machine.ContractVersion, SourceModelPath: sourceModelPath, MappingPath: mappingPath,
				Application: model.Application, Source: sourceID, Removed: true, RemovedMapping: removedMapping,
				DetachedComponents: detached, SourceCount: len(model.Sources), ComponentCount: len(model.Components),
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "removed source %s\n", sourceID)
			if len(detached) > 0 {
				fmt.Fprintf(out, "detached components: %s\n", strings.Join(detached, ", "))
			}
			if removedMapping {
				fmt.Fprintf(out, "detached local mapping; local files were not deleted: %s\n", mappingPath)
			}
			fmt.Fprintf(out, "source model: %s\n", sourceModelPath)
			return nil
		},
	}
}

func appWorkspaceSourceCommand() *cli.Command {
	return &cli.Command{
		Name:    "source",
		Summary: "Add or remove individual application sources",
		Usage:   "baha app workspace source <add|remove> [options]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return usageError("workspace source requires add or remove", "Run 'baha app workspace source add --help' or 'baha app workspace source remove --help'.")
		},
		Children: []*cli.Command{
			appWorkspaceSourceAddCommand(),
			appWorkspaceSourceRemoveCommand(),
		},
	}
}

func appWorkspaceComponentListCommand() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Summary: "List unique components and the source identity each one maps to",
		Usage:   "baha app workspace component list [COMPONENT] [--manifest PATH] [--source ID] [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg, format, sourceFilter, positional, err := parseWorkspaceComponentListArgs(args)
			if err != nil {
				return err
			}
			if len(positional) > 1 {
				return usageError("component list accepts at most one optional component filter", "Example: baha app workspace component list [COMPONENT]")
			}
			manifestPath, state, err := loadWorkspaceSourceModel(ctx, manifestArg)
			if err != nil {
				return err
			}
			if err := authorizeWorkspaceSourceModel(ctx, "workspace.show", manifestPath, state.Manifest); err != nil {
				return err
			}
			model := state.Model
			views := componentViewsForModel(model, sourceFilter)
			if len(positional) == 1 {
				needle := strings.TrimSpace(positional[0])
				filtered := views[:0:0]
				for _, view := range views {
					if view.Component == needle {
						filtered = append(filtered, view)
					}
				}
				views = filtered
			}
			sort.Slice(views, func(i, j int) bool { return views[i].Component < views[j].Component })
			if format == outputJSON {
				return writeJSON(out, map[string]any{"schema_version": machine.ContractVersion, "application": model.Application, "components": views})
			}
			fmt.Fprintf(out, "application: %s\n", model.Application)
			if len(views) == 0 {
				fmt.Fprintln(out, "components: none")
				return nil
			}
			for _, view := range views {
				sub := ""
				if view.SubPath != "" && view.SubPath != "." {
					sub = " @" + view.SubPath
				}
				fmt.Fprintf(out, "  %s -> %s (%s%s)\n", view.Component, view.Source, view.Type, sub)
			}
			return nil
		},
	}
}

func componentViewsForModel(model development.SourceModel, sourceFilter string) []sourceComponentView {
	views := make([]sourceComponentView, 0, len(model.Components))
	for _, component := range model.Components {
		if sourceFilter != "" && strings.TrimSpace(component.Source) != sourceFilter {
			continue
		}
		var definition development.SourceDefinition
		for _, source := range model.Sources {
			if source.ID == strings.TrimSpace(component.Source) {
				definition = source
				break
			}
		}
		views = append(views, sourceComponentView{
			Component: component.Component, Source: component.Source, Type: string(definition.Type),
			Identity: sourceDefinitionIdentity(definition), SubPath: filepath.ToSlash(strings.TrimSpace(component.SubPath)),
		})
	}
	return views
}

func appWorkspaceComponentAddCommand() *cli.Command {
	return &cli.Command{
		Name:    "add",
		Summary: "Add or retarget the unique mapping of one component without inventing it implicitly",
		Usage:   "baha app workspace component add COMPONENT SOURCE[@SUBPATH] [--manifest PATH] [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg := "."
			format := outputHuman
			var positional []string
			for i := 0; i < len(args); i++ {
				if args[i] == "--manifest" {
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace component add --help'.")
					}
					i++
					manifestArg = args[i]
					continue
				}
				if args[i] == "-o" || args[i] == "--output" {
					if i+1 >= len(args) || args[i+1] != "json" {
						return usageError("--output supports only json", "Use --output json.")
					}
					i++
					format = outputJSON
					continue
				}
				if strings.HasPrefix(args[i], "-") {
					return unknownOptionUsage("baha app workspace component add", args[i], "--manifest", "--output")
				}
				positional = append(positional, args[i])
			}
			if len(positional) != 2 {
				return usageError("component add requires COMPONENT and SOURCE[@SUBPATH]", "Example: baha app workspace component add api backend@services/api")
			}
			componentName := strings.TrimSpace(positional[0])
			source, subPath, hasSubPath := strings.Cut(strings.TrimSpace(positional[1]), "@")
			mapping := development.ComponentSource{Component: componentName, Source: strings.TrimSpace(source)}
			if hasSubPath {
				mapping.SubPath = strings.TrimSpace(subPath)
			}
			manifestPath, state, err := loadWorkspaceSourceModel(ctx, manifestArg)
			if err != nil {
				return err
			}
			if err := authorizeWorkspaceSourceModel(ctx, "workspace.init", manifestPath, state.Manifest); err != nil {
				return err
			}
			model := state.Model
			if err := development.UpsertComponentMapping(&model, mapping); err != nil {
				return err
			}
			path, err := development.WriteSourceModel(manifestPath, model)
			if err != nil {
				return err
			}
			result := sourceMutationResult{
				SchemaVersion: machine.ContractVersion, SourceModelPath: path, Application: model.Application,
				Component: componentName, Source: mapping.Source, ComponentSourceRef: mapping.Source, SubPath: filepath.ToSlash(mapping.SubPath),
				SourceCount: len(model.Sources), ComponentCount: len(model.Components),
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			target := mapping.Source
			if mapping.SubPath != "" {
				target += " @" + filepath.ToSlash(mapping.SubPath)
			}
			fmt.Fprintf(out, "component %s -> source %s\n", componentName, target)
			fmt.Fprintf(out, "source model: %s\n", path)
			return nil
		},
	}
}

func appWorkspaceComponentRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Summary: "Detach one component mapping without deleting sources or local files",
		Usage:   "baha app workspace component remove COMPONENT [--manifest PATH] [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg := "."
			format := outputHuman
			var positional []string
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--manifest":
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace component remove --help'.")
					}
					i++
					manifestArg = args[i]
				case "-o", "--output":
					if i+1 >= len(args) {
						return usageError(args[i]+" requires a value", "Use --output json.")
					}
					i++
					if args[i] != "json" {
						return usageError("--output supports only json", "Use --output json.")
					}
					format = outputJSON
				default:
					if strings.HasPrefix(args[i], "-") {
						return unknownOptionUsage("baha app workspace component remove", args[i], "--manifest", "--output")
					}
					positional = append(positional, args[i])
				}
			}
			if len(positional) != 1 {
				return usageError("component remove requires exactly one COMPONENT", "Example: baha app workspace component remove worker")
			}
			manifestPath, state, err := loadWorkspaceSourceModel(ctx, manifestArg)
			if err != nil {
				return err
			}
			if err := authorizeWorkspaceSourceModel(ctx, "workspace.init", manifestPath, state.Manifest); err != nil {
				return err
			}
			model := state.Model
			componentName := strings.TrimSpace(positional[0])
			sourceID := ""
			for _, component := range model.Components {
				if strings.TrimSpace(component.Component) == componentName {
					sourceID = strings.TrimSpace(component.Source)
					break
				}
			}
			if err := development.RemoveComponentMapping(&model, componentName); err != nil {
				return err
			}
			path, err := development.WriteSourceModel(manifestPath, model)
			if err != nil {
				return err
			}
			result := sourceMutationResult{
				SchemaVersion: machine.ContractVersion, SourceModelPath: path, Application: model.Application,
				Component: componentName, Source: sourceID, Removed: true,
				SourceCount: len(model.Sources), ComponentCount: len(model.Components),
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "detached component %s\n", componentName)
			if remaining := development.SourceComponentNames(model, sourceID); len(remaining) == 0 && len(model.Sources) > 1 {
				fmt.Fprintf(out, "source %s is now unowned; remove it with 'baha app workspace source remove %s'\n", sourceID, sourceID)
			}
			fmt.Fprintf(out, "source model: %s\n", path)
			return nil
		},
	}
}

func appWorkspaceComponentCommand() *cli.Command {
	return &cli.Command{
		Name:    "component",
		Summary: "Inspect and maintain unique component-to-source mappings",
		Usage:   "baha app workspace component <list|add|remove> [options]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return usageError("workspace component requires list, add or remove", "Run 'baha app workspace component list --help'.")
		},
		Children: []*cli.Command{
			appWorkspaceComponentListCommand(),
			appWorkspaceComponentAddCommand(),
			appWorkspaceComponentRemoveCommand(),
		},
	}
}

func appWorkspacePathCommand() *cli.Command {
	return &cli.Command{
		Name:    "path",
		Summary: "Set or show the local path of one source (checkout mapping or directory source)",
		Usage:   "baha app workspace path SOURCE [PATH] [--manifest PATH] [--show-current] [-o json]",
		Aliases: []string{"paths"},
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			manifestArg := "."
			format := outputHuman
			showCurrent := false
			var positional []string
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--manifest":
					if i+1 >= len(args) {
						return usageError("--manifest requires a path", "Run 'baha app workspace path --help'.")
					}
					i++
					manifestArg = args[i]
				case "--show-current":
					showCurrent = true
				case "-o", "--output":
					if i+1 >= len(args) {
						return usageError(args[i]+" requires a value", "Use --output json.")
					}
					i++
					if args[i] != "json" {
						return usageError("--output supports only json", "Use --output json.")
					}
					format = outputJSON
				default:
					if strings.HasPrefix(args[i], "-") {
						return unknownOptionUsage("baha app workspace path", args[i], "--manifest", "--show-current", "--output")
					}
					positional = append(positional, args[i])
				}
			}
			if len(positional) > 1 && showCurrent {
				return usageError("--show-current cannot be combined with a new path", "Choose one of them.")
			}
			if len(positional) == 0 {
				return usageError("path requires SOURCE", "Example: baha app workspace path api-source ~/dev/api")
			}
			sourceID := strings.TrimSpace(positional[0])
			manifestPath, state, err := loadWorkspaceSourceModel(ctx, manifestArg)
			if err != nil {
				return err
			}
			model := state.Model
			// Local source paths must not collide with another source's path.
			ownershipCandidate := ""
			if len(positional) == 2 && !showCurrent {
				ownershipCandidate = strings.TrimSpace(positional[1])
			}
			var definition development.SourceDefinition
			found := false
			for _, source := range model.Sources {
				if strings.TrimSpace(source.ID) == sourceID {
					definition, found = source, true
					break
				}
			}
			if !found {
				return usageError("unknown source "+sourceID, "Run 'baha app workspace show' to inspect source identities.")
			}
			authorizationOperation := "workspace.show"
			if len(positional) == 2 && !showCurrent {
				if definition.Type == development.SourceRepository {
					authorizationOperation = "workspace.map"
				} else if definition.Type == development.SourceLocal {
					authorizationOperation = "workspace.init"
				}
			}
			if err := authorizeWorkspaceSourceModel(ctx, authorizationOperation, manifestPath, state.Manifest); err != nil {
				return err
			}
			// Local source paths are persisted verbatim in the portable model;
			// normalize them here the same way the init parser does.
			if definition.Type == development.SourceLocal {
				resolved, err := filepath.Abs(strings.TrimSpace(definition.Path))
				if err != nil {
					return err
				}
				definition.Path = resolved
			}
			if ownershipCandidate != "" && definition.Type == development.SourceLocal {
				mappingForOwnership, _, mapErr := development.LoadWorkspaceMapping(manifestPath, state.Manifest.Name)
				if mapErr == nil {
					candidateAbs, absErr := filepath.Abs(ownershipCandidate)
					if absErr != nil {
						return absErr
					}
					for id, mapped := range mappingForOwnership.Sources {
						mappedAbs, mappedErr := filepath.Abs(strings.TrimSpace(mapped))
						if mappedErr == nil && id != sourceID && mappedAbs == candidateAbs {
							return usageError("this path is already mapped to source "+id, "One path belongs to exactly one source; remove that mapping first.")
						}
					}
				} else if !os.IsNotExist(mapErr) {
					return mapErr
				}
			}
			result := sourceMutationResult{
				SchemaVersion: machine.ContractVersion, SourceModelPath: state.ModelPath, Application: model.Application,
				Source: sourceID, Type: string(definition.Type), SourceCount: len(model.Sources), ComponentCount: len(model.Components),
			}
			switch definition.Type {
			case development.SourceLocal:
				current := definition.Path
				if len(positional) == 1 || showCurrent {
					result.Identity = current
					return writePathResult(out, format, result, sourceID, current)
				}
				if len(positional) > 2 {
					return usageError("path accepts SOURCE and one PATH", "Example: baha app workspace path legacy ~/srv/legacy")
				}
				resolved, err := filepath.Abs(strings.TrimSpace(positional[1]))
				if err != nil {
					return err
				}
				if err := development.UpsertSourceDefinition(&model, development.SourceDefinition{ID: sourceID, Type: development.SourceLocal, Path: resolved}); err != nil {
					return err
				}
				path, err := development.WriteSourceModel(manifestPath, model)
				if err != nil {
					return err
				}
				result.SourceModelPath = path
				result.Identity = resolved
				return writePathResult(out, format, result, sourceID, resolved)
			case development.SourceRepository:
				if len(positional) > 2 || (len(positional) == 2 && showCurrent) {
					return usageError("path accepts SOURCE and one PATH", "Example: baha app workspace path api-source ~/dev/api")
				}
				mapping, mappingPath, mapErr := development.LoadWorkspaceMapping(manifestPath, state.Manifest.Name)
				if os.IsNotExist(mapErr) {
					mapping = development.WorkspaceMapping{
						SchemaVersion: development.WorkspaceMappingVersion, Application: state.Manifest.Name,
						Manifest: manifestPath, Sources: map[string]string{},
					}
				} else if mapErr != nil {
					return mapErr
				}
				if mapping.Sources == nil {
					mapping.Sources = map[string]string{}
				}
				if len(positional) == 1 || showCurrent {
					result.Identity = mapping.Sources[sourceID]
					result.MappingPath = mappingPath
					return writePathResult(out, format, result, sourceID, mapping.Sources[sourceID])
				}
				candidate := strings.TrimSpace(positional[1])
				abs, err := filepath.Abs(candidate)
				if err != nil {
					return err
				}
				info, err := os.Stat(abs)
				if err != nil || !info.IsDir() {
					return usageError("workspace source path is not an existing directory", "Point at an existing Git checkout/worktree; 'baha app workspace path' never creates or deletes directories.")
				}
				// Ownership-safe: a path may belong to exactly one source of
				// this application. The declared repository identity stays
				// portable metadata; the Git state of a checkout belongs to
				// 'baha app workspace status', not to this mapping.
				for id, mapped := range mapping.Sources {
					mappedAbs, mapErr := filepath.Abs(strings.TrimSpace(mapped))
					if mapErr == nil && id != sourceID && mappedAbs == abs {
						return usageError("this path is already mapped to source "+id, "One path belongs to exactly one source; remove that mapping first.")
					}
				}
				mapping.Sources[sourceID] = abs
				path, err := development.SaveWorkspaceMapping(manifestPath, mapping)
				if err != nil {
					return err
				}
				result.MappingPath = path
				result.Identity = abs
				if format == outputJSON {
					return writeJSON(out, result)
				}
				fmt.Fprintf(out, "mapped %s -> %s\n", sourceID, displayUserPath(abs))
				fmt.Fprintf(out, "workspace state: %s\n", path)
				return nil
			default:
				return usageError("OCI sources do not have local paths", "Inspect the image identity with 'baha app workspace show'.")
			}
		},
	}
}

func writePathResult(out io.Writer, format cliOutputFormat, result sourceMutationResult, sourceID, current string) error {
	if format == outputJSON {
		return writeJSON(out, result)
	}
	if strings.TrimSpace(current) == "" {
		fmt.Fprintf(out, "%s: UNMAPPED\n", sourceID)
		fmt.Fprintln(out, "next: baha app workspace path "+sourceID+" <existing-directory>")
		return nil
	}
	fmt.Fprintf(out, "%s -> %s\n", sourceID, displayUserPath(current))
	return nil
}

type sourceComponentView struct {
	Component string `json:"component"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Identity  string `json:"identity"`
	SubPath   string `json:"sub_path,omitempty"`
}

func writeSourceMutationResult(out io.Writer, format cliOutputFormat, result sourceMutationResult, lines []string) error {
	if format == outputJSON {
		return writeJSON(out, result)
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return nil
}

func parseWorkspaceComponentListArgs(args []string) (manifestArg string, format cliOutputFormat, sourceFilter string, positional []string, err error) {
	manifestArg = "."
	format = outputHuman
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--manifest":
			if i+1 >= len(args) {
				return "", outputHuman, "", nil, usageError("--manifest requires a path", "Run 'baha app workspace component list --help'.")
			}
			i++
			manifestArg = args[i]
		case "--source":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", outputHuman, "", nil, usageError("--source requires a source id", "Run 'baha app workspace component list --help'.")
			}
			i++
			sourceFilter = strings.TrimSpace(args[i])
		case "-o", "--output":
			if i+1 >= len(args) {
				return "", outputHuman, "", nil, usageError(args[i]+" requires a value", "Use --output json.")
			}
			i++
			if args[i] != "json" {
				return "", outputHuman, "", nil, usageError("--output supports only json", "Use --output json.")
			}
			format = outputJSON
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", outputHuman, "", nil, unknownOptionUsage("baha app workspace component list", args[i], "--manifest", "--source", "--output")
			}
			positional = append(positional, args[i])
		}
	}
	return manifestArg, format, sourceFilter, positional, nil
}
