package development

import (
	"crypto/sha256"
	"errors"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

var ErrWorkspaceSourceMissing = errors.New("workspace source is missing")

type WorkspaceSourceError struct {
	Source    string
	Component string
	Path      string
	Problem   string
}

func (e *WorkspaceSourceError) Error() string {
	if e == nil {
		return ErrWorkspaceSourceMissing.Error()
	}
	detail := strings.TrimSpace(e.Problem)
	if detail == "" {
		detail = "local worktree mapping is unavailable"
	}
	if strings.TrimSpace(e.Path) != "" {
		return fmt.Sprintf("workspace source %q for component %q: %s at %s", e.Source, e.Component, detail, e.Path)
	}
	return fmt.Sprintf("workspace source %q for component %q: %s", e.Source, e.Component, detail)
}

func (e *WorkspaceSourceError) Unwrap() error { return ErrWorkspaceSourceMissing }

const (
	SourceModelVersion         = "baseharbor.sources/v1"
	WorkspaceMappingVersion    = "baseharbor.workspace/v1"
	WorkspaceResolutionVersion = "baseharbor.workspace-resolution/v1"
	SourceModelRelativePath    = ".baseharbor/sources.yaml"
)

type SourceKind string

const (
	SourceRepository SourceKind = "repository"
	SourceOCI        SourceKind = "oci"
)

type SourceDefinition struct {
	ID         string     `json:"id" yaml:"id"`
	Type       SourceKind `json:"type" yaml:"type"`
	Repository string     `json:"repository,omitempty" yaml:"repository,omitempty"`
	Ref        string     `json:"ref,omitempty" yaml:"ref,omitempty"`
	Image      string     `json:"image,omitempty" yaml:"image,omitempty"`
}

type ComponentSource struct {
	Component string `json:"component" yaml:"component"`
	Source    string `json:"source" yaml:"source"`
	SubPath   string `json:"sub_path,omitempty" yaml:"subPath,omitempty"`
}

type SourceModel struct {
	SchemaVersion string             `json:"schema_version" yaml:"schemaVersion"`
	Application   string             `json:"application" yaml:"application"`
	Sources       []SourceDefinition `json:"sources" yaml:"sources"`
	Components    []ComponentSource  `json:"components" yaml:"components"`
}

type WorkspaceMapping struct {
	SchemaVersion string            `json:"schema_version"`
	Application   string            `json:"application"`
	Manifest      string            `json:"manifest"`
	Sources       map[string]string `json:"sources"`
}

type ResolvedComponentSource struct {
	Component string     `json:"component"`
	Source    string     `json:"source"`
	Type      SourceKind `json:"type"`
	Identity  string     `json:"identity"`
	Ref       string     `json:"ref,omitempty"`
	Root      string     `json:"root,omitempty"`
	SubPath   string     `json:"sub_path,omitempty"`
	Image     string     `json:"image,omitempty"`
}

type WorkspaceResolution struct {
	SchemaVersion string                    `json:"schema_version"`
	Application   string                    `json:"application"`
	Manifest      string                    `json:"manifest"`
	Components    []ResolvedComponentSource `json:"components"`
}

func (m SourceModel) Validate() error {
	if m.SchemaVersion != SourceModelVersion {
		return fmt.Errorf("source model schema_version %q is unsupported; expected %q", m.SchemaVersion, SourceModelVersion)
	}
	if strings.TrimSpace(m.Application) == "" {
		return fmt.Errorf("source model application is required")
	}
	if len(m.Sources) == 0 {
		return fmt.Errorf("source model requires at least one source")
	}
	sourceIDs := map[string]SourceDefinition{}
	for _, source := range m.Sources {
		id := strings.TrimSpace(source.ID)
		if id == "" {
			return fmt.Errorf("source id is required")
		}
		if _, exists := sourceIDs[id]; exists {
			return fmt.Errorf("source %q is declared more than once", id)
		}
		switch source.Type {
		case SourceRepository:
			if strings.TrimSpace(source.Repository) == "" {
				return fmt.Errorf("repository source %q requires repository identity", id)
			}
			if strings.TrimSpace(source.Image) != "" {
				return fmt.Errorf("repository source %q must not declare image", id)
			}
		case SourceOCI:
			if strings.TrimSpace(source.Image) == "" {
				return fmt.Errorf("OCI source %q requires image reference", id)
			}
			if strings.TrimSpace(source.Repository) != "" || strings.TrimSpace(source.Ref) != "" {
				return fmt.Errorf("OCI source %q must not declare repository/ref", id)
			}
		default:
			return fmt.Errorf("source %q has unsupported type %q", id, source.Type)
		}
		sourceIDs[id] = source
	}
	if len(m.Components) == 0 {
		return fmt.Errorf("source model requires at least one component mapping")
	}
	components := map[string]struct{}{}
	for _, component := range m.Components {
		id := strings.TrimSpace(component.Component)
		if id == "" {
			return fmt.Errorf("component source mapping requires component")
		}
		if _, exists := components[id]; exists {
			return fmt.Errorf("component %q is mapped more than once", id)
		}
		components[id] = struct{}{}
		if _, exists := sourceIDs[strings.TrimSpace(component.Source)]; !exists {
			return fmt.Errorf("component %q references unknown source %q", id, component.Source)
		}
		if err := validateSourceSubPath(component.SubPath); err != nil {
			return fmt.Errorf("component %q: %w", id, err)
		}
	}
	return nil
}

func validateSourceSubPath(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return nil
	}
	if filepath.IsAbs(value) {
		return fmt.Errorf("source subPath must be relative")
	}
	clean := filepath.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("source subPath escapes its source root")
	}
	return nil
}

