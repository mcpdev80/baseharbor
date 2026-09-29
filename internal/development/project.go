package development

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
	"go.yaml.in/yaml/v3"
)

const NewApplicationResultVersion = "baseharbor.app-new/v1"

type ProjectResult struct {
	SchemaVersion string                   `json:"schema_version"`
	Root          string                   `json:"root"`
	Application   string                   `json:"application"`
	Profile       string                   `json:"profile"`
	Plan          DevelopmentPlan          `json:"development_plan"`
	Files         []string                 `json:"files"`
	Inspection    repositoryinspect.Result `json:"inspection"`
	Satisfied     bool                     `json:"satisfied"`
	Validations   map[string]Validation    `json:"validations,omitempty"`
}

func BootstrapProject(root string, manifest application.Manifest, profile StackProfile, registry Registry, extras ...GeneratedFile) (ProjectResult, error) {
	if err := manifest.Validate(); err != nil {
		return ProjectResult{}, fmt.Errorf("application contract: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return ProjectResult{}, err
	}
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil {
		return ProjectResult{}, err
	}
	plan, err := BuildPlan(contract, profile, registry)
	if err != nil {
		return ProjectResult{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("resolve project root: %w", err)
	}
	if err := ensureEmptyProjectRoot(root); err != nil {
		return ProjectResult{}, err
	}

	generated := map[string]GeneratedFile{}
	multiComponent := len(profile.Components) > 1
	componentComposes := map[string][]byte{}
	for _, extra := range extras {
		path, err := cleanProjectPath(extra.Path)
		if err != nil {
			return ProjectResult{}, fmt.Errorf("extra generated artifact: %w", err)
		}
		if path == application.RepositoryManifestName || path == ".baseharbor/stack-profile.yaml" || path == ".baseharbor/development-plan.json" {
			return ProjectResult{}, fmt.Errorf("extra generated artifact %q collides with BaseHarbor-owned bootstrap metadata", path)
		}
		if _, exists := generated[path]; exists {
			return ProjectResult{}, fmt.Errorf("generated file collision at %q", path)
		}
		extra.Path = path
		generated[path] = extra
	}

	for _, component := range profile.Components {
		adapter, err := registry.Resolve(component.Adapter)
		if err != nil {
			return ProjectResult{}, err
		}
		files, err := adapter.Bootstrap(plan, component)
		if err != nil {
			return ProjectResult{}, fmt.Errorf("bootstrap component %q: %w", component.ID, err)
		}
		for _, file := range files {
			path, err := cleanProjectPath(file.Path)
			if err != nil {
				return ProjectResult{}, fmt.Errorf("component %q: %w", component.ID, err)
			}
			if multiComponent {
				if path == "compose.yaml" {
					componentComposes[component.ID] = append([]byte(nil), file.Content...)
					continue
				}
				path = filepath.ToSlash(filepath.Join(component.ID, path))
			}
			if _, exists := generated[path]; exists {
				return ProjectResult{}, fmt.Errorf("generated file collision at %q", path)
			}
			file.Path = path
			generated[path] = file
		}
	}
	if multiComponent {
		compose, err := renderMultiComponentCompose(profile, componentComposes)
		if err != nil {
			return ProjectResult{}, err
		}
		generated["compose.yaml"] = GeneratedFile{Path: "compose.yaml", Content: compose, Mode: 0o644}
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return ProjectResult{}, err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = os.RemoveAll(root)
		}
	}()

	var files []string
	write := func(rel string, data []byte, mode os.FileMode) error {
		path, err := cleanProjectPath(rel)
		if err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return err
		}
		files = append(files, path)
		return nil
	}

	if err := write(application.RepositoryManifestName, []byte(manifest.YAML()), 0o644); err != nil {
		return ProjectResult{}, err
	}
	profileYAML, err := yaml.Marshal(profile)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("encode stack profile: %w", err)
	}
	if err := write(".baseharbor/stack-profile.yaml", profileYAML, 0o644); err != nil {
		return ProjectResult{}, err
	}
	planJSON, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return ProjectResult{}, fmt.Errorf("encode development plan: %w", err)
	}
	planJSON = append(planJSON, '\n')
	if err := write(".baseharbor/development-plan.json", planJSON, 0o644); err != nil {
		return ProjectResult{}, err
	}
	for _, rel := range sortedGeneratedPaths(generated) {
		file := generated[rel]
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := write(rel, file.Content, mode); err != nil {
			return ProjectResult{}, err
		}
	}

	inspection, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("inspect generated project: %w", err)
	}
	satisfied := allDeclaredCapabilitiesSatisfied(inspection)
	validations := make(map[string]Validation, len(profile.Components))
	for _, component := range profile.Components {
		adapter, err := registry.Resolve(component.Adapter)
		if err != nil {
			return ProjectResult{}, err
		}
		componentContract := contractForComponent(contract, profile, component.ID)
		validation, err := adapter.Validate(root, componentContract, component)
		if err != nil {
			return ProjectResult{}, fmt.Errorf("validate component %q: %w", component.ID, err)
		}
		validations[component.ID] = validation
		if !validation.Satisfied {
			satisfied = false
		}
	}
	result := ProjectResult{
		SchemaVersion: NewApplicationResultVersion,
		Root:          root,
		Application:   manifest.Name,
		Profile:       profile.Metadata.Name,
		Plan:          plan,
		Files:         files,
		Inspection:    inspection,
		Satisfied:     satisfied,
		Validations:   validations,
	}
	if !result.Satisfied {
		return ProjectResult{}, fmt.Errorf("generated project does not satisfy declared application contract")
	}
	sort.Strings(result.Files)
	rollback = false
	return result, nil
}

