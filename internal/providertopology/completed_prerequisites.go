package providertopology

import (
	"errors"
	"sort"
)

// The caller must already have verified its canonical protected renderer.
// A name or restart policy alone never establishes a successful initializer.
func CompletedPrerequisiteServices(compose string) ([]string, error) {
	services, err := ServiceNames(compose)
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, raw := range services {
		spec, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("provider service definition is invalid")
		}
		dependencies, _ := spec["depends_on"].(map[string]any)
		for service, rawRequirement := range dependencies {
			requirement, ok := rawRequirement.(map[string]any)
			if !ok {
				continue
			}
			if requirement["condition"] != "service_completed_successfully" {
				continue
			}
			prerequisite, ok := services[service].(map[string]any)
			if !ok || prerequisite["restart"] != "no" {
				return nil, errors.New("completed provider prerequisite has no canonical one-shot definition")
			}
			selected[service] = true
		}
	}
	result := []string{}
	for service := range selected {
		result = append(result, service)
	}
	sort.Strings(result)
	return result, nil
}