func LoadSourceModel(manifestPath string) (SourceModel, string, error) {
	manifestPath, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return SourceModel{}, "", err
	}
	path := filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(SourceModelRelativePath))
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceModel{}, path, err
	}
	var model SourceModel
	if err := yaml.Unmarshal(data, &model); err != nil {
		return SourceModel{}, path, fmt.Errorf("decode source model: %w", err)
	}
	if err := model.Validate(); err != nil {
		return SourceModel{}, path, err
	}
	return model, path, nil
}

func WriteSourceModel(manifestPath string, model SourceModel) (string, error) {
	if err := model.Validate(); err != nil {
		return "", err
	}
	manifestPath, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(SourceModelRelativePath))
	data, err := yaml.Marshal(model)
	if err != nil {
		return "", fmt.Errorf("encode source model: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

func WorkspaceMappingPath(manifestPath, application string) (string, error) {
	manifestPath, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return "", err
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(manifestPath)))
	key := hex.EncodeToString(sum[:6])
	app := strings.TrimSpace(application)
	if app == "" {
		return "", fmt.Errorf("workspace application is required")
	}
	return filepath.Join(configRoot, "baseharbor", "workspaces", app+"-"+key+".json"), nil
}

func LoadWorkspaceMapping(manifestPath, application string) (WorkspaceMapping, string, error) {
	path, err := WorkspaceMappingPath(manifestPath, application)
	if err != nil {
		return WorkspaceMapping{}, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceMapping{}, path, err
	}
	var mapping WorkspaceMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		return WorkspaceMapping{}, path, fmt.Errorf("decode workspace mapping: %w", err)
	}
	if mapping.SchemaVersion != WorkspaceMappingVersion {
		return WorkspaceMapping{}, path, fmt.Errorf("workspace mapping schema_version %q is unsupported", mapping.SchemaVersion)
	}
	if mapping.Application != application {
		return WorkspaceMapping{}, path, fmt.Errorf("workspace mapping application %q does not match %q", mapping.Application, application)
	}
	return mapping, path, nil
}

func SaveWorkspaceMapping(manifestPath string, mapping WorkspaceMapping) (string, error) {
	manifestPath, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return "", err
	}
	if mapping.SchemaVersion == "" {
		mapping.SchemaVersion = WorkspaceMappingVersion
	}
	if mapping.SchemaVersion != WorkspaceMappingVersion {
		return "", fmt.Errorf("workspace mapping schema_version %q is unsupported", mapping.SchemaVersion)
	}
	if strings.TrimSpace(mapping.Application) == "" {
		return "", fmt.Errorf("workspace mapping application is required")
	}
	if mapping.Sources == nil {
		mapping.Sources = map[string]string{}
	}
	mapping.Manifest = manifestPath
	for id, root := range mapping.Sources {
		if strings.TrimSpace(id) == "" {
			return "", fmt.Errorf("workspace source id is required")
		}
		abs, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			return "", err
		}
		mapping.Sources[id] = abs
	}
	path, err := WorkspaceMappingPath(manifestPath, mapping.Application)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

