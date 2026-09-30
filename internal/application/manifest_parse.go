package application

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

type manifestYAMLParser struct {
	manifest               Manifest
	section                string
	service                string
	serviceField           string
	secretField            string
	secretIndex            int
	secretGenerate         bool
	workloadField          string
	exposureField          string
	exposureIndex          int
	telemetryField         string
	metricsField           string
	metricsIndex           int
	logsField              string
	runtimeField           string
	runtimePermissionIndex int
	runtimePermissionList  string
	identityField          string
	serviceSeen            map[string]string
}

func newManifestYAMLParser() *manifestYAMLParser {
	return &manifestYAMLParser{
		secretIndex:            -1,
		exposureIndex:          -1,
		metricsIndex:           -1,
		runtimePermissionIndex: -1,
		serviceSeen:            map[string]string{},
	}
}

// ParseYAML parses the intentionally small v1 manifest grammar without adding a runtime dependency.
func ParseYAML(input string) (Manifest, error) {
	parser := newManifestYAMLParser()
	scanner := bufio.NewScanner(strings.NewReader(input))

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimRight(scanner.Text(), " \t\r")
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if err := parser.parseLine(lineNo, indent, trim); err != nil {
			return Manifest{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return Manifest{}, err
	}
	if parser.manifest.ApplicationID == "" {
		return Manifest{}, fmt.Errorf("application id is required")
	}
	if err := ValidateApplicationID(parser.manifest.ApplicationID); err != nil {
		return Manifest{}, err
	}
	if err := parser.manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return parser.manifest, nil
}

func (p *manifestYAMLParser) parseLine(lineNo, indent int, trim string) error {
	switch indent {
	case 0:
		return p.parseTopLevel(lineNo, trim)
	case 2:
		return p.parseIndent2(lineNo, trim)
	case 4:
		return p.parseIndent4(lineNo, trim)
	case 6:
		return p.parseIndent6(lineNo, trim)
	case 8:
		return p.parseIndent8(lineNo, trim)
	default:
		return fmt.Errorf("line %d: indentation must use 0, 2, 4, 6 or 8 spaces", lineNo)
	}
}

func (p *manifestYAMLParser) resetNestedState() {
	p.service = ""
	p.serviceField = ""
	p.secretField = ""
	p.secretIndex = -1
	p.secretGenerate = false
	p.workloadField = ""
	p.exposureField = ""
	p.exposureIndex = -1
	p.telemetryField = ""
	p.metricsField = ""
	p.metricsIndex = -1
	p.logsField = ""
	p.runtimeField = ""
	p.runtimePermissionIndex = -1
	p.runtimePermissionList = ""
	p.identityField = ""
}

func (p *manifestYAMLParser) parseTopLevel(lineNo int, trim string) error {
	p.resetNestedState()

	switch {
	case strings.HasPrefix(trim, "version:"):
		v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trim, "version:")))
		if err != nil {
			return fmt.Errorf("line %d: invalid version", lineNo)
		}
		p.manifest.Version = v
		p.section = ""
	case trim == "app:":
		p.section = "app"
	case trim == "services:":
		p.section = "services"
	case trim == "secrets:":
		p.section = "secrets"
	case trim == "workload:":
		p.section = "workload"
	case trim == "exposure:":
		p.section = "exposure"
	case trim == "telemetry:":
		p.section = "telemetry"
	case trim == "metrics:":
		p.section = "metrics"
	case trim == "logs:":
		p.section = "logs"
	case trim == "runtime:":
		p.section = "runtime"
	case trim == "identity:":
		p.section = "identity"
	default:
		return fmt.Errorf("line %d: unsupported top-level field %q", lineNo, trim)
	}
	return nil
}

