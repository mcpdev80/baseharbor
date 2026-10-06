package availability

import (
	"fmt"
	"sort"
	"strings"
)

type Override struct {
	HA        *bool `json:"ha,omitempty"`
	Instances int   `json:"instances,omitempty"`
}

type Requirement struct {
	Component         string `json:"component"`
	HA                bool   `json:"ha"`
	Instances         int    `json:"instances,omitempty"`
	ExplicitException bool   `json:"explicit_exception,omitempty"`
}

type Intent struct {
	HA        bool                `json:"ha"`
	Overrides map[string]Override `json:"overrides,omitempty"`
}

func (i Intent) Resolve(component string) Requirement {
	component = strings.TrimSpace(component)
	req := Requirement{Component: component, HA: i.HA}
	if o, ok := i.Overrides[component]; ok {
		if o.HA != nil {
			req.HA = *o.HA
			req.ExplicitException = i.HA && !*o.HA
		}
		req.Instances = o.Instances
	}
	return req
}

func (i Intent) Validate() error {
	for component, override := range i.Overrides {
		if strings.TrimSpace(component) == "" {
			return fmt.Errorf("availability override component is required")
		}
		if override.Instances < 0 || override.Instances == 1 && override.HA != nil && *override.HA {
			return fmt.Errorf("availability override %q has invalid HA instance count %d", component, override.Instances)
		}
		if override.Instances > 0 && override.Instances < 2 && effectiveHA(i.HA, override) {
			return fmt.Errorf("availability override %q requires at least 2 instances when HA is enabled", component)
		}
	}
	return nil
}

func effectiveHA(global bool, override Override) bool {
	if override.HA != nil {
		return *override.HA
	}
	return global
}

func (i Intent) Components() []string {
	out := make([]string, 0, len(i.Overrides))
	for component := range i.Overrides {
		out = append(out, component)
	}
	sort.Strings(out)
	return out
}
