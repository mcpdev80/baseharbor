package development

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

var (
	ErrWorkspaceSourceMissing = errors.New("workspace source is missing")
	ErrWorkspaceModelMissing  = errors.New("workspace source model is not initialized")
	// ErrWorkspaceLastSource refuses removing the final source of a model so an
	// application never loses its last portable source identity by accident.
	ErrWorkspaceLastSource = errors.New("workspace source model requires at least one source")
)

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
	SourceLocal      SourceKind = "local"
	SourceOCI        SourceKind = "oci"
)

type SourceDefinition struct {
	ID         string     `json:"id" yaml:"id"`
	Type       SourceKind `json:"type" yaml:"type"`
	Repository string     `json:"repository,omitempty" yaml:"repository,omitempty"`
	Ref        string     `json:"ref,omitempty" yaml:"ref,omitempty"`
	Path       string     `json:"path,omitempty" yaml:"path,omitempty"`
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
		return fmt.Errorf("%w: run 'baha app workspace init' to create %s", ErrWorkspaceModelMissing, SourceModelRelativePath)
	}
	sourceIDs := map[string]SourceDefinition{}
	for _, source := range m.Sources {
		if err := validatePublicSourceIdentity(source.Repository); err != nil {
			return err
		}
		if err := validatePublicSourceIdentity(source.Image); err != nil {
			return err
		}
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
			if strings.TrimSpace(source.Image) != "" || strings.TrimSpace(source.Path) != "" {
				return fmt.Errorf("repository source %q must not declare image/path", id)
			}
		case SourceLocal:
			if strings.TrimSpace(source.Path) == "" {
				return fmt.Errorf("local source %q requires an existing directory path", id)
			}
			if err := validateLocalSourceLocation(id, source.Path); err != nil {
				return err
			}
			if strings.TrimSpace(source.Repository) != "" || strings.TrimSpace(source.Ref) != "" || strings.TrimSpace(source.Image) != "" {
				return fmt.Errorf("local source %q must not declare repository/ref/image", id)
			}
		case SourceOCI:
			if strings.TrimSpace(source.Image) == "" {
				return fmt.Errorf("OCI source %q requires image reference", id)
			}
			if strings.TrimSpace(source.Repository) != "" || strings.TrimSpace(source.Ref) != "" || strings.TrimSpace(source.Path) != "" {
				return fmt.Errorf("OCI source %q must not declare repository/ref/path", id)
			}
		default:
			return fmt.Errorf("source %q has unsupported type %q", id, source.Type)
		}
		sourceIDs[id] = source
	}
	// Components are optional unique identities: an initialized source model may
	// legitimately declare zero components until one is added incrementally.
	// Multiple components may share one source; each component name is unique.
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

// validateLocalSourceLocation keeps portable source identity honest: a local
// directory source names an absolute existing directory and never a file.
func validateLocalSourceLocation(id, location string) error {
	trimmed := strings.TrimSpace(location)
	if !filepath.IsAbs(trimmed) {
		return fmt.Errorf("local source %q must use an absolute directory path", id)
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("local source %q directory %s does not exist", id, trimmed)
		}
		return fmt.Errorf("local source %q directory %s: %w", id, trimmed, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local source %q path %s is not a directory", id, trimmed)
	}
	return nil
}

// FilterSourcesForComponents restricts a source model and its workspace mapping
// to the sources backing the requested components. Components are optional and
// unique per app, and several components may share one source, so the filtered
// model keeps each underlying repository exactly once. An empty request returns
// the model and mapping unchanged. Every requested component must resolve to a
// declared source; an unknown request fails closed so a constrained pull is
// never silently partial. The filtered model is a valid SourceModel (same
// schema_version, application, sources and the shared components), so callers
// can hand it to the existing inspection/update helpers unchanged.
func FilterSourcesForComponents(model SourceModel, mapping WorkspaceMapping, requested []string) (SourceModel, WorkspaceMapping, error) {
	if len(requested) == 0 {
		return model, mapping, nil
	}

	if err := model.Validate(); err != nil {
		return SourceModel{}, WorkspaceMapping{}, err
	}

	byID := map[string]SourceDefinition{}
	for _, source := range model.Sources {
		byID[source.ID] = source
	}

	selected := map[string]struct{}{}
	for _, declaredComponent := range model.Components {
		name := strings.TrimSpace(declaredComponent.Component)
		if name == "" || !containsRequested(requested, name) {
			continue
		}
		source, ok := byID[declaredComponent.Source]
		if !ok {
			return SourceModel{}, WorkspaceMapping{}, fmt.Errorf("component %q references unknown source %q", name, declaredComponent.Source)
		}
		selected[source.ID] = struct{}{}
	}
	if len(selected) == 0 {
		return SourceModel{}, WorkspaceMapping{}, fmt.Errorf("%w: no component named %q resolves to a source", ErrWorkspaceSourceMissing, strings.Join(requested, ", "))
	}

	filtered := model
	filtered.Sources = make([]SourceDefinition, 0, len(selected))
	for _, source := range model.Sources {
		if _, ok := selected[source.ID]; ok {
			filtered.Sources = append(filtered.Sources, source)
		}
	}
	filtered.Components = nil
	for _, component := range model.Components {
		source, ok := byID[component.Source]
		if !ok {
			continue
		}
		if _, keep := selected[source.ID]; keep {
			filtered.Components = append(filtered.Components, component)
		}
	}

	filteredMapping := mapping
	filteredMapping.Sources = map[string]string{}
	for id, path := range mapping.Sources {
		if _, keep := selected[id]; keep {
			filteredMapping.Sources[id] = path
		}
	}
	return filtered, filteredMapping, nil
}