func (p *manifestYAMLParser) parseIndent2(lineNo int, trim string) error {
	p.serviceField = ""
	p.secretIndex = -1
	p.secretGenerate = false

	switch {
	case p.section == "app":
		return p.parseAppField(lineNo, trim)
	case p.section == "services" && strings.HasSuffix(trim, ":"):
		return p.parseServiceSection(lineNo, trim)
	case p.section == "secrets" && (trim == "required:" || trim == "optional:"):
		p.secretField = strings.TrimSuffix(trim, ":")
		return nil
	case p.section == "exposure" && trim == "http:":
		p.exposureField = "http"
		return nil
	case p.section == "telemetry" && trim == "otlp:":
		p.manifest.Telemetry.OTLP = &OTLPRequirement{}
		p.telemetryField = "otlp"
		return nil
	case p.section == "metrics" && trim == "sources:":
		p.metricsField = "sources"
		return nil
	case p.section == "logs" && trim == "collect:":
		p.logsField = "collect"
		return nil
	case p.section == "runtime" && trim == "permissions:":
		p.runtimeField = "permissions"
		return nil
	case p.section == "identity":
		switch trim {
		case "callback_paths:", "logout_paths:", "scopes:", "claims:", "authentication:":
			p.identityField = strings.TrimSuffix(trim, ":")
			return nil
		default:
			return fmt.Errorf("line %d: unsupported identity field %q", lineNo, trim)
		}
	case p.section == "workload":
		return p.parseWorkloadField(lineNo, trim)
	default:
		return fmt.Errorf("line %d: invalid manifest structure", lineNo)
	}
}

func (p *manifestYAMLParser) parseAppField(lineNo int, trim string) error {
	key, value, ok := strings.Cut(trim, ":")
	if !ok {
		return fmt.Errorf("line %d: expected key: value", lineNo)
	}
	switch key {
	case "id":
		p.manifest.ApplicationID = strings.TrimSpace(value)
	case "name":
		p.manifest.Name = strings.TrimSpace(value)
	case "environment":
		p.manifest.Environment = strings.TrimSpace(value)
	default:
		return fmt.Errorf("line %d: unsupported app field %q", lineNo, key)
	}
	return nil
}

func (p *manifestYAMLParser) parseServiceSection(lineNo int, trim string) error {
	rawService := strings.TrimSuffix(trim, ":")
	switch rawService {
	case "sql", "cache", "key_value", "messaging_queue", "messaging_pubsub", "messaging_stream", "object_storage", "secrets", "identity", "observability":
		p.service = rawService
	default:
		return fmt.Errorf("line %d: unsupported service %q", lineNo, rawService)
	}
	if _, exists := p.serviceSeen[p.service]; exists {
		return fmt.Errorf("line %d: duplicate service %q", lineNo, rawService)
	}
	p.serviceSeen[p.service] = rawService
	return nil
}

func (p *manifestYAMLParser) parseWorkloadField(lineNo int, trim string) error {
	if trim == "services:" {
		p.workloadField = "services"
		return nil
	}
	key, value, ok := strings.Cut(trim, ":")
	if !ok || key != "compose" {
		return fmt.Errorf("line %d: expected compose: PATH or services:", lineNo)
	}
	p.manifest.Workload.Compose = strings.TrimSpace(value)
	return nil
}

