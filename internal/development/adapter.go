package development

import (
	"fmt"
	"sort"
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
	Satisfied    bool                     `json:"satisfied"`
	Capabilities map[capability.Kind]bool `json:"capabilities,omitempty"`
	Diagnostics  []string                 `json:"diagnostics,omitempty"`
}

type Adapter interface {
	Descriptor() extension.Metadata
	Detect(root string) (Detection, error)
	Supports(requirement capability.Requirement) bool
	Plan(contract application.PortableContract, profile StackProfile, component Component) ([]Action, error)
	Bootstrap(plan DevelopmentPlan, component Component) ([]GeneratedFile, error)
	Validate(root string, contract application.PortableContract, component Component) (Validation, error)
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


func (r Registry) IDs() []string {
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r Registry) Supports(id string, kind capability.Kind) bool {
	adapter, ok := r.adapters[strings.TrimSpace(id)]
	if !ok {
		return false
	}
	return adapter.Supports(capability.Requirement{Kind: kind})
}
