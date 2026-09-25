package repositoryinspect

import (
	"path/filepath"
	"sort"
	"strings"
)

func artifactPaths(artifacts []Artifact, kind string) []string {
	var paths []string
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			paths = append(paths, artifact.Path)
		}
	}
	return uniqueSorted(paths)
}

func isComposeFile(base string) bool {
	switch base {
	case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
		return true
	default:
		return false
	}
}

func isEnvFile(base string) bool {
	switch base {
	case ".env", ".env.example", ".env.template", ".env.sample":
		return true
	default:
		return false
	}
}

func isDependencyFile(base string) bool {
	switch base {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"go.mod", "go.sum", "pyproject.toml", "requirements.txt", "poetry.lock",
		"cargo.toml", "cargo.lock":
		return true
	default:
		return false
	}
}

func isSourceFile(base string) bool {
	switch strings.ToLower(filepath.Ext(base)) {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
		return true
	default:
		return false
	}
}

func containsExactFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsAnyToken(value string, needles []string) bool {
	for _, needle := range needles {
		n := strings.ToLower(needle)
		if strings.Contains(value, "\""+n+"\"") ||
			strings.Contains(value, "'"+n+"'") ||
			strings.Contains(value, "/"+n) ||
			strings.Contains(value, n+"/") ||
			strings.Contains(value, n+"@") ||
			strings.Contains(value, n+" ") ||
			strings.Contains(value, " "+n) {
			return true
		}
	}
	return false
}

func mergeFindings(current []Finding, incoming []Finding) []Finding {
	index := map[string]int{}
	for i, finding := range current {
		index[finding.Capability+"\x00"+finding.Name] = i
	}
	for _, finding := range incoming {
		key := finding.Capability + "\x00" + finding.Name
		if i, exists := index[key]; exists {
			if confidenceRank(finding.Confidence) > confidenceRank(current[i].Confidence) {
				current[i].Confidence = finding.Confidence
			}
			if current[i].Direction == "" {
				current[i].Direction = finding.Direction
			}
			current[i].Operations = uniqueRuntimeOperations(append(current[i].Operations, finding.Operations...))
			current[i].Evidence = uniqueEvidence(append(current[i].Evidence, finding.Evidence...))
			continue
		}
		finding.Operations = uniqueRuntimeOperations(finding.Operations)
		finding.Evidence = uniqueEvidence(finding.Evidence)
		current = append(current, finding)
		index[key] = len(current) - 1
	}
	return current
}

func confidenceRank(c Confidence) int {
	switch c {
	case ConfidenceDetected:
		return 3
	case ConfidenceSuggested:
		return 2
	case ConfidencePossible:
		return 1
	default:
		return 0
	}
}

func uniqueEvidence(items []Evidence) []Evidence {
	seen := map[string]struct{}{}
	result := make([]Evidence, 0, len(items))
	for _, item := range items {
		key := string(item.Kind) + "\x00" + item.Path + "\x00" + item.Detail
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Detail < result[j].Detail
	})
	return result
}

func uniqueSorted(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func normalizeFindingService(finding *Finding) {
	switch finding.Capability {
	case "database.sql":
		finding.Service = "sql"
	case "cache.key-value":
		finding.Service = "cache"
		finding.Protocol = "RESP"
	case "object-storage.s3":
		finding.Service = "object-storage"
		finding.Protocol = "S3"
	case "secrets":
		finding.Service = "secrets"
	case "metrics":
		finding.Service = "observability"
		finding.Protocol = "OpenMetrics"
	case "telemetry.otlp":
		finding.Service = "observability"
		finding.Protocol = "OTLP"
	case "logs", "traces":
		finding.Service = "observability"
	case "identity":
		finding.Service = "identity"
		finding.Protocol = "OIDC/OAuth"
	case "messaging":
		finding.Service = "messaging"
	case "vector":
		finding.Service = "vector"
	}
}

func sortResult(result *Result) {
	sort.Slice(result.Findings, func(i, j int) bool {
		if result.Findings[i].Capability != result.Findings[j].Capability {
			return result.Findings[i].Capability < result.Findings[j].Capability
		}
		return result.Findings[i].Name < result.Findings[j].Name
	})
	sort.Slice(result.Ports, func(i, j int) bool {
		if result.Ports[i].Path != result.Ports[j].Path {
			return result.Ports[i].Path < result.Ports[j].Path
		}
		if result.Ports[i].Service != result.Ports[j].Service {
			return result.Ports[i].Service < result.Ports[j].Service
		}
		return result.Ports[i].Value < result.Ports[j].Value
	})
	result.HealthChecks = uniqueEvidence(result.HealthChecks)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 63 {
		result = strings.Trim(result[:63], "-")
	}
	return result
}

// MarshalJSONResult exists so control surfaces can share one stable rendering
// path without each re-encoding repository inspection state differently.