func containsRequested(requested []string, value string) bool {
	for _, item := range requested {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func LoadSourceModel(manifestPath string) (SourceModel, string, error) {
	manifestPath, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return SourceModel{}, "", err
	}
	path := filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(SourceModelRelativePath))
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SourceModel{}, path, fmt.Errorf("%w: run 'baha app workspace init' to create %s", ErrWorkspaceModelMissing, SourceModelRelativePath)
		}
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

func ListWorkspaceMappings() ([]WorkspaceMapping, []error, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return nil, nil, err
	}
	root := filepath.Join(configRoot, "baseharbor", "workspaces")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []WorkspaceMapping{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	mappings := make([]WorkspaceMapping, 0, len(entries))
	var warnings []error
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			warnings = append(warnings, fmt.Errorf("inspect workspace mapping %s: %w", entry.Name(), infoErr))
			continue
		}
		if !info.Mode().IsRegular() {
			warnings = append(warnings, fmt.Errorf("ignore non-regular workspace mapping %s", entry.Name()))
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			warnings = append(warnings, fmt.Errorf("read workspace mapping %s: %w", entry.Name(), readErr))
			continue
		}
		var mapping WorkspaceMapping
		if decodeErr := json.Unmarshal(data, &mapping); decodeErr != nil {
			warnings = append(warnings, fmt.Errorf("decode workspace mapping %s: %w", entry.Name(), decodeErr))
			continue
		}
		if mapping.SchemaVersion != WorkspaceMappingVersion || strings.TrimSpace(mapping.Application) == "" || strings.TrimSpace(mapping.Manifest) == "" {
			warnings = append(warnings, fmt.Errorf("ignore invalid workspace mapping %s", entry.Name()))
			continue
		}
		mappings = append(mappings, mapping)
	}
	sort.Slice(mappings, func(i, j int) bool {
		if mappings[i].Application != mappings[j].Application {
			return mappings[i].Application < mappings[j].Application
		}
		return mappings[i].Manifest < mappings[j].Manifest
	})
	return mappings, warnings, nil
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
		case SourceLocal:
			resolved.Identity = filepath.Clean(strings.TrimSpace(source.Path))
			root := strings.TrimSpace(mapping.Sources[source.ID])
			if root == "" {
				root = resolved.Identity
			}
			root, err = filepath.Abs(root)
			if err != nil {
				return WorkspaceResolution{}, err
			}
			if err := validateLocalSourceLocation(source.ID, root); err != nil {
				return WorkspaceResolution{}, err
			}
			componentRoot, err := resolveContainedWorkspaceDirectory(root, component.SubPath, source.ID, component.Component, "local source directory")
			if err != nil {
				return WorkspaceResolution{}, err
			}
			resolved.Root = componentRoot
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
			componentRoot, err := resolveContainedWorkspaceDirectory(root, component.SubPath, source.ID, component.Component, "mapped worktree")
			if err != nil {
				return WorkspaceResolution{}, err
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

// resolveContainedWorkspaceDirectory checks both lexical traversal and the
// canonical filesystem path. A lexical Rel check alone accepts a subdirectory
// symlink that points outside its declared source root.
func resolveContainedWorkspaceDirectory(root, subPath, sourceID, component, rootDescription string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := root
	if strings.TrimSpace(subPath) != "" {
		candidate = filepath.Join(root, filepath.FromSlash(subPath))
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if rel, relErr := filepath.Rel(root, candidate); relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("component %q source path escapes %s", component, rootDescription)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	canonicalCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if os.IsNotExist(err) && strings.TrimSpace(subPath) != "" {
			return "", &WorkspaceSourceError{Source: sourceID, Component: component, Path: candidate, Problem: "component subpath is missing inside " + rootDescription}
		}
		return "", err
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalCandidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("component %q source path escapes %s through a symbolic link", component, rootDescription)
	}
	info, err := os.Stat(canonicalCandidate)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("component %q path %s is not a directory", component, canonicalCandidate)
	}
	return canonicalCandidate, nil
}
