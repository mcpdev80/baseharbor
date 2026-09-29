package development

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const (
	StackProfileAPIVersion = "baseharbor.dev/v1"
	StackProfileKind       = "StackProfile"
)

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

type ProfileMetadata struct {
	Name string `json:"name" yaml:"name"`
}

type StackProfile struct {
	APIVersion   string                 `json:"api_version" yaml:"apiVersion"`
	Kind         string                 `json:"kind" yaml:"kind"`
	Metadata     ProfileMetadata        `json:"metadata" yaml:"metadata"`
	Extends      []string               `json:"extends,omitempty" yaml:"extends,omitempty"`
	Components   []Component            `json:"components" yaml:"components"`
	Capabilities []CapabilityPreference `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

func (p StackProfile) Validate() error {
	if p.APIVersion != StackProfileAPIVersion {
		return fmt.Errorf("stack profile apiVersion %q is unsupported; expected %q", p.APIVersion, StackProfileAPIVersion)
	}
	if p.Kind != StackProfileKind {
		return fmt.Errorf("stack profile kind %q is unsupported; expected %q", p.Kind, StackProfileKind)
	}
	if strings.TrimSpace(p.Metadata.Name) == "" {
		return fmt.Errorf("stack profile metadata.name is required")
	}
	if len(p.Components) == 0 && len(p.Extends) == 0 {
		return fmt.Errorf("stack profile %q requires at least one development component or parent profile", p.Metadata.Name)
	}
	components := map[string]struct{}{}
	for _, component := range p.Components {
		id := strings.TrimSpace(component.ID)
		if id == "" || strings.TrimSpace(component.Role) == "" || strings.TrimSpace(component.Adapter) == "" {
			return fmt.Errorf("stack profile %q has incomplete component %#v", p.Metadata.Name, component)
		}
		if _, exists := components[id]; exists {
			return fmt.Errorf("stack profile %q component %q is declared more than once", p.Metadata.Name, id)
		}
		components[id] = struct{}{}
	}
	seenCapabilities := map[string]struct{}{}
	for _, preference := range p.Capabilities {
		if preference.Capability == "" {
			return fmt.Errorf("stack profile %q capability preference requires capability", p.Metadata.Name)
		}
		componentIDs := append([]string(nil), preference.Components...)
		sort.Strings(componentIDs)
		key := string(preference.Capability) + ":" + strings.Join(componentIDs, ",")
		if _, exists := seenCapabilities[key]; exists {
			return fmt.Errorf("stack profile %q capability preference %q is declared more than once", p.Metadata.Name, key)
		}
		seenCapabilities[key] = struct{}{}
		for _, component := range preference.Components {
			if _, exists := components[component]; !exists && len(p.Extends) == 0 {
				return fmt.Errorf("stack profile %q capability %q references unknown component %q", p.Metadata.Name, preference.Capability, component)
			}
		}
	}
	return nil
}
