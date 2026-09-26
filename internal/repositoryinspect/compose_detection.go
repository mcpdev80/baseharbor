package repositoryinspect

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type composeService struct {
	Name                    string
	Postgres                bool
	Redis                   bool
	ObjectStorage           bool
	AmbiguousInfrastructure bool
	Unresolved              bool
	HasBuild                bool
	HasImage                bool
	HasPorts                bool
	Ports                   []string
	HealthCheck             bool
	DatabaseBootstrap       bool
}

type composeDocument struct {
	Include  any                       `yaml:"include"`
	Services map[string]map[string]any `yaml:"services"`
}

func detectComposeServices(data []byte) ([]composeService, error) {
	var document composeDocument
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode Compose YAML: %w", err)
	}
	if document.Include != nil {
		return nil, fmt.Errorf("Compose include is not yet supported by repository inspection; refusing incomplete service detection")
	}
	if len(document.Services) == 0 {
		return nil, nil
	}

	result := make([]composeService, 0, len(document.Services))
	for name, definition := range document.Services {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, inherited := definition["extends"]; inherited {
			return nil, fmt.Errorf("Compose service %q uses extends, which repository inspection cannot resolve safely yet", name)
		}

		item := composeService{Name: name}
		lowerName := strings.ToLower(name)
		item.Postgres = strings.Contains(lowerName, "postgres") || strings.Contains(lowerName, "postgresql")
		item.Redis = strings.Contains(lowerName, "redis") || strings.Contains(lowerName, "valkey")
		item.ObjectStorage = composeObjectStorageMarker(lowerName)
		item.AmbiguousInfrastructure = !item.Postgres && !item.Redis && !item.ObjectStorage &&
			(composeAmbiguousInfrastructureMarker(lowerName) || composeUnsupportedDatabaseMarker(lowerName))

		if raw, ok := definition["image"]; ok {
			image := strings.TrimSpace(fmt.Sprint(raw))
			if image != "" && image != "<nil>" {
				item.HasImage = true
				lowerImage := strings.ToLower(image)
				item.Postgres = item.Postgres || strings.Contains(lowerImage, "postgres") || strings.Contains(lowerImage, "postgresql")
				item.Redis = item.Redis || strings.Contains(lowerImage, "redis") || strings.Contains(lowerImage, "valkey")
				item.ObjectStorage = item.ObjectStorage || composeObjectStorageMarker(lowerImage)
				if !item.Postgres && !item.Redis && !item.ObjectStorage && composeUnsupportedDatabaseMarker(lowerImage) {
					item.AmbiguousInfrastructure = true
				}
			}
		}
		if raw, ok := definition["build"]; ok && raw != nil {
			item.HasBuild = true
		}
		if raw, ok := definition["ports"]; ok {
			item.Ports = composePortValues(raw)
			item.HasPorts = len(item.Ports) > 0
		}
		if raw, ok := definition["healthcheck"]; ok && raw != nil {
			item.HealthCheck = true
		}
		if raw, ok := definition["volumes"]; ok {
			item.DatabaseBootstrap = composeUsesDatabaseInitDirectory(raw)
		}
		if item.Postgres || item.Redis || item.ObjectStorage {
			item.AmbiguousInfrastructure = false
		}
		if !item.Postgres && !item.Redis && !item.ObjectStorage && !item.AmbiguousInfrastructure && !item.HasBuild && !item.HasImage && !item.HasPorts {
			item.Unresolved = true
		}
		item.Ports = uniqueSorted(item.Ports)
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func composeUnsupportedDatabaseMarker(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, marker := range []string{"mysql", "mariadb"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func composeUsesDatabaseInitDirectory(raw any) bool {
	values, ok := raw.([]any)
	if !ok {
		return false
	}
	isInitTarget := func(target string) bool {
		target = strings.TrimSpace(target)
		return target == "/docker-entrypoint-initdb.d" || strings.HasPrefix(target, "/docker-entrypoint-initdb.d/")
	}
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			parts := strings.Split(typed, ":")
			if len(parts) >= 2 && isInitTarget(parts[1]) {
				return true
			}
		case map[string]any:
			if isInitTarget(fmt.Sprint(typed["target"])) {
				return true
			}
		}
	}
	return false
}

func composePortValues(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	var ports []string
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if port := strings.TrimSpace(typed); port != "" {
				ports = append(ports, port)
			}
		case int:
			ports = append(ports, fmt.Sprint(typed))
		case map[string]any:
			target := strings.TrimSpace(fmt.Sprint(typed["target"]))
			published := strings.TrimSpace(fmt.Sprint(typed["published"]))
			hostIP := strings.TrimSpace(fmt.Sprint(typed["host_ip"]))
			if target == "" || target == "<nil>" {
				continue
			}
			value := target
			if published != "" && published != "<nil>" {
				value = published + ":" + target
				if hostIP != "" && hostIP != "<nil>" {
					value = hostIP + ":" + value
				}
			}
			if protocol := strings.TrimSpace(fmt.Sprint(typed["protocol"])); protocol != "" && protocol != "<nil>" && protocol != "tcp" {
				value += "/" + protocol
			}
			ports = append(ports, value)
		}
	}
	return ports
}