func ensureEmptyProjectRoot(root string) error {
	entries, err := os.ReadDir(root)
	if err == nil {
		if len(entries) != 0 {
			return fmt.Errorf("project directory %s is not empty", root)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cleanProjectPath(path string) (string, error) {
	path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if path == "" || path == "." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
		return "", fmt.Errorf("generated project path %q is invalid", path)
	}
	return path, nil
}

func sortedGeneratedPaths(files map[string]GeneratedFile) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func allDeclaredCapabilitiesSatisfied(result repositoryinspect.Result) bool {
	if len(result.Declared) == 0 {
		return false
	}
	for _, declared := range result.Declared {
		found := false
		for _, item := range result.Reconciliation {
			if item.Capability != declared.Capability {
				continue
			}
			if item.Name != "" && declared.Name != "" && item.Name != declared.Name {
				continue
			}
			if item.State == repositoryinspect.ReconciliationSatisfied {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func renderMultiComponentCompose(profile StackProfile, sources map[string][]byte) ([]byte, error) {
	services := map[string]any{}
	for _, component := range profile.Components {
		data, ok := sources[component.ID]
		if !ok {
			return nil, fmt.Errorf("component %q adapter did not generate compose.yaml", component.ID)
		}
		var document map[string]any
		if err := yaml.Unmarshal(data, &document); err != nil {
			return nil, fmt.Errorf("parse component %q compose.yaml: %w", component.ID, err)
		}
		rawServices, ok := document["services"].(map[string]any)
		if !ok || len(rawServices) == 0 {
			return nil, fmt.Errorf("component %q compose.yaml has no services", component.ID)
		}
		var service map[string]any
		if candidate, ok := rawServices["app"].(map[string]any); ok {
			service = candidate
		} else if len(rawServices) == 1 {
			for _, candidate := range rawServices {
				service, _ = candidate.(map[string]any)
			}
		}
		if service == nil {
			return nil, fmt.Errorf("component %q compose.yaml must expose one application service", component.ID)
		}
		service = cloneAnyMap(service)
		switch build := service["build"].(type) {
		case string:
			if strings.TrimSpace(build) == "." {
				service["build"] = "./" + component.ID
			}
		case map[string]any:
			build = cloneAnyMap(build)
			if context, ok := build["context"].(string); ok && strings.TrimSpace(context) == "." {
				build["context"] = "./" + component.ID
			}
			service["build"] = build
		}
		if !profileCapabilityAppliesToComponent(profile, capability.ExposureHTTP, component.ID) {
			delete(service, "ports")
		}
		services[component.ID] = service
	}
	return yaml.Marshal(map[string]any{"services": services})
}

func cloneAnyMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
