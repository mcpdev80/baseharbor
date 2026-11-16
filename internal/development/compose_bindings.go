package development

import (
	"fmt"
	"regexp"

	"go.yaml.in/yaml/v3"
)

var composeBindingName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Generated source and Compose must consume the same normal runtime bindings.
// These are placeholders, never secret payloads. Core resolves managed values
// before workload convergence; *_FILE bindings contain only protected paths.
func ComposeWithBindings(source string, plan DevelopmentPlan, component Component) ([]byte, error) {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		return nil, err
	}
	services, ok := document["services"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("generated Compose has no services")
	}
	service, ok := services["app"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("generated Compose has no application service")
	}
	environment, _ := service["environment"].(map[string]any)
	if environment == nil {
		environment = map[string]any{}
	}
	for _, action := range plan.Actions {
		if action.Component != component.ID || action.Kind != ActionBinding {
			continue
		}
		if !composeBindingName.MatchString(action.Name) {
			return nil, fmt.Errorf("runtime binding %q requires an environment-compatible name", action.Name)
		}
		value := action.Value
		if value == "" {
			value = "${" + action.Name + "}"
		}
		environment[action.Name] = value
	}
	if len(environment) > 0 {
		service["environment"] = environment
	}
	return yaml.Marshal(document)
}
