package development

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

type NewApplicationRequest struct {
	Name         string            `json:"name"`
	Environment  string            `json:"environment,omitempty"`
	Adapter      string            `json:"adapter"`
	Capabilities []capability.Kind `json:"capabilities"`
	Secrets      []string          `json:"secrets,omitempty"`
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
	adapterID := strings.TrimSpace(request.Adapter)
	if adapterID == "" {
		return BootstrapResult{}, fmt.Errorf("development adapter is required")
	}
	adapter, err := registry.Resolve(adapterID)
	if err != nil {
		return BootstrapResult{}, err
	}

	manifest := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        name,
		Environment: environment,
		Workload: application.WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"app"},
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
			manifest = application.WithHTTPExposure(manifest, "web", "app", 8080, "http")
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
		if !adapter.Supports(requirement) {
			return BootstrapResult{}, fmt.Errorf("development adapter %q does not support %s", adapterID, requirement.Kind)
		}
	}

	profile := StackProfile{
		SchemaVersion: StackProfileVersion,
		Name:          "generated/" + strings.TrimPrefix(adapterID, "development/"),
		Components: []Component{{
			ID:      "app",
			Role:    "application",
			Adapter: adapterID,
		}},
	}
	plan, err := BuildPlan(contract, profile, registry)
	if err != nil {
		return BootstrapResult{}, err
	}
	files, err := adapter.Bootstrap(plan, profile.Components[0])
	if err != nil {
		return BootstrapResult{}, err
	}
	files = append(files, GeneratedFile{
		Path:    application.RepositoryManifestName,
		Content: []byte(manifest.YAML()),
		Mode:    0o644,
	})
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
	if err := WriteGeneratedFiles(root, bootstrap.Files); err != nil {
		return CreationResult{}, err
	}

	cleanup := true
	defer func() {
		if !cleanup {
			return
		}
		for _, file := range bootstrap.Files {
			_ = os.Remove(filepath.Join(root, filepath.Clean(file.Path)))
		}
	}()

	adapter, err := registry.Resolve(bootstrap.Profile.Components[0].Adapter)
	if err != nil {
		return CreationResult{}, err
	}
	validation, err := adapter.Validate(root, bootstrap.Contract, bootstrap.Profile.Components[0])
	if err != nil {
		return CreationResult{}, fmt.Errorf("validate generated application: %w", err)
	}
	if !validation.Satisfied {
		return CreationResult{}, fmt.Errorf("generated application does not satisfy its contract: %s", strings.Join(validation.Diagnostics, "; "))
	}
	cleanup = false
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
