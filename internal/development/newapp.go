package development

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"go.yaml.in/yaml/v3"
)

type NewApplicationRequest struct {
	ApplicationID          string            `json:"-"`
	Name                   string            `json:"name"`
	Environment            string            `json:"environment,omitempty"`
	Adapter                string            `json:"adapter,omitempty"`
	Profile                *StackProfile     `json:"profile,omitempty"`
	Capabilities           []capability.Kind `json:"capabilities"`
	Secrets                []string          `json:"secrets,omitempty"`
	EmitBackstage          bool              `json:"emit_backstage,omitempty"`
	BackstageOwner         string            `json:"backstage_owner,omitempty"`
	BackstageLifecycle     string            `json:"backstage_lifecycle,omitempty"`
	RepositoryStackProfile *StackProfile     `json:"repository_stack_profile,omitempty"`
}

type BootstrapResult struct {
	Manifest  application.Manifest         `json:"-"`
	Contract  application.PortableContract `json:"-"`
	Profile   StackProfile                 `json:"profile"`
	Plan      DevelopmentPlan              `json:"development_plan"`
	Files     []GeneratedFile              `json:"-"`
	FilePaths []string                     `json:"files"`
}

func BootstrapApplication(request NewApplicationRequest, registry Registry) (BootstrapResult, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return BootstrapResult{}, fmt.Errorf("application name is required")
	}
	environment := strings.TrimSpace(request.Environment)
	if environment == "" {
		environment = "dev"
	}
	var profile StackProfile
	if request.Profile != nil {
		profile = *request.Profile
		if err := profile.Validate(); err != nil {
			return BootstrapResult{}, fmt.Errorf("stack profile: %w", err)
		}
	} else {
		adapterID := strings.TrimSpace(request.Adapter)
		if adapterID == "" {
			return BootstrapResult{}, fmt.Errorf("development adapter or stack profile is required")
		}
		if _, err := registry.Resolve(adapterID); err != nil {
			return BootstrapResult{}, err
		}
		profile = StackProfile{
			APIVersion: StackProfileAPIVersion,
			Kind:       StackProfileKind,
			Metadata:   ProfileMetadata{Name: "generated/" + strings.TrimPrefix(adapterID, "development/")},
			Components: []Component{{
				ID:      "app",
				Role:    "application",
				Adapter: adapterID,
			}},
		}
	}

	applicationID := strings.TrimSpace(request.ApplicationID)
	if applicationID == "" {
		applicationID = application.MustNewApplicationID()
	} else if err := application.ValidateApplicationID(applicationID); err != nil {
		return BootstrapResult{}, err
	}
	manifest := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: applicationID,
		Name:          name,
		Environment:   environment,
		Workload: application.WorkloadConfig{
			Compose:  "compose.yaml",
			Services: profileComponentIDs(profile),
		},
	}
	seen := map[capability.Kind]struct{}{}
	for _, kind := range request.Capabilities {
		if _, exists := seen[kind]; exists {
			continue
		}
		seen[kind] = struct{}{}
		switch kind {
		case capability.ExposureHTTP:
			manifest = application.WithHTTPExposure(manifest, "web", profileCapabilityTarget(profile, capability.ExposureHTTP), 8080, "http")
		case capability.SQL:
			manifest.Services.SQL = true
		case capability.KeyValue:
			manifest.Services.Cache = true
		case capability.ObjectStorageS3:
			manifest.Services.ObjectStorage = true
		case capability.Secrets:
			secrets := append([]string(nil), request.Secrets...)
			if len(secrets) == 0 {
				secrets = []string{"APP_SECRET"}
			}
			manifest = application.WithRequiredSecrets(manifest, secrets...)
		case capability.TelemetryOTLP:
			manifest = application.WithOTLPTelemetry(manifest, "traces")
		default:
			return BootstrapResult{}, fmt.Errorf("greenfield capability %q is not supported yet", kind)
		}
	}
	if err := manifest.Validate(); err != nil {
		return BootstrapResult{}, fmt.Errorf("greenfield application contract: %w", err)
	}
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil {
		return BootstrapResult{}, err
	}
	for _, requirement := range contract.Capabilities {
		if err := validateProfileCapabilitySupport(profile, requirement, registry); err != nil {
			return BootstrapResult{}, err
		}
	}

	plan, err := BuildPlan(contract, profile, registry)
	if err != nil {
		return BootstrapResult{}, err
	}
	files, err := bootstrapProfileFiles(plan, profile, registry)
	if err != nil {
		return BootstrapResult{}, err
	}
	files = append(files, GeneratedFile{
		Path:    application.RepositoryManifestName,
		Content: []byte(manifest.YAML()),
		Mode:    0o644,
	})
	if request.RepositoryStackProfile != nil {
		reusable := *request.RepositoryStackProfile
		if err := reusable.Validate(); err != nil {
			return BootstrapResult{}, fmt.Errorf("repository stack profile: %w", err)
		}
		data, err := yaml.Marshal(reusable)
		if err != nil {
			return BootstrapResult{}, fmt.Errorf("encode repository stack profile: %w", err)
		}
		files = append(files, GeneratedFile{
			Path:    filepath.ToSlash(filepath.Join(".baseharbor", "stacks", safeProfileFilename(reusable.Metadata.Name)+".yaml")),
			Content: data,
			Mode:    0o644,
		})
	}
	if request.EmitBackstage {
		catalog, err := RenderBackstageCatalog(manifest, BackstageCatalogOptions{
			Owner:     request.BackstageOwner,
			Lifecycle: request.BackstageLifecycle,
		})
		if err != nil {
			return BootstrapResult{}, err
		}
		files = append(files, GeneratedFile{
			Path:    "catalog-info.yaml",
			Content: []byte(catalog),
			Mode:    0o644,
		})
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return BootstrapResult{
		Manifest:  manifest,
		Contract:  contract,
		Profile:   profile,
		Plan:      plan,
		Files:     files,
		FilePaths: paths,
	}, nil
}

