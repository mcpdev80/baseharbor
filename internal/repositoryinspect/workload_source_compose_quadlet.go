package repositoryinspect

import (
	"fmt"
	"path/filepath"
	"strings"
)

func normalizeComposeSource(snapshot Snapshot, candidate WorkloadSourceCandidate) ([]WorkloadComponent, error) {
	data, ok := snapshot.Files[candidate.Path]
	if !ok {
		return nil, fmt.Errorf("Compose source %s is missing from snapshot", candidate.Path)
	}
	services, err := detectComposeServices(data)
	if err != nil {
		return nil, err
	}
	components := make([]WorkloadComponent, 0, len(services))
	for _, service := range services {
		component := WorkloadComponent{
			ID:                normalizeLogicalComponentID(service.Name),
			Source:            []WorkloadSourceReference{{Kind: WorkloadSourceCompose, Path: candidate.Path, Resource: "services." + service.Name}},
			Image:             service.Image,
			Build:             service.Build,
			Ports:             append([]string(nil), service.Ports...),
			Exposure:          append([]string(nil), service.Ports...),
			Health:            service.HealthCheck,
			EnvironmentRefs:   append([]string(nil), service.EnvironmentRefs...),
			ConfigRefs:        append([]string(nil), service.ConfigRefs...),
			Dependencies:      append([]string(nil), service.Dependencies...),
			PersistentStorage: append([]string(nil), service.Volumes...),
		}
		switch {
		case service.Postgres:
			component.InfrastructureClass = "database.sql"
		case service.Redis:
			component.InfrastructureClass = "cache.key-value"
		case service.MongoDB:
			component.InfrastructureClass = "database.document"
		case service.RabbitMQ:
			component.InfrastructureClass = "messaging"
		case service.ObjectStorage:
			component.InfrastructureClass = "storage.object"
		}
		if component.InfrastructureClass == "" {
			component.InfrastructureClass = classifyInfrastructure(service.Name, service.Image)
		}
		components = append(components, component)
	}
	return components, nil
}

func normalizeQuadletSource(snapshot Snapshot, candidate WorkloadSourceCandidate) ([]WorkloadComponent, error) {
	var components []WorkloadComponent
	for _, path := range candidate.Evidence {
		data, ok := snapshot.Files[path]
		if !ok {
			continue
		}
		base := filepath.Base(path)
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".container" && ext != ".kube" && ext != ".pod" {
			continue
		}
		values := parseINIValues(string(data))
		if ext == ".kube" {
			rawRef := firstINIValue(values, "kube.yaml")
			ref := resolveQuadletLocalReference(path, rawRef)
			if strings.TrimSpace(rawRef) != "" {
				if ref == "" {
					return nil, fmt.Errorf("Quadlet kube unit %s has unresolved inputs: invalid local Yaml=%s reference", path, rawRef)
				}
				if _, ok := snapshot.Files[ref]; !ok {
					return nil, fmt.Errorf("Quadlet kube unit %s has unresolved inputs: referenced YAML %s was not found", path, ref)
				}
				nestedCandidate := WorkloadSourceCandidate{
					Kind:       WorkloadSourceKubernetes,
					Path:       filepath.ToSlash(filepath.Dir(ref)),
					Confidence: ConfidenceDetected,
					Evidence:   []string{ref},
				}
				nestedComponents, _, err := normalizeKubernetesSource(snapshot, nestedCandidate)
				if err != nil {
					return nil, fmt.Errorf("normalize Quadlet kube unit %s: %w", path, err)
				}
				for i := range nestedComponents {
					nestedComponents[i].Source = append(nestedComponents[i].Source, WorkloadSourceReference{
						Kind: WorkloadSourceQuadlet, Path: path, Resource: base,
					})
				}
				components = append(components, nestedComponents...)
				continue
			}
		}
		id := normalizeLogicalComponentID(strings.TrimSuffix(base, ext))
		component := WorkloadComponent{
			ID:     id,
			Source: []WorkloadSourceReference{{Kind: WorkloadSourceQuadlet, Path: path, Resource: base}},
		}
		component.Image = firstINIValue(values, "container.image")
		component.Ports = append(component.Ports, values["container.publishport"]...)
		component.Exposure = append(component.Exposure, values["container.publishport"]...)
		component.EnvironmentRefs = append(component.EnvironmentRefs, values["container.environmentfile"]...)
		component.PersistentStorage = append(component.PersistentStorage, values["container.volume"]...)
		component.Dependencies = append(component.Dependencies, values["unit.requires"]...)
		component.Health = firstINIValue(values, "container.healthcmd", "container.healthonfailure") != ""
		component.InfrastructureClass = classifyInfrastructure(id, component.Image)
		components = append(components, component)
	}
	return mergeComponents(components), nil
}


func parseINIValues(content string) map[string][]string {
	result := map[string][]string{}
	section := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		full := section + "." + strings.ToLower(strings.TrimSpace(key))
		result[full] = append(result[full], strings.TrimSpace(value))
	}
	return result
}

func firstINIValue(values map[string][]string, keys ...string) string {
	for _, key := range keys {
		if v := values[key]; len(v) > 0 && strings.TrimSpace(v[0]) != "" {
			return strings.TrimSpace(v[0])
		}
	}
	return ""
}


func isComposeSourceFile(base string) bool {
	switch strings.ToLower(base) {
	case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
		return true
	default:
		return false
	}
}

func isQuadletFile(base string) bool {
	switch strings.ToLower(filepath.Ext(base)) {
	case ".container", ".pod", ".network", ".volume", ".kube":
		return true
	default:
		return false
	}
}

