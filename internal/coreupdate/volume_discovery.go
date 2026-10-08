package coreupdate

import (
	"errors"
	"fmt"
	"go.yaml.in/yaml/v3"
	"os"
	"strings"
)

// ResolveOwnedServiceVolume validates the actual Compose service data mount.
// No guessed named volume or host bind directory is accepted for recovery.
func ResolveOwnedServiceVolume(path, service, project string) (string, error) {
	if path == "" || service == "" || project == "" {
		return "", errors.New("missing provider Compose identity")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var root struct {
		Services map[string]struct {
			Volumes []yaml.Node `yaml:"volumes"`
		} `yaml:"services"`
		Volumes map[string]yaml.Node `yaml:"volumes"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return "", fmt.Errorf("parse protected Compose volumes: %w", err)
	}
	spec, ok := root.Services[service]
	if !ok {
		return "", fmt.Errorf("provider service %s missing", service)
	}
	found := ""
	for _, entry := range spec.Volumes {
		if entry.Kind != yaml.ScalarNode {
			return "", errors.New("unsupported complex provider volume mapping")
		}
		source, destination, ok := strings.Cut(entry.Value, ":")
		if !ok || source == "" || destination == "" {
			return "", errors.New("ambiguous provider mount")
		}
		destination = strings.SplitN(destination, ":", 2)[0]
		durable := destination == "/var/lib/postgresql" || destination == "/var/lib/postgresql/data" || destination == "/home/postgres/pgroot" || destination == "/openbao/file" || destination == "/vault/file"
		if !durable {
			continue
		}
		if strings.ContainsAny(source, "/\\") || strings.HasPrefix(source, ".") {
			return "", fmt.Errorf("provider %s uses bind-backed data requiring provider-native backup", service)
		}
		declared, declaredOK := root.Volumes[source]
		if !declaredOK {
			return "", fmt.Errorf("provider %s data volume %s is not declared", service, source)
		}
		actual := project + "_" + source
		if declared.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(declared.Content); i += 2 {
				key, value := declared.Content[i].Value, declared.Content[i+1]
				if key == "external" && value.Value != "false" {
					return "", fmt.Errorf("provider %s uses foreign external data volume %s", service, source)
				}
				if key == "name" {
					if value.Kind != yaml.ScalarNode || strings.TrimSpace(value.Value) == "" || strings.Contains(value.Value, "$") {
						return "", fmt.Errorf("provider %s has unverified data volume name", service)
					}
					actual = value.Value
				}
			}
		} else if declared.Kind != yaml.ScalarNode || (declared.Tag != "!!null" && declared.Value != "") {
			return "", fmt.Errorf("provider %s data volume declaration is not verifiable", service)
		}
		if found != "" {
			return "", fmt.Errorf("provider %s has ambiguous multiple data volumes", service)
		}
		found = actual
	}
	if found == "" {
		return "", fmt.Errorf("provider %s has no verified persistent data volume", service)
	}
	return found, nil
}
