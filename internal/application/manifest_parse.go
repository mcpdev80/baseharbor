package application

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

func ParseYAML(input string) (Manifest, error) {
	var m Manifest
	section := ""
	service := ""
	serviceField := ""
	secretField := ""
	secretIndex := -1
	secretGenerate := false
	workloadField := ""
	exposureField := ""
	exposureIndex := -1
	telemetryField := ""
	metricsField := ""
	metricsIndex := -1
	logsField := ""
	runtimeField := ""
	runtimePermissionIndex := -1
	runtimePermissionList := ""
	serviceSeen := map[string]string{}
	s := bufio.NewScanner(strings.NewReader(input))
	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := strings.TrimRight(s.Text(), " \t\r")
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		switch indent {
		case 0:
			service = ""
			serviceField = ""
			secretField = ""
			secretIndex = -1
			secretGenerate = false
			workloadField = ""
			exposureField = ""
			exposureIndex = -1
			telemetryField = ""
			metricsField = ""
			metricsIndex = -1
			logsField = ""
			runtimeField = ""
			runtimePermissionIndex = -1
			runtimePermissionList = ""
			switch {
			case strings.HasPrefix(trim, "version:"):
				v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trim, "version:")))
				if err != nil {
					return Manifest{}, fmt.Errorf("line %d: invalid version", lineNo)
				}
				m.Version = v
				section = ""
			case trim == "app:":
				section = "app"
			case trim == "services:":
				section = "services"
			case trim == "secrets:":
				section = "secrets"
			case trim == "workload:":
				section = "workload"
			case trim == "exposure:":
				section = "exposure"
			case trim == "telemetry:":
				section = "telemetry"
			case trim == "metrics:":
				section = "metrics"
			case trim == "logs:":
				section = "logs"
			case trim == "runtime:":
				section = "runtime"
			default:
				return Manifest{}, fmt.Errorf("line %d: unsupported top-level field %q", lineNo, trim)
			}
		case 2:
			serviceField = ""
			secretIndex = -1
			secretGenerate = false
			if section == "app" {
				key, value, ok := strings.Cut(trim, ":")
				if !ok {
					return Manifest{}, fmt.Errorf("line %d: expected key: value", lineNo)
				}
				switch key {
				case "name":
					m.Name = strings.TrimSpace(value)
				case "environment":
					m.Environment = strings.TrimSpace(value)
				default:
					return Manifest{}, fmt.Errorf("line %d: unsupported app field %q", lineNo, key)
				}
				continue
			}
			if section == "services" && strings.HasSuffix(trim, ":") {
				rawService := strings.TrimSuffix(trim, ":")
				switch rawService {
				case "sql", "cache", "object_storage", "secrets":
					service = rawService
				default:
					return Manifest{}, fmt.Errorf("line %d: unsupported service %q", lineNo, rawService)
				}
				if _, exists := serviceSeen[service]; exists {
					return Manifest{}, fmt.Errorf("line %d: duplicate service %q", lineNo, rawService)
				}
				serviceSeen[service] = rawService
				continue
			}
			if section == "secrets" && (trim == "required:" || trim == "optional:") {
				secretField = strings.TrimSuffix(trim, ":")
				continue
			}
			if section == "exposure" && trim == "http:" {
				exposureField = "http"
				continue
			}
			if section == "telemetry" && trim == "otlp:" {
				m.Telemetry.OTLP = &OTLPRequirement{}
				telemetryField = "otlp"
				continue
			}
			if section == "metrics" && trim == "sources:" {
				metricsField = "sources"
				continue
			}
			if section == "logs" && trim == "collect:" {
				logsField = "collect"
				continue
			}
			if section == "runtime" && trim == "permissions:" {
				runtimeField = "permissions"
				continue
			}
			if section == "workload" {
				if trim == "services:" {
					workloadField = "services"
					continue
				}
				key, value, ok := strings.Cut(trim, ":")
				if !ok || key != "compose" {
					return Manifest{}, fmt.Errorf("line %d: expected compose: PATH or services:", lineNo)
				}
				m.Workload.Compose = strings.TrimSpace(value)
				continue
			}
			return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
		case 4:
			secretGenerate = false
			if section == "services" && service != "" {
				if trim == "instances:" && (service == "sql" || service == "cache") {
					serviceField = "instances"
					continue
				}
				if trim == "buckets:" && service == "object_storage" {
					serviceField = "buckets"
					continue
				}
				key, value, ok := strings.Cut(trim, ":")
				if !ok || key != "enabled" {
					return Manifest{}, fmt.Errorf("line %d: expected enabled: true|false or instances:", lineNo)
				}
				enabled, err := strconv.ParseBool(strings.TrimSpace(value))
				if err != nil {
					return Manifest{}, fmt.Errorf("line %d: invalid enabled value", lineNo)
				}
				switch service {
				case "sql":
					m.Services.SQL = enabled
				case "cache":
					m.Services.Cache = enabled
				case "object_storage":
					m.Services.ObjectStorage = enabled
				case "secrets":
					m.Services.Secrets = enabled
				}
				continue
			}
			if section == "secrets" && (secretField == "required" || secretField == "optional") && strings.HasPrefix(trim, "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				name := item
				if strings.HasPrefix(item, "name:") {
					name = strings.TrimSpace(strings.TrimPrefix(item, "name:"))
				}
				if name == "" {
					return Manifest{}, fmt.Errorf("line %d: application secret key is empty", lineNo)
				}
				if secretField == "required" {
					m.Secrets.Required = append(m.Secrets.Required, SecretRequirement{Name: name})
					secretIndex = len(m.Secrets.Required) - 1
				} else {
					m.Secrets.Optional = append(m.Secrets.Optional, SecretRequirement{Name: name})
					secretIndex = len(m.Secrets.Optional) - 1
				}
				continue
			}
			if section == "workload" && workloadField == "services" && strings.HasPrefix(trim, "- ") {
				m.Workload.Services = append(m.Workload.Services, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
				continue
			}
			if section == "telemetry" && telemetryField == "otlp" && trim == "signals:" {
				telemetryField = "otlp-signals"
				continue
			}
			if section == "logs" && logsField == "collect" && strings.HasPrefix(trim, "- ") {
				source := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				if source == "" {
					return Manifest{}, fmt.Errorf("line %d: logs collect source is empty", lineNo)
				}
				m.Logs.Collect = append(m.Logs.Collect, source)
				continue
			}
			if section == "metrics" && metricsField == "sources" && strings.HasPrefix(trim, "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				key, value, ok := strings.Cut(item, ":")
				if !ok || key != "name" || strings.TrimSpace(value) == "" {
					return Manifest{}, fmt.Errorf("line %d: metrics source must start with - name: NAME", lineNo)
				}
				m.Metrics.Sources = append(m.Metrics.Sources, MetricsSourceRequirement{Name: strings.TrimSpace(value)})
				metricsIndex = len(m.Metrics.Sources) - 1
				continue
			}
			if section == "runtime" && runtimeField == "permissions" && strings.HasPrefix(trim, "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				key, value, ok := strings.Cut(item, ":")
				if !ok || key != "capability" || strings.TrimSpace(value) == "" {
					return Manifest{}, fmt.Errorf("line %d: runtime permission must start with - capability: ID", lineNo)
				}
				m.Runtime.Permissions = append(m.Runtime.Permissions, RuntimePermission{Capability: strings.TrimSpace(value)})
				runtimePermissionIndex = len(m.Runtime.Permissions) - 1
				runtimePermissionList = ""
				continue
			}
			if section == "exposure" && exposureField == "http" && strings.HasPrefix(trim, "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				key, value, ok := strings.Cut(item, ":")
				if !ok || key != "name" || strings.TrimSpace(value) == "" {
					return Manifest{}, fmt.Errorf("line %d: HTTP exposure must start with - name: NAME", lineNo)
				}
				m.Exposures = append(m.Exposures, HTTPExposureRequirement{Name: strings.TrimSpace(value)})
				exposureIndex = len(m.Exposures) - 1
				continue
			}
			return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
		case 6:
			if section == "runtime" && runtimeField == "permissions" && runtimePermissionIndex >= 0 {
				switch trim {
				case "services:":
					runtimePermissionList = "services"
					continue
				case "operations:":
					runtimePermissionList = "operations"
					continue
				}
			}
			if section == "telemetry" && telemetryField == "otlp-signals" && strings.HasPrefix(trim, "- ") {
				if m.Telemetry.OTLP == nil {
					return Manifest{}, fmt.Errorf("line %d: invalid OTLP telemetry structure", lineNo)
				}
				m.Telemetry.OTLP.Signals = append(m.Telemetry.OTLP.Signals, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
				continue
			}
			if section == "metrics" && metricsField == "sources" && metricsIndex >= 0 {
				key, value, ok := strings.Cut(trim, ":")
				if !ok {
					return Manifest{}, fmt.Errorf("line %d: expected metrics source key: value", lineNo)
				}
				value = strings.TrimSpace(value)
				source := &m.Metrics.Sources[metricsIndex]
				switch key {
				case "service":
					source.Service = value
				case "port":
					port, err := strconv.Atoi(value)
					if err != nil {
						return Manifest{}, fmt.Errorf("line %d: invalid metrics source port", lineNo)
					}
					source.Port = port
				case "path":
					source.Path = value
				default:
					return Manifest{}, fmt.Errorf("line %d: unsupported metrics source field %q", lineNo, key)
				}
				continue
			}
			if section == "exposure" && exposureField == "http" && exposureIndex >= 0 {
				key, value, ok := strings.Cut(trim, ":")
				if !ok {
					return Manifest{}, fmt.Errorf("line %d: expected HTTP exposure key: value", lineNo)
				}
				value = strings.TrimSpace(value)
				exposure := &m.Exposures[exposureIndex]
				switch key {
				case "service":
					exposure.Service = value
				case "port":
					port, err := strconv.Atoi(value)
					if err != nil {
						return Manifest{}, fmt.Errorf("line %d: invalid HTTP exposure port", lineNo)
					}
					exposure.Port = port
				case "protocol":
					exposure.Protocol = value
				case "visibility":
					exposure.Visibility = value
				default:
					return Manifest{}, fmt.Errorf("line %d: unsupported HTTP exposure field %q", lineNo, key)
				}
				continue
			}
			if section == "secrets" && (secretField == "required" || secretField == "optional") && secretIndex >= 0 && trim == "generate:" {
				requirement := secretRequirementPointer(&m, secretField, secretIndex)
				if requirement == nil {
					return Manifest{}, fmt.Errorf("line %d: invalid secret requirement", lineNo)
				}
				requirement.Generate = &SecretGeneration{}
				secretGenerate = true
				continue
			}
			if section != "services" || (serviceField != "instances" && serviceField != "buckets") {
				return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
			}
			if serviceField == "instances" && service != "sql" && service != "cache" {
				return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
			}
			if serviceField == "buckets" && service != "object_storage" {
				return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
			}
			name, value, ok := strings.Cut(trim, ":")
			if !ok || (strings.TrimSpace(value) != "" && strings.TrimSpace(value) != "{}") {
				return Manifest{}, fmt.Errorf("line %d: service instance must use NAME: {}", lineNo)
			}
			name = strings.TrimSpace(name)
			if err := validateSlug("service instance name", name); err != nil {
				return Manifest{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			switch service {
			case "sql":
				if m.Services.SQLInstances == nil {
					m.Services.SQLInstances = map[string]ServiceInstance{}
				}
				m.Services.SQLInstances[name] = ServiceInstance{}
				m.Services.SQL = true
			case "cache":
				if m.Services.CacheInstances == nil {
					m.Services.CacheInstances = map[string]ServiceInstance{}
				}
				m.Services.CacheInstances[name] = ServiceInstance{}
				m.Services.Cache = true
			case "object_storage":
				if m.Services.ObjectStorageBuckets == nil {
					m.Services.ObjectStorageBuckets = map[string]ServiceInstance{}
				}
				m.Services.ObjectStorageBuckets[name] = ServiceInstance{}
				m.Services.ObjectStorage = true
			}
		case 8:
			if section == "runtime" && runtimeField == "permissions" && runtimePermissionIndex >= 0 && strings.HasPrefix(trim, "- ") {
				value := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				switch runtimePermissionList {
				case "services":
					m.Runtime.Permissions[runtimePermissionIndex].Services = append(m.Runtime.Permissions[runtimePermissionIndex].Services, value)
					continue
				case "operations":
					m.Runtime.Permissions[runtimePermissionIndex].Operations = append(m.Runtime.Permissions[runtimePermissionIndex].Operations, value)
					continue
				}
			}
			requirement := secretRequirementPointer(&m, secretField, secretIndex)
			if section != "secrets" || requirement == nil || !secretGenerate || requirement.Generate == nil {
				return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
			}
			key, value, ok := strings.Cut(trim, ":")
			if !ok {
				return Manifest{}, fmt.Errorf("line %d: expected generated secret key: value", lineNo)
			}
			value = strings.TrimSpace(value)
			generation := requirement.Generate
			switch key {
			case "type":
				generation.Type = value
			case "length":
				n, err := strconv.Atoi(value)
				if err != nil {
					return Manifest{}, fmt.Errorf("line %d: invalid generated secret length", lineNo)
				}
				generation.Length = n
			case "bytes":
				n, err := strconv.Atoi(value)
				if err != nil {
					return Manifest{}, fmt.Errorf("line %d: invalid generated secret bytes", lineNo)
				}
				generation.Bytes = n
			default:
				return Manifest{}, fmt.Errorf("line %d: unsupported generated secret field %q", lineNo, key)
			}
		default:
			return Manifest{}, fmt.Errorf("line %d: indentation must use 0, 2, 4, 6 or 8 spaces", lineNo)
		}
	}
	if err := s.Err(); err != nil {
		return Manifest{}, err
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

