package repositoryinspect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func MarshalJSONResult(result Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// ParsePublishedPort extracts the host-side numeric port from common Compose
// short syntax. It is intentionally conservative and returns false on ambiguity.
func ParsePublishedPort(value string) (int, bool) {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return 0, false
	}
	host := parts[len(parts)-2]
	if strings.Contains(host, "-") {
		return 0, false
	}
	port, err := strconv.Atoi(host)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func detectedLogicalInstanceName(serviceName, kind string) string {
	name := slugify(serviceName)
	prefixes := []string{kind + "-"}
	suffixes := []string{"-" + kind}
	if kind == "postgres" {
		prefixes = append(prefixes, "postgresql-", "pg-")
		suffixes = append(suffixes, "-postgresql", "-pg")
	} else {
		prefixes = append(prefixes, "valkey-", "redis-")
		suffixes = append(suffixes, "-valkey", "-redis")
	}
	for _, prefix := range prefixes {
		name = strings.TrimPrefix(name, prefix)
	}
	for _, suffix := range suffixes {
		name = strings.TrimSuffix(name, suffix)
	}
	if name == "" {
		return slugify(serviceName)
	}
	return name
}

func AnalyzeComposeFile(root, rel string) (ComposeAnalysis, error) {
	if strings.TrimSpace(rel) == "" {
		return ComposeAnalysis{}, fmt.Errorf("compose path is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ComposeAnalysis{}, fmt.Errorf("resolve compose root: %w", err)
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return ComposeAnalysis{}, fmt.Errorf("compose path %q must remain inside repository", rel)
	}
	path := filepath.Join(absRoot, cleanRel)
	data, err := os.ReadFile(path)
	if err != nil {
		return ComposeAnalysis{}, fmt.Errorf("read compose file %s: %w", filepath.ToSlash(cleanRel), err)
	}
	analysis := ComposeAnalysis{}
	for _, service := range detectComposeServices(data) {
		if service.Postgres {
			analysis.SQLInstances = append(analysis.SQLInstances, detectedLogicalInstanceName(service.Name, "postgres"))
		}
		if service.Redis {
			analysis.CacheInstances = append(analysis.CacheInstances, detectedLogicalInstanceName(service.Name, "redis"))
		}
		if service.ObjectStorage {
			analysis.ObjectStorageServices = append(analysis.ObjectStorageServices, service.Name)
		}
		if service.Postgres || service.Redis || service.ObjectStorage {
			analysis.InfrastructureServices = append(analysis.InfrastructureServices, service.Name)
		} else if service.AmbiguousInfrastructure {
			analysis.AmbiguousServices = append(analysis.AmbiguousServices, service.Name)
		} else if service.HasBuild || service.HasImage || service.HasPorts {
			analysis.WorkloadServices = append(analysis.WorkloadServices, service.Name)
		}
		for _, port := range service.Ports {
			analysis.Ports = append(analysis.Ports, PortEvidence{
				Path: filepath.ToSlash(cleanRel), Service: service.Name, Value: port,
			})
		}
		if service.HealthCheck {
			analysis.HealthChecks = append(analysis.HealthChecks, Evidence{
				Kind: EvidenceHealth, Path: filepath.ToSlash(cleanRel),
				Detail: "compose service " + service.Name + " declares healthcheck",
			})
		}
	}
	analysis.SQLInstances = uniqueSorted(analysis.SQLInstances)
	analysis.CacheInstances = uniqueSorted(analysis.CacheInstances)
	analysis.ObjectStorageServices = uniqueSorted(analysis.ObjectStorageServices)
	analysis.InfrastructureServices = uniqueSorted(analysis.InfrastructureServices)
	analysis.AmbiguousServices = uniqueSorted(analysis.AmbiguousServices)
	analysis.WorkloadServices = uniqueSorted(analysis.WorkloadServices)
	sort.Slice(analysis.Ports, func(i, j int) bool {
		if analysis.Ports[i].Service != analysis.Ports[j].Service {
			return analysis.Ports[i].Service < analysis.Ports[j].Service
		}
		return analysis.Ports[i].Value < analysis.Ports[j].Value
	})
	analysis.HealthChecks = uniqueEvidence(analysis.HealthChecks)
	return analysis, nil
}

func inspectDockerfile(data []byte, path string) ([]PortEvidence, []Evidence) {
	var ports []PortEvidence
	var health []Evidence
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "EXPOSE ") {
			for _, value := range strings.Fields(strings.TrimSpace(line[len("EXPOSE "):])) {
				ports = append(ports, PortEvidence{Path: path, Value: value})
			}
		}
		if strings.HasPrefix(upper, "HEALTHCHECK ") {
			health = append(health, Evidence{
				Kind: EvidenceHealth, Path: path, Detail: "Dockerfile declares HEALTHCHECK",
			})
		}
	}
	return ports, health
}

func envNamesOnly(data []byte) []byte {
	var b strings.Builder
	for _, name := range readEnvNames(data) {
		b.WriteString(name)
		b.WriteString("=\n")
	}
	return []byte(b.String())
}
