package repositoryinspect

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
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
	WorkloadProtocol        string
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
		if raw, ok := definition["labels"]; ok {
			protocol, err := composeBaseHarborWorkloadProtocol(raw)
			if err != nil {
				return nil, fmt.Errorf("Compose service %q: %w", name, err)
			}
			item.WorkloadProtocol = protocol
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

type ReclaimableComposeVolume struct {
	LogicalName string
	RuntimeName string
	Services    []string
}

type renderedComposeVolumeModel struct {
	Services map[string]struct {
		Image   string                       `json:"image"`
		Volumes []renderedComposeVolumeMount `json:"volumes"`
	} `json:"services"`
	Volumes map[string]struct {
		Name     string `json:"name"`
		External bool   `json:"external"`
	} `json:"volumes"`
}

type renderedComposeVolumeMount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func (m *renderedComposeVolumeMount) UnmarshalJSON(data []byte) error {
	var object struct {
		Type   string `json:"type"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if len(data) > 0 && data[0] == '{' {
		if err := json.Unmarshal(data, &object); err != nil {
			return err
		}
		m.Type = object.Type
		m.Source = object.Source
		m.Target = object.Target
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ":")
	if len(parts) == 1 {
		m.Type = "volume"
		m.Target = parts[0]
		return nil
	}
	m.Source = strings.TrimSpace(parts[0])
	m.Target = strings.TrimSpace(parts[1])
	if filepath.IsAbs(m.Source) || strings.HasPrefix(m.Source, "./") || strings.HasPrefix(m.Source, "../") {
		m.Type = "bind"
	} else {
		m.Type = "volume"
	}
	return nil
}

// ReclaimableReplacedInfrastructureVolumes returns only named volumes whose
// rendered Compose ownership can be attributed exclusively to repository
// PostgreSQL/Redis services that BaseHarbor replaced with managed capabilities.
// Volumes shared with selected workload services, external volumes and
// unrecognized infrastructure are intentionally excluded.
func ReclaimableReplacedInfrastructureVolumes(rendered []byte, selected []string, managedSQL, managedCache bool) ([]ReclaimableComposeVolume, error) {
	var model renderedComposeVolumeModel
	if err := json.Unmarshal(rendered, &model); err != nil {
		return nil, fmt.Errorf("decode rendered Compose volume model: %w", err)
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, service := range selected {
		selectedSet[service] = struct{}{}
	}

	replaced := map[string]struct{}{}
	for service, definition := range model.Services {
		if _, keep := selectedSet[service]; keep {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(service))
		image := strings.ToLower(strings.TrimSpace(definition.Image))
		postgres := strings.Contains(name, "postgres") || strings.Contains(name, "postgresql") ||
			strings.Contains(image, "postgres") || strings.Contains(image, "postgresql")
		redis := strings.Contains(name, "redis") || strings.Contains(name, "valkey") ||
			strings.Contains(image, "redis") || strings.Contains(image, "valkey")
		if (postgres && managedSQL) || (redis && managedCache) {
			replaced[service] = struct{}{}
		}
	}
	if len(replaced) == 0 {
		return nil, nil
	}

	consumers := map[string][]string{}
	for service, definition := range model.Services {
		for _, mount := range definition.Volumes {
			if strings.TrimSpace(mount.Type) != "volume" {
				continue
			}
			source := strings.TrimSpace(mount.Source)
			if source == "" {
				continue
			}
			if _, declared := model.Volumes[source]; !declared {
				continue
			}
			consumers[source] = append(consumers[source], service)
		}
	}

	var result []ReclaimableComposeVolume
	for logical, definition := range model.Volumes {
		if definition.External {
			continue
		}
		usedBy := uniqueSorted(consumers[logical])
		if len(usedBy) == 0 {
			continue
		}
		onlyReplaced := true
		for _, service := range usedBy {
			if _, ok := replaced[service]; !ok {
				onlyReplaced = false
				break
			}
		}
		if !onlyReplaced {
			continue
		}
		result = append(result, ReclaimableComposeVolume{
			LogicalName: logical,
			RuntimeName: strings.TrimSpace(definition.Name),
			Services:    usedBy,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LogicalName < result[j].LogicalName })
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

func composeBaseHarborWorkloadProtocol(raw any) (string, error) {
	const key = "io.baseharbor.workload.protocol"
	labels := map[string]string{}
	switch typed := raw.(type) {
	case map[string]any:
		for name, value := range typed {
			labels[strings.TrimSpace(name)] = strings.TrimSpace(fmt.Sprint(value))
		}
	case []any:
		for _, entry := range typed {
			value := strings.TrimSpace(fmt.Sprint(entry))
			name, setting, ok := strings.Cut(value, "=")
			if ok {
				labels[strings.TrimSpace(name)] = strings.TrimSpace(setting)
			}
		}
	}
	rawProtocol, declared := labels[key]
	if !declared {
		return "", nil
	}
	protocol := strings.ToLower(strings.TrimSpace(rawProtocol))
	switch protocol {
	case "http", "https":
		return protocol, nil
	default:
		return "", fmt.Errorf("%s must be http or https, got %q", key, rawProtocol)
	}
}
