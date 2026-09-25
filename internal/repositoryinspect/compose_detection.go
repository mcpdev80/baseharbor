package repositoryinspect

import (
	"bufio"
	"sort"
	"strings"
)

type composeService struct {
	Name                    string
	Postgres                bool
	Redis                   bool
	ObjectStorage           bool
	AmbiguousInfrastructure bool
	HasBuild                bool
	HasImage                bool
	HasPorts                bool
	Ports                   []string
	HealthCheck             bool
}

func detectComposeServices(data []byte) []composeService {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inServices := false
	current := ""
	items := map[string]*composeService{}
	inPorts := false
	for scanner.Scan() {
		raw := strings.TrimRight(scanner.Text(), " 	")
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			inServices = trim == "services:"
			current = ""
			inPorts = false
			continue
		}
		if !inServices {
			continue
		}
		if indent == 2 && strings.HasSuffix(trim, ":") {
			name := strings.TrimSpace(strings.TrimSuffix(trim, ":"))
			if name == "" || strings.Contains(name, " ") {
				continue
			}
			current = name
			item := &composeService{Name: name}
			lowerName := strings.ToLower(name)
			item.Postgres = strings.Contains(lowerName, "postgres") || strings.Contains(lowerName, "postgresql")
			item.Redis = strings.Contains(lowerName, "redis") || strings.Contains(lowerName, "valkey")
			item.ObjectStorage = composeObjectStorageMarker(lowerName)
			item.AmbiguousInfrastructure = !item.Postgres && !item.Redis && !item.ObjectStorage && composeAmbiguousInfrastructureMarker(lowerName)
			items[name] = item
			inPorts = false
			continue
		}
		if current == "" || indent < 4 {
			continue
		}
		item := items[current]
		lower := strings.ToLower(trim)
		switch {
		case strings.HasPrefix(lower, "image:"):
			item.HasImage = true
			image := strings.TrimSpace(strings.TrimPrefix(lower, "image:"))
			item.Postgres = item.Postgres || strings.Contains(image, "postgres") || strings.Contains(image, "postgresql")
			item.Redis = item.Redis || strings.Contains(image, "redis") || strings.Contains(image, "valkey")
			item.ObjectStorage = item.ObjectStorage || composeObjectStorageMarker(image)
			if item.Postgres || item.Redis || item.ObjectStorage {
				item.AmbiguousInfrastructure = false
			}
		case strings.HasPrefix(lower, "build:"):
			item.HasBuild = true
		case lower == "ports:" || strings.HasPrefix(lower, "ports:"):
			item.HasPorts = true
			inPorts = true
		case lower == "healthcheck:" || strings.HasPrefix(lower, "healthcheck:"):
			item.HealthCheck = true
			inPorts = false
		default:
			if indent <= 4 {
				inPorts = false
			}
		}
		if inPorts && strings.HasPrefix(trim, "-") {
			value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trim, "-")), "\"'")
			if value != "" {
				item.Ports = append(item.Ports, value)
			}
		}
	}
	result := make([]composeService, 0, len(items))
	for _, item := range items {
		item.Ports = uniqueSorted(item.Ports)
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
