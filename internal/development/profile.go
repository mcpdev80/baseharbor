package development

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const StackProfileVersion = "baseharbor.stack-profile/v1"

type Component struct {
	ID      string `json:"id" yaml:"id"`
	Role    string `json:"role" yaml:"role"`
	Adapter string `json:"adapter" yaml:"adapter"`
}

type CapabilityPreference struct {
	Capability               capability.Kind `json:"capability" yaml:"capability"`
	Components               []string        `json:"components,omitempty" yaml:"components,omitempty"`
	ImplementationPreference string          `json:"implementation_preference,omitempty" yaml:"implementation-preference,omitempty"`
	DevelopmentIntegration   string          `json:"development_integration,omitempty" yaml:"development-integration,omitempty"`
}

type StackProfile struct {
	SchemaVersion string                 `json:"schema_version" yaml:"apiVersion"`
	Name          string                 `json:"name" yaml:"name"`
	Extends       []string               `json:"extends,omitempty" yaml:"extends,omitempty"`
	Components    []Component            `json:"components" yaml:"components"`
	Capabilities  []CapabilityPreference `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

func (p StackProfile) Validate() error {
	if p.SchemaVersion != StackProfileVersion {
		return fmt.Errorf("stack profile schema version %q is unsupported; expected %q", p.SchemaVersion, StackProfileVersion)
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("stack profile name is required")
	}
	if len(p.Components) == 0 {
		return fmt.Errorf("stack profile %q requires at least one development component", p.Name)
	}
	components := map[string]struct{}{}
	for _, component := range p.Components {
		id := strings.TrimSpace(component.ID)
		if id == "" || strings.TrimSpace(component.Role) == "" || strings.TrimSpace(component.Adapter) == "" {
			return fmt.Errorf("stack profile %q has incomplete component %#v", p.Name, component)
		}
		if _, exists := components[id]; exists {
			return fmt.Errorf("stack profile %q component %q is declared more than once", p.Name, id)
		}
		components[id] = struct{}{}
	}
	seenCapabilities := map[string]struct{}{}
	for _, preference := range p.Capabilities {
		if preference.Capability == "" {
			return fmt.Errorf("stack profile %q capability preference requires capability", p.Name)
		}
		componentIDs := append([]string(nil), preference.Components...)
		sort.Strings(componentIDs)
		key := string(preference.Capability) + ":" + strings.Join(componentIDs, ",")
		if _, exists := seenCapabilities[key]; exists {
			return fmt.Errorf("stack profile %q capability preference %q is declared more than once", p.Name, key)
		}
		seenCapabilities[key] = struct{}{}
		for _, component := range preference.Components {
			if _, exists := components[component]; !exists {
				return fmt.Errorf("stack profile %q capability %q references unknown component %q", p.Name, preference.Capability, component)
			}
		}
	}
	return nil
}
