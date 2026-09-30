package resourcepreflight

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type composeDocument struct {
	Services map[string]struct {
		MemLimit any `yaml:"mem_limit"`
		Deploy struct {
			Resources struct {
				Limits struct {
					Memory any `yaml:"memory"`
				} `yaml:"limits"`
				Reservations struct {
					Memory any `yaml:"memory"`
				} `yaml:"reservations"`
			} `yaml:"resources"`
		} `yaml:"deploy"`
	} `yaml:"services"`
}

func ParseComposeWorkloadMemory(data []byte, selected []string) ([]Requirement, error) {
	var doc composeDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse Compose workload memory: %w", err)
	}
	out := make([]Requirement, 0, len(selected))
	for _, name := range selected {
		service, ok := doc.Services[name]
		if !ok {
			return nil, fmt.Errorf("selected workload service %q is missing from Compose", name)
		}
		var raw any
		source := ""
		if service.Deploy.Resources.Reservations.Memory != nil {
			raw = service.Deploy.Resources.Reservations.Memory
			source = "compose deploy.resources.reservations.memory"
		} else if service.MemLimit != nil {
			raw = service.MemLimit
			source = "compose mem_limit"
		} else if service.Deploy.Resources.Limits.Memory != nil {
			raw = service.Deploy.Resources.Limits.Memory
			source = "compose deploy.resources.limits.memory"
		}
		if raw == nil {
			out = append(out, Requirement{
				ID: "workload/" + name, Kind: "workload", Confidence: ConfidenceUnknown,
				Source: "no explicit Compose memory request/limit and no runtime observation",
			})
			continue
		}
		bytes, err := parseMemoryValue(raw)
		if err != nil {
			return nil, fmt.Errorf("service %s %s: %w", name, source, err)
		}
		out = append(out, Requirement{
			ID: "workload/" + name, Kind: "workload", MinimumBytes: bytes,
			EstimatedBytes: bytes, Confidence: ConfidenceExplicit, Source: source,
		})
	}
	return out, nil
}

func parseMemoryValue(raw any) (uint64, error) {
	switch value := raw.(type) {
	case int:
		if value <= 0 {
			return 0, fmt.Errorf("memory value must be positive")
		}
		return uint64(value), nil
	case int64:
		if value <= 0 {
			return 0, fmt.Errorf("memory value must be positive")
		}
		return uint64(value), nil
	case uint64:
		if value == 0 {
			return 0, fmt.Errorf("memory value must be positive")
		}
		return value, nil
	case float64:
		if value <= 0 {
			return 0, fmt.Errorf("memory value must be positive")
		}
		return uint64(value), nil
	case string:
		return ParseMemoryBytes(value)
	default:
		return ParseMemoryBytes(fmt.Sprint(raw))
	}
}

func ParseMemoryBytes(raw string) (uint64, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return 0, fmt.Errorf("memory value is empty")
	}
	multiplier := uint64(1)
	for _, suffix := range []struct {
		S string
		M uint64
	}{
		{"gib", 1024 * 1024 * 1024}, {"gb", 1000 * 1000 * 1000}, {"g", 1000 * 1000 * 1000},
		{"mib", 1024 * 1024}, {"mb", 1000 * 1000}, {"m", 1000 * 1000},
		{"kib", 1024}, {"kb", 1000}, {"k", 1000}, {"b", 1},
	} {
		if strings.HasSuffix(value, suffix.S) {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix.S))
			multiplier = suffix.M
			break
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid memory value %q", raw)
	}
	return uint64(number * float64(multiplier)), nil
}
