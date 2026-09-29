package development

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"go.yaml.in/yaml/v3"
)

const (
	BackstageAPIVersion       = "backstage.io/v1alpha1"
	BackstageKind             = "Component"
	DefaultBackstageLifecycle = "experimental"
)

type BackstageCatalogOptions struct {
	Owner     string
	Lifecycle string
}

type backstageCatalog struct {
	APIVersion string                   `yaml:"apiVersion"`
	Kind       string                   `yaml:"kind"`
	Metadata   backstageCatalogMetadata `yaml:"metadata"`
	Spec       backstageCatalogSpec     `yaml:"spec"`
}

type backstageCatalogMetadata struct {
	Name        string            `yaml:"name"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type backstageCatalogSpec struct {
	Type      string `yaml:"type"`
	Lifecycle string `yaml:"lifecycle"`
	Owner     string `yaml:"owner"`
}

func RenderBackstageCatalog(manifest application.Manifest, options BackstageCatalogOptions) (string, error) {
	owner := strings.TrimSpace(options.Owner)
	if owner == "" {
		return "", fmt.Errorf("Backstage owner is required when catalog emission is enabled")
	}
	lifecycle := strings.TrimSpace(options.Lifecycle)
	if lifecycle == "" {
		lifecycle = DefaultBackstageLifecycle
	}
	catalog := backstageCatalog{
		APIVersion: BackstageAPIVersion,
		Kind:       BackstageKind,
		Metadata: backstageCatalogMetadata{
			Name: manifest.Name,
			Annotations: map[string]string{
				"baseharbor.dev/application-contract": application.RepositoryManifestName,
			},
		},
		Spec: backstageCatalogSpec{
			Type:      "service",
			Lifecycle: lifecycle,
			Owner:     owner,
		},
	}
	data, err := yaml.Marshal(catalog)
	if err != nil {
		return "", fmt.Errorf("encode Backstage catalog metadata: %w", err)
	}
	return string(data), nil
}
