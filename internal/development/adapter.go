package development

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

type Detection struct {
	Detected bool     `json:"detected"`
	Evidence []string `json:"evidence,omitempty"`
}

type GeneratedFile struct {
	Path    string `json:"path"`
	Content []byte `json:"-"`
	Mode    uint32 `json:"mode,omitempty"`
}

type Validation struct {
	Satisfied   bool              `json:"satisfied"`
	Capabilities map[capability.Kind]bool `json:"capabilities,omitempty"`
	Diagnostics []string          `json:"diagnostics,omitempty"`
}

type Adapter interface {
	Descriptor() extension.Metadata
	Detect(root string) (Detection, error)
	Supports(capability.Requirement) bool
	Plan(application.PortableContract, StackProfile, Component) ([]Action, error)
	Bootstrap(DevelopmentPlan, Component) ([]GeneratedFile, error)
	Validate(root string, application.PortableContract, Component) (Validation, error)
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) (Registry, error) {
	registry := Registry{adapters: make(map[string]Adapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil {
			return Registry{}, fmt.Errorf("development adapter is required")
		}
		descriptor := adapter.Descriptor()
		if err := descriptor.Validate(); err != nil {
			return Registry{}, fmt.Errorf("development adapter descriptor: %w", err)
		}
		if descriptor.Family != extension.FamilyDevelopment {
			return Registry{}, fmt.Errorf("extension %q is not a development adapter", descriptor.ID)
		}
		if _, exists := registry.adapters[descriptor.ID]; exists {
			return Registry{}, fmt.Errorf("development adapter %q is registered more than once", descriptor.ID)
		}
		registry.adapters[descriptor.ID] = adapter
	}
	return registry, nil
}

func (r Registry) Resolve(id string) (Adapter, error) {
	id = strings.TrimSpace(id)
	adapter, ok := r.adapters[id]
	if !ok {
		return nil, fmt.Errorf("development adapter %q is not registered", id)
	}
	return adapter, nil
}