func (p *manifestYAMLParser) parseIndent4(lineNo int, trim string) error {
	p.secretGenerate = false

	switch {
	case p.section == "services" && p.service != "":
		return p.parseServiceField(lineNo, trim)
	case p.section == "secrets" && (p.secretField == "required" || p.secretField == "optional") && strings.HasPrefix(trim, "- "):
		return p.parseSecretRequirement(lineNo, trim)
	case p.section == "workload" && p.workloadField == "services" && strings.HasPrefix(trim, "- "):
		p.manifest.Workload.Services = append(p.manifest.Workload.Services, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
		return nil
	case p.section == "telemetry" && p.telemetryField == "otlp" && trim == "signals:":
		p.telemetryField = "otlp-signals"
		return nil
	case p.section == "logs" && p.logsField == "collect" && strings.HasPrefix(trim, "- "):
		source := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
		if source == "" {
			return fmt.Errorf("line %d: logs collect source is empty", lineNo)
		}
		p.manifest.Logs.Collect = append(p.manifest.Logs.Collect, source)
		return nil
	case p.section == "metrics" && p.metricsField == "sources" && strings.HasPrefix(trim, "- "):
		return p.parseMetricsSource(lineNo, trim)
	case p.section == "runtime" && p.runtimeField == "permissions" && strings.HasPrefix(trim, "- "):
		return p.parseRuntimePermission(lineNo, trim)
	case p.section == "exposure" && p.exposureField == "http" && strings.HasPrefix(trim, "- "):
		return p.parseHTTPExposure(lineNo, trim)
	case p.section == "identity":
		if p.identityField == "authentication" {
			key, value, ok := strings.Cut(trim, ":")
			if !ok {
				return fmt.Errorf("line %d: expected identity authentication key: value", lineNo)
			}
			value = strings.TrimSpace(value)
			switch key {
			case "mfa":
				p.manifest.Identity.Authentication.MFA = value
			case "passwordless":
				enabled, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("line %d: invalid identity passwordless value", lineNo)
				}
				p.manifest.Identity.Authentication.Passwordless = enabled
			case "methods":
				p.identityField = "authentication-methods"
			default:
				return fmt.Errorf("line %d: unsupported identity authentication field %q", lineNo, key)
			}
			return nil
		}
		if strings.HasPrefix(trim, "- ") {
			value := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
			if value == "" {
				return fmt.Errorf("line %d: identity list value is empty", lineNo)
			}
			switch p.identityField {
			case "callback_paths":
				p.manifest.Identity.CallbackPaths = append(p.manifest.Identity.CallbackPaths, value)
			case "logout_paths":
				p.manifest.Identity.LogoutPaths = append(p.manifest.Identity.LogoutPaths, value)
			case "scopes":
				p.manifest.Identity.Scopes = append(p.manifest.Identity.Scopes, value)
			case "claims":
				p.manifest.Identity.Claims = append(p.manifest.Identity.Claims, value)
			default:
				return fmt.Errorf("line %d: invalid identity list", lineNo)
			}
			return nil
		}
		return fmt.Errorf("line %d: invalid identity structure", lineNo)
	default:
		return fmt.Errorf("line %d: invalid manifest structure", lineNo)
	}
}

func (p *manifestYAMLParser) parseServiceField(lineNo int, trim string) error {
	if trim == "instances:" && (p.service == "sql" || p.service == "cache" || p.service == "key_value" || p.service == "messaging_queue" || p.service == "messaging_pubsub" || p.service == "messaging_stream") {
		p.serviceField = "instances"
		return nil
	}
	if trim == "buckets:" && p.service == "object_storage" {
		p.serviceField = "buckets"
		return nil
	}

	key, value, ok := strings.Cut(trim, ":")
	if !ok || (key != "enabled" && key != "management_ui") {
		return fmt.Errorf("line %d: expected enabled: true|false, management_ui: true|false or instances:", lineNo)
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("line %d: invalid enabled value", lineNo)
	}
	if key == "management_ui" {
		switch p.service {
		case "sql":
			p.manifest.Services.SQLManagementUI = enabled
		case "cache":
			p.manifest.Services.CacheManagementUI = enabled
		case "key_value":
			p.manifest.Services.KeyValueManagementUI = enabled
		case "object_storage":
			p.manifest.Services.ObjectStorageManagementUI = enabled
		case "secrets":
			p.manifest.Services.SecretsManagementUI = enabled
		case "identity":
			p.manifest.Services.IdentityManagementUI = enabled
		case "observability":
			p.manifest.Services.ObservabilityManagementUI = enabled
		}
		return nil
	}
	switch p.service {
	case "sql":
		p.manifest.Services.SQL = enabled
	case "cache":
		p.manifest.Services.Cache = enabled
	case "key_value":
		p.manifest.Services.KeyValue = enabled
	case "messaging_queue":
		p.manifest.Services.MessagingQueue = enabled
	case "messaging_pubsub":
		p.manifest.Services.MessagingPubSub = enabled
	case "messaging_stream":
		p.manifest.Services.MessagingStream = enabled
	case "object_storage":
		p.manifest.Services.ObjectStorage = enabled
	case "secrets":
		p.manifest.Services.Secrets = enabled
	case "identity":
		p.manifest.Services.Identity = enabled
	case "observability":
		if enabled {
			return fmt.Errorf("line %d: observability is selected by telemetry/metrics/logs/traces; only management_ui is valid here", lineNo)
		}
	}
	return nil
}

