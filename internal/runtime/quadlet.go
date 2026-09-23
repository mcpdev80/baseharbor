package runtime

import (
	"errors"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type composeBuild struct {
	Context    string
	Dockerfile string
}

func (b *composeBuild) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		b.Context = node.Value
		return nil
	case yaml.MappingNode:
		type plain composeBuild
		var value plain
		if err := node.Decode(&value); err != nil {
			return err
		}
		*b = composeBuild(value)
		return nil
	case 0:
		return nil
	default:
		return errors.New("unsupported Compose build syntax")
	}
}

type composeEnv map[string]string

func (e *composeEnv) UnmarshalYAML(node *yaml.Node) error {
	result := map[string]string{}
	switch node.Kind {
	case 0:
		*e = result
		return nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := strings.TrimSpace(node.Content[i].Value)
			if key == "" {
				continue
			}
			var value string
			if node.Content[i+1].Tag == "!!null" {
				value = os.Getenv(key)
			} else {
				value = node.Content[i+1].Value
			}
			result[key] = value
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			key, value, found := strings.Cut(item.Value, "=")
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if !found {
				value = os.Getenv(key)
			}
			result[key] = value
		}
	default:
		return errors.New("unsupported Compose environment syntax")
	}
	*e = result
	return nil
}

var safeUnitName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func sanitizeQuadletName(value string) string {
	value = safeUnitName.ReplaceAllString(strings.TrimSpace(value), "-")
	return strings.Trim(value, "-.")
}

func systemdEscapeValue(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}
