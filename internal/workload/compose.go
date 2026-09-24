package workload

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string             `yaml:"image"`
	Build       composeBuild       `yaml:"build"`
	Command     composeCommand     `yaml:"command"`
	Environment composeEnvironment `yaml:"environment"`
	Ports       []any              `yaml:"ports"`
}

type composeBuild struct {
	Context    string
	Dockerfile string
	Set        bool
}

func (b *composeBuild) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		b.Context = strings.TrimSpace(node.Value)
		b.Set = b.Context != ""
		return nil
	case yaml.MappingNode:
		var value struct {
			Context    string `yaml:"context"`
			Dockerfile string `yaml:"dockerfile"`
		}
		if err := node.Decode(&value); err != nil {
			return err
		}
		b.Context = strings.TrimSpace(value.Context)
		b.Dockerfile = strings.TrimSpace(value.Dockerfile)
		b.Set = b.Context != "" || b.Dockerfile != ""
		return nil
	default:
		return fmt.Errorf("unsupported Compose build value")
	}
}

type composeCommand []string

func (c *composeCommand) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		value := strings.TrimSpace(node.Value)
		if value != "" {
			*c = []string{value}
		}
		return nil
	case yaml.SequenceNode:
		var values []string
		if err := node.Decode(&values); err != nil {
			return err
		}
		*c = values
		return nil
	default:
		return fmt.Errorf("unsupported Compose command value")
	}
}

type composeEnvironment map[string]string

func (e *composeEnvironment) UnmarshalYAML(node *yaml.Node) error {
	values := map[string]string{}
	switch node.Kind {
	case 0:
		*e = values
		return nil
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := strings.TrimSpace(node.Content[i].Value)
			if key == "" {
				continue
			}
			valueNode := node.Content[i+1]
			if valueNode.Tag == "!!null" {
				values[key] = ""
				continue
			}
			var value string
			if err := valueNode.Decode(&value); err != nil {
				return fmt.Errorf("decode Compose environment %s: %w", key, err)
			}
			values[key] = value
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			raw := strings.TrimSpace(item.Value)
			if raw == "" {
				continue
			}
			key, value, ok := strings.Cut(raw, "=")
			if !ok {
				values[raw] = ""
				continue
			}
			values[strings.TrimSpace(key)] = value
		}
	default:
		return fmt.Errorf("unsupported Compose environment value")
	}
	*e = values
	return nil
}

// FromCompose translates a repository-owned Compose source into portable
// workload semantics. selected limits translation to application workload
// services; repository infrastructure may remain present in the source file.
func FromCompose(path string, selected []string) (Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Model{}, fmt.Errorf("read Compose workload: %w", err)
	}
	var source composeFile
	if err := yaml.Unmarshal(data, &source); err != nil {
		return Model{}, fmt.Errorf("parse Compose workload: %w", err)
	}
	if len(source.Services) == 0 {
		return Model{}, errors.New("Compose workload contains no services")
	}

	names := append([]string(nil), selected...)
	if len(names) == 0 {
		for name := range source.Services {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	seen := map[string]struct{}{}
	model := Model{Services: make([]Service, 0, len(names))}
	for _, name := range names {
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		src, ok := source.Services[name]
		if !ok {
			return Model{}, fmt.Errorf("selected workload service %q is not present in Compose source", name)
		}
		service := Service{
			Name:        name,
			Image:       strings.TrimSpace(src.Image),
			Command:     append([]string(nil), src.Command...),
			Environment: cloneStringMap(src.Environment),
		}
		if src.Build.Set {
			context := src.Build.Context
			if context == "" {
				context = "."
			}
			service.Build = &Build{
				Context:    filepath.ToSlash(context),
				Dockerfile: filepath.ToSlash(src.Build.Dockerfile),
			}
		}
		for _, raw := range src.Ports {
			port, ok, err := composeContainerPort(raw)
			if err != nil {
				return Model{}, fmt.Errorf("service %s: %w", name, err)
			}
			if ok {
				service.Ports = append(service.Ports, port)
			}
		}
		model.Services = append(model.Services, service)
	}
	return model, nil
}

func composeContainerPort(raw any) (Port, bool, error) {
	switch value := raw.(type) {
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return Port{}, false, nil
		}
		protocol := "tcp"
		if before, after, ok := strings.Cut(text, "/"); ok {
			text = before
			protocol = strings.ToLower(strings.TrimSpace(after))
		}
		parts := strings.Split(text, ":")
		target := strings.TrimSpace(parts[len(parts)-1])
		target = strings.Trim(target, ""'")
		number, err := strconv.Atoi(target)
		if err != nil || number < 1 || number > 65535 {
			return Port{}, false, fmt.Errorf("unsupported Compose port %q", value)
		}
		return Port{Container: number, Protocol: protocol}, true, nil
	case map[string]any:
		target, ok := value["target"]
		if !ok {
			return Port{}, false, nil
		}
		number, err := integerValue(target)
		if err != nil {
			return Port{}, false, fmt.Errorf("unsupported Compose target port: %w", err)
		}
		protocol := "tcp"
		if rawProtocol, ok := value["protocol"]; ok {
			protocol = strings.ToLower(strings.TrimSpace(fmt.Sprint(rawProtocol)))
		}
		return Port{Container: number, Protocol: protocol}, true, nil
	default:
		// yaml.v3 normally decodes map values as map[string]interface{}, but
		// scalar numeric short syntax is also valid for a container-only port.
		number, err := integerValue(value)
		if err != nil {
			return Port{}, false, fmt.Errorf("unsupported Compose port value %v", raw)
		}
		return Port{Container: number, Protocol: "tcp"}, true, nil
	}
}

func integerValue(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case uint64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(v))
	default:
		return 0, fmt.Errorf("%T", value)
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