func (p *manifestYAMLParser) parseSecretRequirement(lineNo int, trim string) error {
	item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
	name := item
	if strings.HasPrefix(item, "name:") {
		name = strings.TrimSpace(strings.TrimPrefix(item, "name:"))
	}
	if name == "" {
		return fmt.Errorf("line %d: application secret key is empty", lineNo)
	}
	if p.secretField == "required" {
		p.manifest.Secrets.Required = append(p.manifest.Secrets.Required, SecretRequirement{Name: name})
		p.secretIndex = len(p.manifest.Secrets.Required) - 1
	} else {
		p.manifest.Secrets.Optional = append(p.manifest.Secrets.Optional, SecretRequirement{Name: name})
		p.secretIndex = len(p.manifest.Secrets.Optional) - 1
	}
	return nil
}

func (p *manifestYAMLParser) parseMetricsSource(lineNo int, trim string) error {
	item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
	key, value, ok := strings.Cut(item, ":")
	if !ok || key != "name" || strings.TrimSpace(value) == "" {
		return fmt.Errorf("line %d: metrics source must start with - name: NAME", lineNo)
	}
	p.manifest.Metrics.Sources = append(p.manifest.Metrics.Sources, MetricsSourceRequirement{Name: strings.TrimSpace(value)})
	p.metricsIndex = len(p.manifest.Metrics.Sources) - 1
	return nil
}

func (p *manifestYAMLParser) parseRuntimePermission(lineNo int, trim string) error {
	item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
	key, value, ok := strings.Cut(item, ":")
	if !ok || key != "capability" || strings.TrimSpace(value) == "" {
		return fmt.Errorf("line %d: runtime permission must start with - capability: ID", lineNo)
	}
	p.manifest.Runtime.Permissions = append(p.manifest.Runtime.Permissions, RuntimePermission{Capability: strings.TrimSpace(value)})
	p.runtimePermissionIndex = len(p.manifest.Runtime.Permissions) - 1
	p.runtimePermissionList = ""
	return nil
}

func (p *manifestYAMLParser) parseHTTPExposure(lineNo int, trim string) error {
	item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
	key, value, ok := strings.Cut(item, ":")
	if !ok || key != "name" || strings.TrimSpace(value) == "" {
		return fmt.Errorf("line %d: HTTP exposure must start with - name: NAME", lineNo)
	}
	p.manifest.Exposures = append(p.manifest.Exposures, HTTPExposureRequirement{Name: strings.TrimSpace(value)})
	p.exposureIndex = len(p.manifest.Exposures) - 1
	return nil
}

