package runtime

import (
	"fmt"
	"sort"

	"go.yaml.in/yaml/v3"
)

// ControlPlaneStartupServices reads the actual embedded startup topology, including
// transient admin/bootstrap services. Resource planning must not assume replicas.
func ControlPlaneStartupServices() ([]string, error) {
	var spec struct {
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(composeYAML, &spec); err != nil {
		return nil, fmt.Errorf("decode control-plane startup topology: %w", err)
	}
	if len(spec.Services) == 0 {
		return nil, fmt.Errorf("control-plane startup topology has no services")
	}
	names := make([]string, 0, len(spec.Services))
	for name := range spec.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