type CreationResult struct {
	BootstrapResult
	Validation Validation `json:"validation"`
}

func CreateApplication(root string, request NewApplicationRequest, registry Registry) (CreationResult, error) {
	bootstrap, err := BootstrapApplication(request, registry)
	if err != nil {
		return CreationResult{}, err
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	var extras []GeneratedFile
	for _, file := range bootstrap.Files {
		if file.Path == "catalog-info.yaml" || strings.HasPrefix(file.Path, ".baseharbor/stacks/") {
			extras = append(extras, file)
		}
	}
	project, err := BootstrapProject(root, bootstrap.Manifest, bootstrap.Profile, registry, extras...)
	if err != nil {
		return CreationResult{}, err
	}
	bootstrap.Plan = project.Plan
	bootstrap.FilePaths = append([]string(nil), project.Files...)
	validation := Validation{Satisfied: project.Satisfied, Capabilities: map[capability.Kind]bool{}}
	for _, componentValidation := range project.Validations {
		for kind, satisfied := range componentValidation.Capabilities {
			if current, exists := validation.Capabilities[kind]; !exists || satisfied {
				validation.Capabilities[kind] = current || satisfied
			}
		}
		validation.Diagnostics = append(validation.Diagnostics, componentValidation.Diagnostics...)
	}
	return CreationResult{BootstrapResult: bootstrap, Validation: validation}, nil
}

func WriteGeneratedFiles(root string, files []GeneratedFile) error {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Clean(file.Path)
		if path == "." || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return fmt.Errorf("generated file path %q escapes project root", file.Path)
		}
		target := filepath.Join(absRoot, path)
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("generated file %s already exists", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, file := range files {
		target := filepath.Join(absRoot, filepath.Clean(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, file.Content, mode); err != nil {
			return fmt.Errorf("write generated file %s: %w", file.Path, err)
		}
	}
	return nil
}

func profileComponentIDs(profile StackProfile) []string {
	ids := make([]string, 0, len(profile.Components))
	for _, component := range profile.Components {
		ids = append(ids, component.ID)
	}
	return ids
}

func validateProfileCapabilitySupport(profile StackProfile, requirement capability.Requirement, registry Registry) error {
	var preferred []string
	for _, preference := range profile.Capabilities {
		if preference.Capability == requirement.Kind {
			preferred = append(preferred, preference.Components...)
		}
	}
	if len(preferred) > 0 {
		for _, componentID := range preferred {
			for _, component := range profile.Components {
				if component.ID != componentID {
					continue
				}
				adapter, err := registry.Resolve(component.Adapter)
				if err != nil {
					return err
				}
				if !adapter.Supports(requirement) {
					return fmt.Errorf("stack profile component %q adapter %q does not support %s", component.ID, component.Adapter, requirement.Kind)
				}
			}
		}
		return nil
	}
	var supporting []string
	for _, component := range profile.Components {
		adapter, err := registry.Resolve(component.Adapter)
		if err != nil {
			return err
		}
		if adapter.Supports(requirement) {
			supporting = append(supporting, component.ID)
		}
	}
	if len(supporting) == 0 {
		return fmt.Errorf("stack profile %q has no component that supports %s", profile.Metadata.Name, requirement.Kind)
	}
	if len(supporting) > 1 {
		return fmt.Errorf("stack profile %q has ambiguous placement for %s across components %s; declare capability.components explicitly", profile.Metadata.Name, requirement.Kind, strings.Join(supporting, ", "))
	}
	return nil
}

func profileCapabilityTarget(profile StackProfile, kind capability.Kind) string {
	for _, preference := range profile.Capabilities {
		if preference.Capability == kind && len(preference.Components) > 0 {
			return preference.Components[0]
		}
	}
	if len(profile.Components) > 0 {
		return profile.Components[0].ID
	}
	return "app"
}

func bootstrapProfileFiles(plan DevelopmentPlan, profile StackProfile, registry Registry) ([]GeneratedFile, error) {
	multiComponent := len(profile.Components) > 1
	componentComposes := map[string][]byte{}
	var files []GeneratedFile
	seen := map[string]struct{}{}
	for _, component := range profile.Components {
		adapter, err := registry.Resolve(component.Adapter)
		if err != nil {
			return nil, err
		}
		generated, err := adapter.Bootstrap(plan, component)
		if err != nil {
			return nil, fmt.Errorf("bootstrap component %q: %w", component.ID, err)
		}
		for _, file := range generated {
			path, err := cleanProjectPath(file.Path)
			if err != nil {
				return nil, fmt.Errorf("component %q: %w", component.ID, err)
			}
			if multiComponent {
				if path == "compose.yaml" {
					componentComposes[component.ID] = append([]byte(nil), file.Content...)
					continue
				}
				path = filepath.ToSlash(filepath.Join(component.ID, path))
			}
			if _, exists := seen[path]; exists {
				return nil, fmt.Errorf("generated file collision at %q", path)
			}
			seen[path] = struct{}{}
			file.Path = path
			files = append(files, file)
		}
	}
	if multiComponent {
		compose, err := renderMultiComponentCompose(profile, componentComposes)
		if err != nil {
			return nil, err
		}
		files = append(files, GeneratedFile{Path: "compose.yaml", Content: compose, Mode: 0o644})
	}
	return files, nil
}