func (p *manifestYAMLParser) parseIndent6(lineNo int, trim string) error {
	if p.section == "runtime" && p.runtimeField == "permissions" && p.runtimePermissionIndex >= 0 {
		switch trim {
		case "services:":
			p.runtimePermissionList = "services"
			return nil
		case "operations:":
			p.runtimePermissionList = "operations"
			return nil
		}
	}
	if p.section == "identity" && p.identityField == "authentication-methods" && strings.HasPrefix(trim, "- ") {
		value := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
		if value == "" {
			return fmt.Errorf("line %d: identity authentication method is empty", lineNo)
		}
		p.manifest.Identity.Authentication.Methods = append(p.manifest.Identity.Authentication.Methods, value)
		return nil
	}
	if p.section == "telemetry" && p.telemetryField == "otlp-signals" && strings.HasPrefix(trim, "- ") {
		if p.manifest.Telemetry.OTLP == nil {
			return fmt.Errorf("line %d: invalid OTLP telemetry structure", lineNo)
		}
		p.manifest.Telemetry.OTLP.Signals = append(p.manifest.Telemetry.OTLP.Signals, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
		return nil
	}
	if p.section == "metrics" && p.metricsField == "sources" && p.metricsIndex >= 0 {
		return p.parseMetricsSourceField(lineNo, trim)
	}
	if p.section == "exposure" && p.exposureField == "http" && p.exposureIndex >= 0 {
		return p.parseHTTPExposureField(lineNo, trim)
	}
	if p.section == "secrets" && (p.secretField == "required" || p.secretField == "optional") && p.secretIndex >= 0 && trim == "generate:" {
		requirement := secretRequirementPointer(&p.manifest, p.secretField, p.secretIndex)
		if requirement == nil {
			return fmt.Errorf("line %d: invalid secret requirement", lineNo)
		}
		requirement.Generate = &SecretGeneration{}
		p.secretGenerate = true
		return nil
	}
	return p.parseServiceInstance(lineNo, trim)
}

func (p *manifestYAMLParser) parseMetricsSourceField(lineNo int, trim string) error {
	key, value, ok := strings.Cut(trim, ":")
	if !ok {
		return fmt.Errorf("line %d: expected metrics source key: value", lineNo)
	}
	value = strings.TrimSpace(value)
	source := &p.manifest.Metrics.Sources[p.metricsIndex]
	switch key {
	case "service":
		source.Service = value
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("line %d: invalid metrics source port", lineNo)
		}
		source.Port = port
	case "path":
		source.Path = value
	default:
		return fmt.Errorf("line %d: unsupported metrics source field %q", lineNo, key)
	}
	return nil
}

func (p *manifestYAMLParser) parseHTTPExposureField(lineNo int, trim string) error {
	key, value, ok := strings.Cut(trim, ":")
	if !ok {
		return fmt.Errorf("line %d: expected HTTP exposure key: value", lineNo)
	}
	value = strings.TrimSpace(value)
	exposure := &p.manifest.Exposures[p.exposureIndex]
	switch key {
	case "service":
		exposure.Service = value
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("line %d: invalid HTTP exposure port", lineNo)
		}
		exposure.Port = port
	case "protocol":
		exposure.Protocol = value
	case "visibility":
		exposure.Visibility = value
	default:
		return fmt.Errorf("line %d: unsupported HTTP exposure field %q", lineNo, key)
	}
	return nil
}

