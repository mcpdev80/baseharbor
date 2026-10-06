package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type workspaceMutationResult struct {
	SchemaVersion   string `json:"schema_version"`
	SourceModelPath string `json:"source_model_path,omitempty"`
	MappingPath     string `json:"mapping_path,omitempty"`
}

func initializeApplicationWorkspace(ctx context.Context, input machineWorkspaceInitInput) (workspaceMutationResult, error) {
	ctx = withTargetOverride(ctx, input.Target)
	manifestPath, manifest, err := resolveWorkspaceManifest(input.Manifest)
	if err != nil {
		return workspaceMutationResult{}, err
	}
	if err := authorizeMCPOperation(ctx, "workspace.init", "", manifest.Environment, manifest.ApplicationID, manifestPath); err != nil {
		return workspaceMutationResult{}, err
	}
	model := development.SourceModel{SchemaVersion: development.SourceModelVersion, Application: manifest.Name, Sources: input.Sources, Components: input.Components}
	path, err := development.WriteSourceModel(manifestPath, model)
	if err != nil {
		return workspaceMutationResult{}, err
	}
	return workspaceMutationResult{SchemaVersion: machine.ContractVersion, SourceModelPath: path}, nil
}

func mapApplicationWorkspace(ctx context.Context, manifestArg, source, checkout string) (workspaceMutationResult, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(checkout) == "" {
		return workspaceMutationResult{}, usageError("workspace mapping requires a source identity and checkout path", "Provide both source and path.")
	}
	positional := []string{source, checkout}
	manifestPath, manifest, err := resolveWorkspaceManifest(manifestArg)
	if err != nil {
		return workspaceMutationResult{}, err
	}
	if err := authorizeMCPOperation(ctx, "workspace.map", "", manifest.Environment, manifest.ApplicationID, manifestPath); err != nil {
		return workspaceMutationResult{}, err
	}
	model, _, err := development.LoadSourceModel(manifestPath)
	sourceID := strings.TrimSpace(positional[0])
	bootstrap := false
	if err != nil {
		if !errors.Is(err, development.ErrWorkspaceModelMissing) {
			return workspaceMutationResult{}, err
		}
		checkout, absErr := filepath.Abs(strings.TrimSpace(positional[1]))
		if absErr != nil {
			return workspaceMutationResult{}, absErr
		}
		info, statErr := os.Stat(checkout)
		if statErr != nil || !info.IsDir() {
			return workspaceMutationResult{}, usageError("workspace source path is not an existing directory", "Map an existing Git checkout/worktree.")
		}
		repository := strings.TrimSpace(workspaceGitValue(ctx, checkout, "config", "--get", "remote.origin.url"))
		if repository == "" {
			return workspaceMutationResult{}, usageError("cannot bootstrap workspace source without a stable Git origin", "Configure remote.origin.url, or run 'baha app workspace init --source ID=REPOSITORY --component COMPONENT=ID' explicitly.")
		}
		components := application.WorkloadComponentNames(manifest)
		if len(components) != 1 {
			return workspaceMutationResult{}, usageError("cannot infer the first workspace component", "Run 'baha app workspace init --source ID=REPOSITORY --component COMPONENT=ID' explicitly for zero- or multi-component applications.")
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
				return workspaceMutationResult{}, usageError("OCI sources do not have local workspace mappings", "Map only repository sources.")
			}
			found = true
			break
		}
	}
	if !found {
		return workspaceMutationResult{}, usageError("unknown source "+sourceID, "Run 'baha app workspace show' to inspect source identities.")
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
		return workspaceMutationResult{}, err
	}
	if mapping.Sources == nil {
		mapping.Sources = map[string]string{}
	}
	mapping.Sources[sourceID] = positional[1]
	var sourceModelPath string
	if bootstrap {
		sourceModelPath, err = development.WriteSourceModel(manifestPath, model)
		if err != nil {
			return workspaceMutationResult{}, err
		}
	}
	path, err := development.SaveWorkspaceMapping(manifestPath, mapping)
	if err != nil {
		if sourceModelPath != "" {
			_ = os.Remove(sourceModelPath)
		}
		return workspaceMutationResult{}, err
	}
	return workspaceMutationResult{SchemaVersion: machine.ContractVersion, MappingPath: path, SourceModelPath: sourceModelPath}, nil
}