func ResolveWorkspace(manifestPath string, model SourceModel, mapping WorkspaceMapping) (WorkspaceResolution, error) {
	if err := model.Validate(); err != nil {
		return WorkspaceResolution{}, err
	}
	manifestPath, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return WorkspaceResolution{}, err
	}
	if mapping.SchemaVersion != WorkspaceMappingVersion {
		return WorkspaceResolution{}, fmt.Errorf("workspace mapping schema_version %q is unsupported", mapping.SchemaVersion)
	}
	if mapping.Application != model.Application {
		return WorkspaceResolution{}, fmt.Errorf("workspace mapping application %q does not match source model %q", mapping.Application, model.Application)
	}
	sources := make(map[string]SourceDefinition, len(model.Sources))
	for _, source := range model.Sources {
		sources[source.ID] = source
	}
	resolution := WorkspaceResolution{
		SchemaVersion: WorkspaceResolutionVersion,
		Application:   model.Application,
		Manifest:      manifestPath,
	}
	for _, component := range model.Components {
		source := sources[component.Source]
		resolved := ResolvedComponentSource{
			Component: component.Component,
			Source:    source.ID,
			Type:      source.Type,
			Ref:       source.Ref,
			SubPath:   filepath.ToSlash(filepath.Clean(component.SubPath)),
		}
		if component.SubPath == "" {
			resolved.SubPath = ""
		}
		switch source.Type {
		case SourceOCI:
			resolved.Identity = source.Image
			resolved.Image = source.Image
		case SourceRepository:
			resolved.Identity = source.Repository
			root := strings.TrimSpace(mapping.Sources[source.ID])
			if root == "" {
				return WorkspaceResolution{}, &WorkspaceSourceError{Source: source.ID, Component: component.Component, Problem: "is not mapped; map an existing local worktree before mutation"}
			}
			root, err = filepath.Abs(root)
			if err != nil {
				return WorkspaceResolution{}, err
			}
			info, err := os.Stat(root)
			if err != nil {
				if os.IsNotExist(err) {
					return WorkspaceResolution{}, &WorkspaceSourceError{Source: source.ID, Component: component.Component, Path: root, Problem: "mapped worktree is missing; update the local workspace mapping"}
				}
				return WorkspaceResolution{}, err
			}
			if !info.IsDir() {
				return WorkspaceResolution{}, fmt.Errorf("workspace source %q path %s is not a directory", source.ID, root)
			}
			componentRoot := root
			if strings.TrimSpace(component.SubPath) != "" {
				componentRoot = filepath.Join(root, filepath.FromSlash(component.SubPath))
			}
			componentRoot, err = filepath.Abs(componentRoot)
			if err != nil {
				return WorkspaceResolution{}, err
			}
			rel, err := filepath.Rel(root, componentRoot)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return WorkspaceResolution{}, fmt.Errorf("component %q source path escapes mapped worktree", component.Component)
			}
			info, err = os.Stat(componentRoot)
			if err != nil {
				if os.IsNotExist(err) {
					return WorkspaceResolution{}, &WorkspaceSourceError{Source: source.ID, Component: component.Component, Path: componentRoot, Problem: "component subpath is missing inside mapped worktree"}
				}
				return WorkspaceResolution{}, err
			}
			if !info.IsDir() {
				return WorkspaceResolution{}, fmt.Errorf("component %q path %s is not a directory", component.Component, componentRoot)
			}
			resolved.Root = componentRoot
		}
		resolution.Components = append(resolution.Components, resolved)
	}
	sort.Slice(resolution.Components, func(i, j int) bool {
		return resolution.Components[i].Component < resolution.Components[j].Component
	})
	return resolution, nil
}