func (p *manifestYAMLParser) parseServiceInstance(lineNo int, trim string) error {
	if p.section != "services" || (p.serviceField != "instances" && p.serviceField != "buckets") {
		return fmt.Errorf("line %d: invalid manifest structure", lineNo)
	}
	if p.serviceField == "instances" && p.service != "sql" && p.service != "cache" && p.service != "key_value" {
		return fmt.Errorf("line %d: invalid manifest structure", lineNo)
	}
	if p.serviceField == "buckets" && p.service != "object_storage" {
		return fmt.Errorf("line %d: invalid manifest structure", lineNo)
	}

	name, value, ok := strings.Cut(trim, ":")
	if !ok || (strings.TrimSpace(value) != "" && strings.TrimSpace(value) != "{}") {
		return fmt.Errorf("line %d: service instance must use NAME: {}", lineNo)
	}
	name = strings.TrimSpace(name)
	if err := validateSlug("service instance name", name); err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}

	switch p.service {
	case "sql":
		if p.manifest.Services.SQLInstances == nil {
			p.manifest.Services.SQLInstances = map[string]ServiceInstance{}
		}
		p.manifest.Services.SQLInstances[name] = ServiceInstance{}
		p.manifest.Services.SQL = true
	case "cache":
		if p.manifest.Services.CacheInstances == nil {
			p.manifest.Services.CacheInstances = map[string]ServiceInstance{}
		}
		p.manifest.Services.CacheInstances[name] = ServiceInstance{}
		p.manifest.Services.Cache = true
	case "key_value":
		if p.manifest.Services.KeyValueInstances == nil {
			p.manifest.Services.KeyValueInstances = map[string]ServiceInstance{}
		}
		p.manifest.Services.KeyValueInstances[name] = ServiceInstance{}
		p.manifest.Services.KeyValue = true
	case "messaging_queue":
		if p.manifest.Services.MessagingQueueInstances == nil {
			p.manifest.Services.MessagingQueueInstances = map[string]ServiceInstance{}
		}
		p.manifest.Services.MessagingQueueInstances[name] = ServiceInstance{}
		p.manifest.Services.MessagingQueue = true
	case "messaging_pubsub":
		if p.manifest.Services.MessagingPubSubInstances == nil {
			p.manifest.Services.MessagingPubSubInstances = map[string]ServiceInstance{}
		}
		p.manifest.Services.MessagingPubSubInstances[name] = ServiceInstance{}
		p.manifest.Services.MessagingPubSub = true
	case "messaging_stream":
		if p.manifest.Services.MessagingStreamInstances == nil {
			p.manifest.Services.MessagingStreamInstances = map[string]ServiceInstance{}
		}
		p.manifest.Services.MessagingStreamInstances[name] = ServiceInstance{}
		p.manifest.Services.MessagingStream = true
	case "object_storage":
		if p.manifest.Services.ObjectStorageBuckets == nil {
			p.manifest.Services.ObjectStorageBuckets = map[string]ServiceInstance{}
		}
		p.manifest.Services.ObjectStorageBuckets[name] = ServiceInstance{}
		p.manifest.Services.ObjectStorage = true
	}
	return nil
}

func (p *manifestYAMLParser) parseIndent8(lineNo int, trim string) error {
	if p.section == "runtime" && p.runtimeField == "permissions" && p.runtimePermissionIndex >= 0 && strings.HasPrefix(trim, "- ") {
		value := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
		switch p.runtimePermissionList {
		case "services":
			p.manifest.Runtime.Permissions[p.runtimePermissionIndex].Services = append(p.manifest.Runtime.Permissions[p.runtimePermissionIndex].Services, value)
			return nil
		case "operations":
			p.manifest.Runtime.Permissions[p.runtimePermissionIndex].Operations = append(p.manifest.Runtime.Permissions[p.runtimePermissionIndex].Operations, value)
			return nil
		}
	}

	requirement := secretRequirementPointer(&p.manifest, p.secretField, p.secretIndex)
	if p.section != "secrets" || requirement == nil || !p.secretGenerate || requirement.Generate == nil {
		return fmt.Errorf("line %d: invalid manifest structure", lineNo)
	}
	return parseSecretGenerationField(lineNo, trim, requirement.Generate)
}

func parseSecretGenerationField(lineNo int, trim string, generation *SecretGeneration) error {
	key, value, ok := strings.Cut(trim, ":")
	if !ok {
		return fmt.Errorf("line %d: expected generated secret key: value", lineNo)
	}
	value = strings.TrimSpace(value)
	switch key {
	case "type":
		generation.Type = value
	case "length":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("line %d: invalid generated secret length", lineNo)
		}
		generation.Length = n
	case "bytes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("line %d: invalid generated secret bytes", lineNo)
		}
		generation.Bytes = n
	default:
		return fmt.Errorf("line %d: unsupported generated secret field %q", lineNo, key)
	}
	return nil
}
