package application

import (
	"bufio"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const CurrentVersion = 1

const defaultServiceInstance = "default"

// Manifest is the declarative application backend request understood by BaseHarbor.
type Manifest struct {
	Version     int
	Name        string
	Environment string
	Services    Services
	Secrets     SecretRequirements
	Workload    WorkloadConfig
	Exposures   []HTTPExposureRequirement
	Telemetry   TelemetryRequirements
	Metrics     MetricsRequirements
	Logs        LogsRequirements
	Runtime     RuntimeRequirements
}

type RuntimeRequirements struct {
	Permissions []RuntimePermission
}

type RuntimePermission struct {
	Capability string
	Services   []string
	Operations []string
}

type TelemetryRequirements struct {
	OTLP *OTLPRequirement
}

type OTLPRequirement struct {
	Signals []string
}

type MetricsRequirements struct {
	Sources []MetricsSourceRequirement
}

type LogsRequirements struct {
	Collect []string
}

type MetricsSourceRequirement struct {
	Name    string
	Service string
	Port    int
	Path    string
}

type Services struct {
	Postgres             bool
	Redis                bool
	Secrets              bool
	ObjectStorage        bool
	PostgresInstances    map[string]ServiceInstance
	RedisInstances       map[string]ServiceInstance
	ObjectStorageBuckets map[string]ServiceInstance
}

// WorkloadConfig optionally disambiguates an existing application Compose
// workload. Empty values keep the common case convention-based: BaseHarbor may
// detect one unambiguous Compose file and attach all of its services.
type WorkloadConfig struct {
	Compose  string
	Services []string
}

// HTTPExposureRequirement is provider-neutral public HTTP exposure intent.
// Hostname, host port, TLS source paths and reverse-proxy implementation are
// deployment/provider state and deliberately absent.
type HTTPExposureRequirement struct {
	Name       string
	Service    string
	Port       int
	Protocol   string
	Visibility string
}

// ServiceInstance is the stable logical identity of one requested backend
// service. The empty v1 shape is intentional: topology remains a BaseHarbor
// implementation detail and future intent such as availability can evolve here.
type ServiceInstance struct{}

// SecretRequirements declares application-owned secret requirements. Values
// never belong in the manifest. Generate only expresses explicit intent for
// BaseHarbor to create a missing value directly in the managed secret backend.
type SecretRequirements struct {
	Required []SecretRequirement
	Optional []SecretRequirement
}

type SecretRequirement struct {
	Name     string
	Generate *SecretGeneration
}

type SecretGeneration struct {
	Type   string
	Length int
	Bytes  int
}

func New(name, environment string, postgres, redis, secrets bool) Manifest {
	if environment == "" {
		environment = "dev"
	}
	if !postgres && !redis && !secrets {
		postgres = true
	}
	return Manifest{Version: CurrentVersion, Name: name, Environment: environment, Services: Services{Postgres: postgres, Redis: redis, Secrets: secrets}}
}

func WithPostgresInstances(m Manifest, names ...string) Manifest {
	if len(names) == 0 {
		return m
	}
	if m.Services.PostgresInstances == nil {
		m.Services.PostgresInstances = make(map[string]ServiceInstance, len(names))
	}
	for _, name := range names {
		m.Services.PostgresInstances[name] = ServiceInstance{}
	}
	m.Services.Postgres = true
	return m
}

func WithRedisInstances(m Manifest, names ...string) Manifest {
	if len(names) == 0 {
		return m
	}
	if m.Services.RedisInstances == nil {
		m.Services.RedisInstances = make(map[string]ServiceInstance, len(names))
	}
	for _, name := range names {
		m.Services.RedisInstances[name] = ServiceInstance{}
	}
	m.Services.Redis = true
	return m
}

func WithObjectStorageBuckets(m Manifest, names ...string) Manifest {
	if len(names) == 0 {
		return m
	}
	if m.Services.ObjectStorageBuckets == nil {
		m.Services.ObjectStorageBuckets = make(map[string]ServiceInstance, len(names))
	}
	for _, name := range names {
		m.Services.ObjectStorageBuckets[name] = ServiceInstance{}
	}
	m.Services.ObjectStorage = true
	return m
}

func ObjectStorageBucketNames(m Manifest) []string {
	return serviceInstanceNames(m.Services.ObjectStorage, m.Services.ObjectStorageBuckets)
}

func WithWorkload(m Manifest, compose string, services ...string) Manifest {
	m.Workload.Compose = compose
	m.Workload.Services = append([]string(nil), services...)
	return m
}

func WithHTTPExposure(m Manifest, name, service string, port int, protocol string) Manifest {
	m.Exposures = append(m.Exposures, HTTPExposureRequirement{Name: name, Service: service, Port: port, Protocol: protocol, Visibility: "public"})
	return m
}

func WithOTLPTelemetry(m Manifest, signals ...string) Manifest {
	m.Telemetry.OTLP = &OTLPRequirement{Signals: append([]string(nil), signals...)}
	return m
}

func HasOTLPTelemetry(m Manifest) bool { return m.Telemetry.OTLP != nil }

func WithMetricsSource(m Manifest, name, service string, port int, path string) Manifest {
	m.Metrics.Sources = append(m.Metrics.Sources, MetricsSourceRequirement{
		Name: strings.TrimSpace(name), Service: strings.TrimSpace(service), Port: port, Path: strings.TrimSpace(path),
	})
	return m
}

func HasMetricsSources(m Manifest) bool { return len(m.Metrics.Sources) > 0 }

func WithLogsCollection(m Manifest, sources ...string) Manifest {
	m.Logs.Collect = append([]string(nil), sources...)
	return m
}

func HasLogsCollection(m Manifest) bool { return len(m.Logs.Collect) > 0 }

func WithRuntimePermission(m Manifest, capabilityID string, services []string, operations ...string) Manifest {
	m.Runtime.Permissions = append(m.Runtime.Permissions, RuntimePermission{
		Capability: strings.TrimSpace(capabilityID),
		Services:   append([]string(nil), services...),
		Operations: append([]string(nil), operations...),
	})
	return m
}

func HasRuntimeCapabilityPermission(m Manifest, capabilityID string) bool {
	capabilityID = strings.TrimSpace(capabilityID)
	for _, permission := range m.Runtime.Permissions {
		if permission.Capability == capabilityID {
			return true
		}
	}
	return false
}

func RuntimePermissionFor(m Manifest, capabilityID, operation string) bool {
	capabilityID = strings.TrimSpace(capabilityID)
	operation = strings.TrimSpace(operation)
	for _, permission := range m.Runtime.Permissions {
		if permission.Capability != capabilityID {
			continue
		}
		for _, allowed := range permission.Operations {
			if allowed == operation {
				return true
			}
		}
	}
	return false
}

func PostgresInstanceNames(m Manifest) []string {
	return serviceInstanceNames(m.Services.Postgres, m.Services.PostgresInstances)
}

func RedisInstanceNames(m Manifest) []string {
	return serviceInstanceNames(m.Services.Redis, m.Services.RedisInstances)
}

func serviceInstanceNames(enabled bool, instances map[string]ServiceInstance) []string {
	if len(instances) == 0 {
		if enabled {
			return []string{defaultServiceInstance}
		}
		return nil
	}
	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func WithRequiredSecrets(m Manifest, names ...string) Manifest {
	for _, name := range names {
		m.Secrets.Required = append(m.Secrets.Required, SecretRequirement{Name: name})
	}
	if len(names) > 0 {
		m.Services.Secrets = true
	}
	return m
}

func WithOptionalSecrets(m Manifest, names ...string) Manifest {
	for _, name := range names {
		m.Secrets.Optional = append(m.Secrets.Optional, SecretRequirement{Name: name})
	}
	if len(names) > 0 {
		m.Services.Secrets = true
	}
	return m
}

func WithGeneratedSecret(m Manifest, name, generationType string, size int) Manifest {
	return withGeneratedSecret(m, true, name, generationType, size)
}

func WithOptionalGeneratedSecret(m Manifest, name, generationType string, size int) Manifest {
	return withGeneratedSecret(m, false, name, generationType, size)
}

func withGeneratedSecret(m Manifest, required bool, name, generationType string, size int) Manifest {
	generation := &SecretGeneration{Type: generationType}
	switch generationType {
	case "random":
		generation.Length = size
	case "hex":
		generation.Bytes = size
	}
	requirement := SecretRequirement{Name: name, Generate: generation}
	if required {
		m.Secrets.Required = append(m.Secrets.Required, requirement)
	} else {
		m.Secrets.Optional = append(m.Secrets.Optional, requirement)
	}
	m.Services.Secrets = true
	return m
}

func RequiredSecretNames(m Manifest) []string {
	names := make([]string, 0, len(m.Secrets.Required))
	for _, requirement := range m.Secrets.Required {
		names = append(names, requirement.Name)
	}
	return names
}

func OptionalSecretNames(m Manifest) []string {
	names := make([]string, 0, len(m.Secrets.Optional))
	for _, requirement := range m.Secrets.Optional {
		names = append(names, requirement.Name)
	}
	return names
}

func GeneratedSecretRequirements(m Manifest) []SecretRequirement {
	var generated []SecretRequirement
	for _, requirement := range append(append([]SecretRequirement(nil), m.Secrets.Required...), m.Secrets.Optional...) {
		if requirement.Generate != nil {
			generated = append(generated, requirement)
		}
	}
	sort.Slice(generated, func(i, j int) bool { return generated[i].Name < generated[j].Name })
	return generated
}

func SecretRequirementByName(m Manifest, name string) (SecretRequirement, bool) {
	for _, requirement := range append(append([]SecretRequirement(nil), m.Secrets.Required...), m.Secrets.Optional...) {
		if requirement.Name == name {
			return requirement, true
		}
	}
	return SecretRequirement{}, false
}

func (m Manifest) Validate() error {
	if m.Version != CurrentVersion {
		return fmt.Errorf("unsupported manifest version %d (expected %d)", m.Version, CurrentVersion)
	}
	if err := validateSlug("application name", m.Name); err != nil {
		return err
	}
	if err := validateSlug("environment", m.Environment); err != nil {
		return err
	}
	postgres := PostgresInstanceNames(m)
	redis := RedisInstanceNames(m)
	objectStorage := ObjectStorageBucketNames(m)
	if len(postgres) == 0 && len(redis) == 0 && len(objectStorage) == 0 && !m.Services.Secrets && !HasExplicitWorkload(m) && !HasOTLPTelemetry(m) && !HasMetricsSources(m) && !HasLogsCollection(m) {
		return fmt.Errorf("at least one backend service, telemetry binding or explicit Compose workload must be enabled")
	}
	for _, name := range postgres {
		if err := validateSlug("PostgreSQL instance name", name); err != nil {
			return err
		}
	}
	for _, name := range redis {
		if err := validateSlug("Redis/Valkey instance name", name); err != nil {
			return err
		}
	}
	for _, name := range objectStorage {
		if err := validateSlug("object-storage bucket name", name); err != nil {
			return err
		}
	}
	if (len(m.Secrets.Required) > 0 || len(m.Secrets.Optional) > 0) && !m.Services.Secrets {
		return fmt.Errorf("secret requirements need services.secrets enabled")
	}
	seen := make(map[string]struct{}, len(m.Secrets.Required)+len(m.Secrets.Optional))
	for _, group := range []struct {
		label string
		items []SecretRequirement
	}{
		{label: "required", items: m.Secrets.Required},
		{label: "optional", items: m.Secrets.Optional},
	} {
		for _, requirement := range group.items {
			if err := validateSecretKey(requirement.Name); err != nil {
				return err
			}
			if _, exists := seen[requirement.Name]; exists {
				return fmt.Errorf("duplicate application secret %q", requirement.Name)
			}
			seen[requirement.Name] = struct{}{}
			if err := validateSecretGeneration(requirement.Name, requirement.Generate); err != nil {
				return err
			}
		}
	}
	if err := validateWorkload(m.Workload); err != nil {
		return err
	}
	if err := validateHTTPExposures(m.Workload, m.Exposures); err != nil {
		return err
	}
	if err := validateOTLPTelemetry(m.Workload, m.Telemetry.OTLP); err != nil {
		return err
	}
	if err := validateMetricsSources(m.Workload, m.Metrics.Sources); err != nil {
		return err
	}
	if err := validateLogsRequirements(m.Workload, m.Logs); err != nil {
		return err
	}
	if err := validateRuntimePermissions(m.Workload, m.Runtime.Permissions); err != nil {
		return err
	}
	return nil
}

func validateRuntimePermissions(workload WorkloadConfig, permissions []RuntimePermission) error {
	seenCapabilities := map[string]struct{}{}
	selectedServices := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selectedServices[service] = struct{}{}
	}
	for _, permission := range permissions {
		spec, err := capability.ParseSpecificationID(capability.SpecificationID(strings.TrimSpace(permission.Capability)))
		if err != nil {
			return fmt.Errorf("runtime permission capability %q: %w", permission.Capability, err)
		}
		canonical := string(spec.ID)
		if _, exists := seenCapabilities[canonical]; exists {
			return fmt.Errorf("duplicate runtime permission capability %q", canonical)
		}
		seenCapabilities[canonical] = struct{}{}
		if len(permission.Services) == 0 {
			return fmt.Errorf("runtime permission %q requires at least one workload service", canonical)
		}
		seenServices := map[string]struct{}{}
		for _, service := range permission.Services {
			if err := validateComposeServiceName(service); err != nil {
				return fmt.Errorf("runtime permission %q: %w", canonical, err)
			}
			if _, exists := seenServices[service]; exists {
				return fmt.Errorf("runtime permission %q repeats workload service %q", canonical, service)
			}
			seenServices[service] = struct{}{}
			if len(selectedServices) > 0 {
				if _, exists := selectedServices[service]; !exists {
					return fmt.Errorf("runtime permission %q targets workload service %q which is not selected", canonical, service)
				}
			}
		}
		if len(permission.Operations) == 0 {
			return fmt.Errorf("runtime permission %q requires at least one operation", canonical)
		}
		seenOperations := map[string]struct{}{}
		for _, operation := range permission.Operations {
			operation = strings.TrimSpace(operation)
			switch operation {
			case "runtime.create", "runtime.get", "runtime.delete", "runtime.rotate":
			default:
				return fmt.Errorf("runtime permission %q has unsupported operation %q", canonical, operation)
			}
			if _, exists := seenOperations[operation]; exists {
				return fmt.Errorf("runtime permission %q repeats operation %q", canonical, operation)
			}
			seenOperations[operation] = struct{}{}
		}
	}
	return nil
}

func validateLogsRequirements(workload WorkloadConfig, logs LogsRequirements) error {
	if len(logs.Collect) == 0 {
		return nil
	}
	if len(workload.Services) == 0 {
		return errors.New("logs collection requires explicit workload.services")
	}
	seen := map[string]struct{}{}
	for _, raw := range logs.Collect {
		source := strings.TrimSpace(strings.ToLower(raw))
		if source != "application" {
			return fmt.Errorf("unsupported logs collect source %q", raw)
		}
		if _, exists := seen[source]; exists {
			return fmt.Errorf("duplicate logs collect source %q", source)
		}
		seen[source] = struct{}{}
	}
	return nil
}

func validateMetricsSources(workload WorkloadConfig, sources []MetricsSourceRequirement) error {
	if len(sources) == 0 {
		return nil
	}
	if len(workload.Services) == 0 {
		return fmt.Errorf("metrics sources require explicit workload.services so service identity is deterministic")
	}
	selected := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selected[service] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, source := range sources {
		if err := validateSlug("metrics source name", source.Name); err != nil {
			return err
		}
		if _, exists := seen[source.Name]; exists {
			return fmt.Errorf("duplicate metrics source %q", source.Name)
		}
		seen[source.Name] = struct{}{}
		if err := validateComposeServiceName(source.Service); err != nil {
			return fmt.Errorf("metrics source %q: %w", source.Name, err)
		}
		if _, ok := selected[source.Service]; !ok {
			return fmt.Errorf("metrics source %q targets workload service %q which is not selected", source.Name, source.Service)
		}
		if source.Port < 1 || source.Port > 65535 {
			return fmt.Errorf("metrics source %q has invalid target port %d", source.Name, source.Port)
		}
		if source.Path == "" || !strings.HasPrefix(source.Path, "/") || strings.ContainsAny(source.Path, "?#\r\n\x00") {
			return fmt.Errorf("metrics source %q path must be an absolute HTTP path without query or fragment", source.Name)
		}
	}
	return nil
}

func validateOTLPTelemetry(workload WorkloadConfig, requirement *OTLPRequirement) error {
	if requirement == nil {
		return nil
	}
	if len(workload.Services) == 0 {
		return fmt.Errorf("OTLP telemetry requires explicit workload.services so service identity is deterministic")
	}
	if len(requirement.Signals) == 0 {
		return fmt.Errorf("OTLP telemetry requires at least one signal")
	}
	seen := map[string]struct{}{}
	for _, signal := range requirement.Signals {
		signal = strings.TrimSpace(signal)
		switch signal {
		case "traces", "metrics", "logs":
		default:
			return fmt.Errorf("OTLP telemetry signal %q must be traces, metrics or logs", signal)
		}
		if _, exists := seen[signal]; exists {
			return fmt.Errorf("duplicate OTLP telemetry signal %q", signal)
		}
		seen[signal] = struct{}{}
	}
	return nil
}

func validateHTTPExposures(workload WorkloadConfig, exposures []HTTPExposureRequirement) error {
	if len(exposures) > 0 && len(workload.Services) == 0 {
		return fmt.Errorf("managed HTTP exposure requires explicit workload.services so endpoint identity is deterministic")
	}
	seen := make(map[string]struct{}, len(exposures))
	selected := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		selected[service] = struct{}{}
	}
	for _, exposure := range exposures {
		if err := validateSlug("HTTP exposure name", exposure.Name); err != nil {
			return err
		}
		if _, exists := seen[exposure.Name]; exists {
			return fmt.Errorf("duplicate HTTP exposure %q", exposure.Name)
		}
		seen[exposure.Name] = struct{}{}
		if err := validateComposeServiceName(exposure.Service); err != nil {
			return fmt.Errorf("HTTP exposure %q: %w", exposure.Name, err)
		}
		if len(selected) > 0 {
			if _, ok := selected[exposure.Service]; !ok {
				return fmt.Errorf("HTTP exposure %q targets workload service %q which is not selected", exposure.Name, exposure.Service)
			}
		}
		if exposure.Port < 1 || exposure.Port > 65535 {
			return fmt.Errorf("HTTP exposure %q has invalid target port %d", exposure.Name, exposure.Port)
		}
		switch exposure.Protocol {
		case "http", "https":
		default:
			return fmt.Errorf("HTTP exposure %q protocol must be http or https", exposure.Name)
		}
		switch normalizedExposureVisibility(exposure.Visibility) {
		case "public", "internal":
		default:
			return fmt.Errorf("HTTP exposure %q visibility must be public or internal", exposure.Name)
		}
	}
	return nil
}

func normalizedExposureVisibility(value string) string {
	if strings.TrimSpace(value) == "" {
		return "public"
	}
	return strings.TrimSpace(value)
}

func validateSecretGeneration(name string, generation *SecretGeneration) error {
	if generation == nil {
		return nil
	}
	switch generation.Type {
	case "random":
		if generation.Length < 16 || generation.Length > 4096 {
			return fmt.Errorf("generated secret %q random length must be between 16 and 4096", name)
		}
		if generation.Bytes != 0 {
			return fmt.Errorf("generated secret %q random generator must use length, not bytes", name)
		}
	case "hex":
		if generation.Bytes < 16 || generation.Bytes > 1024 {
			return fmt.Errorf("generated secret %q hex bytes must be between 16 and 1024", name)
		}
		if generation.Length != 0 {
			return fmt.Errorf("generated secret %q hex generator must use bytes, not length", name)
		}
	default:
		return fmt.Errorf("generated secret %q uses unsupported generator type %q", name, generation.Type)
	}
	return nil
}

func validateWorkload(workload WorkloadConfig) error {
	if workload.Compose != "" {
		clean := filepath.Clean(workload.Compose)
		if filepath.IsAbs(workload.Compose) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("workload compose path %q must stay inside the application repository", workload.Compose)
		}
		if clean != workload.Compose {
			return fmt.Errorf("workload compose path %q must be normalized", workload.Compose)
		}
	}
	seen := make(map[string]struct{}, len(workload.Services))
	for _, service := range workload.Services {
		if err := validateComposeServiceName(service); err != nil {
			return err
		}
		if _, exists := seen[service]; exists {
			return fmt.Errorf("duplicate workload service %q", service)
		}
		seen[service] = struct{}{}
	}
	return nil
}

func validateComposeServiceName(name string) error {
	if name == "" || len(name) > 128 {
		return fmt.Errorf("invalid workload service name %q", name)
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return fmt.Errorf("invalid workload service name %q", name)
		}
	}
	return nil
}

func validateSlug(label, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > 63 {
		return fmt.Errorf("%s must be at most 63 characters", label)
	}
	for i, r := range value {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-'
		if !valid {
			return fmt.Errorf("%s %q must contain only lowercase letters, digits and hyphens", label, value)
		}
		if r == '-' && (i == 0 || i == len(value)-1) {
			return fmt.Errorf("%s %q must not start or end with a hyphen", label, value)
		}
	}
	return nil
}

func validateSecretKey(key string) error {
	if key == "" {
		return fmt.Errorf("required secret key is empty")
	}
	if len(key) > 128 {
		return fmt.Errorf("required secret key %q must be at most 128 characters", key)
	}
	if key == "_baseharbor" || strings.HasPrefix(key, "__baseharbor_") {
		return fmt.Errorf("required secret key %q uses a reserved BaseHarbor name", key)
	}
	for i, r := range key {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !valid || (i == 0 && (r == '-' || r == '.')) {
			return fmt.Errorf("invalid required secret key %q", key)
		}
	}
	return nil
}

func (m Manifest) YAML() string {
	var b strings.Builder
	fmt.Fprintf(&b, "version: %d\napp:\n  name: %s\n  environment: %s\n", m.Version, m.Name, m.Environment)
	if hasManifestServices(m.Services) {
		b.WriteString("services:\n")
		if m.Services.Postgres || len(m.Services.PostgresInstances) > 0 {
			writeServiceYAML(&b, "postgres", m.Services.Postgres, m.Services.PostgresInstances)
		}
		if m.Services.Redis || len(m.Services.RedisInstances) > 0 {
			writeServiceYAML(&b, "redis", m.Services.Redis, m.Services.RedisInstances)
		}
		if m.Services.ObjectStorage || len(m.Services.ObjectStorageBuckets) > 0 {
			writeObjectStorageYAML(&b, m.Services.ObjectStorage, m.Services.ObjectStorageBuckets)
		}
		if m.Services.Secrets {
			b.WriteString("  secrets:\n    enabled: true\n")
		}
	}
	if len(m.Secrets.Required) > 0 || len(m.Secrets.Optional) > 0 {
		b.WriteString("secrets:\n")
		writeSecretRequirementsYAML(&b, "required", m.Secrets.Required)
		writeSecretRequirementsYAML(&b, "optional", m.Secrets.Optional)
	}
	if len(m.Runtime.Permissions) > 0 {
		permissions := append([]RuntimePermission(nil), m.Runtime.Permissions...)
		sort.Slice(permissions, func(i, j int) bool { return permissions[i].Capability < permissions[j].Capability })
		b.WriteString("runtime:\n  permissions:\n")
		for _, permission := range permissions {
			fmt.Fprintf(&b, "    - capability: %s\n", permission.Capability)
			services := append([]string(nil), permission.Services...)
			sort.Strings(services)
			b.WriteString("      services:\n")
			for _, service := range services {
				fmt.Fprintf(&b, "        - %s\n", service)
			}
			operations := append([]string(nil), permission.Operations...)
			sort.Strings(operations)
			b.WriteString("      operations:\n")
			for _, operation := range operations {
				fmt.Fprintf(&b, "        - %s\n", operation)
			}
		}
	}
	if m.Workload.Compose != "" || len(m.Workload.Services) > 0 {
		b.WriteString("workload:\n")
		if m.Workload.Compose != "" {
			fmt.Fprintf(&b, "  compose: %s\n", m.Workload.Compose)
		}
		if len(m.Workload.Services) > 0 {
			services := append([]string(nil), m.Workload.Services...)
			sort.Strings(services)
			b.WriteString("  services:\n")
			for _, service := range services {
				fmt.Fprintf(&b, "    - %s\n", service)
			}
		}
	}
	if len(m.Metrics.Sources) > 0 {
		sources := append([]MetricsSourceRequirement(nil), m.Metrics.Sources...)
		sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
		b.WriteString("metrics:\n  sources:\n")
		for _, source := range sources {
			fmt.Fprintf(&b, "    - name: %s\n", source.Name)
			fmt.Fprintf(&b, "      service: %s\n", source.Service)
			fmt.Fprintf(&b, "      port: %d\n", source.Port)
			fmt.Fprintf(&b, "      path: %s\n", source.Path)
		}
	}
	if len(m.Logs.Collect) > 0 {
		sources := append([]string(nil), m.Logs.Collect...)
		sort.Strings(sources)
		b.WriteString("logs:\n  collect:\n")
		for _, source := range sources {
			fmt.Fprintf(&b, "    - %s\n", source)
		}
	}
	if m.Telemetry.OTLP != nil {
		b.WriteString("telemetry:\n  otlp:\n    signals:\n")
		signals := append([]string(nil), m.Telemetry.OTLP.Signals...)
		sort.Strings(signals)
		for _, signal := range signals {
			fmt.Fprintf(&b, "      - %s\n", signal)
		}
	}
	if len(m.Exposures) > 0 {
		exposures := append([]HTTPExposureRequirement(nil), m.Exposures...)
		sort.Slice(exposures, func(i, j int) bool { return exposures[i].Name < exposures[j].Name })
		b.WriteString("exposure:\n  http:\n")
		for _, exposure := range exposures {
			fmt.Fprintf(&b, "    - name: %s\n", exposure.Name)
			fmt.Fprintf(&b, "      service: %s\n", exposure.Service)
			fmt.Fprintf(&b, "      port: %d\n", exposure.Port)
			fmt.Fprintf(&b, "      protocol: %s\n", exposure.Protocol)
			fmt.Fprintf(&b, "      visibility: %s\n", normalizedExposureVisibility(exposure.Visibility))
		}
	}
	return b.String()
}

func writeSecretRequirementsYAML(b *strings.Builder, field string, source []SecretRequirement) {
	if len(source) == 0 {
		return
	}
	requirements := append([]SecretRequirement(nil), source...)
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].Name < requirements[j].Name })
	fmt.Fprintf(b, "  %s:\n", field)
	for _, requirement := range requirements {
		fmt.Fprintf(b, "    - name: %s\n", requirement.Name)
		if requirement.Generate != nil {
			fmt.Fprintf(b, "      generate:\n        type: %s\n", requirement.Generate.Type)
			switch requirement.Generate.Type {
			case "random":
				fmt.Fprintf(b, "        length: %d\n", requirement.Generate.Length)
			case "hex":
				fmt.Fprintf(b, "        bytes: %d\n", requirement.Generate.Bytes)
			}
		}
	}
}

func hasManifestServices(services Services) bool {
	return services.Postgres || services.Redis || services.Secrets || services.ObjectStorage ||
		len(services.PostgresInstances) > 0 || len(services.RedisInstances) > 0 || len(services.ObjectStorageBuckets) > 0
}

func writeObjectStorageYAML(b *strings.Builder, enabled bool, buckets map[string]ServiceInstance) {
	b.WriteString("  object_storage:\n")
	if len(buckets) == 0 {
		fmt.Fprintf(b, "    enabled: %t\n", enabled)
		return
	}
	b.WriteString("    buckets:\n")
	for _, name := range serviceInstanceNames(true, buckets) {
		fmt.Fprintf(b, "      %s: {}\n", name)
	}
}

func writeServiceYAML(b *strings.Builder, service string, enabled bool, instances map[string]ServiceInstance) {
	fmt.Fprintf(b, "  %s:\n", service)
	if len(instances) == 0 {
		fmt.Fprintf(b, "    enabled: %t\n", enabled)
		return
	}
	b.WriteString("    instances:\n")
	for _, name := range serviceInstanceNames(true, instances) {
		fmt.Fprintf(b, "      %s: {}\n", name)
	}
}

// ParseYAML parses the intentionally small v1 manifest grammar without adding a runtime dependency.
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
				service = strings.TrimSuffix(trim, ":")
				if service != "postgres" && service != "redis" && service != "object_storage" && service != "secrets" {
					return Manifest{}, fmt.Errorf("line %d: unsupported service %q", lineNo, service)
				}
				continue
			}
			if section == "secrets" && trim == "required:" {
				secretField = "required"
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
				if trim == "instances:" && (service == "postgres" || service == "redis") {
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
				case "postgres":
					m.Services.Postgres = enabled
				case "redis":
					m.Services.Redis = enabled
				case "object_storage":
					m.Services.ObjectStorage = enabled
				case "secrets":
					m.Services.Secrets = enabled
				}
				continue
			}
			if section == "secrets" && secretField == "required" && strings.HasPrefix(trim, "- ") {
				item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				name := item
				if strings.HasPrefix(item, "name:") {
					name = strings.TrimSpace(strings.TrimPrefix(item, "name:"))
				}
				if name == "" {
					return Manifest{}, fmt.Errorf("line %d: required secret key is empty", lineNo)
				}
				m.Secrets.Required = append(m.Secrets.Required, SecretRequirement{Name: name})
				secretIndex = len(m.Secrets.Required) - 1
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
			if section == "secrets" && secretField == "required" && secretIndex >= 0 && trim == "generate:" {
				m.Secrets.Required[secretIndex].Generate = &SecretGeneration{}
				secretGenerate = true
				continue
			}
			if section != "services" || (serviceField != "instances" && serviceField != "buckets") {
				return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
			}
			if serviceField == "instances" && service != "postgres" && service != "redis" {
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
			case "postgres":
				if m.Services.PostgresInstances == nil {
					m.Services.PostgresInstances = map[string]ServiceInstance{}
				}
				m.Services.PostgresInstances[name] = ServiceInstance{}
				m.Services.Postgres = true
			case "redis":
				if m.Services.RedisInstances == nil {
					m.Services.RedisInstances = map[string]ServiceInstance{}
				}
				m.Services.RedisInstances[name] = ServiceInstance{}
				m.Services.Redis = true
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
			if section != "secrets" || secretField != "required" || secretIndex < 0 || !secretGenerate || m.Secrets.Required[secretIndex].Generate == nil {
				return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
			}
			key, value, ok := strings.Cut(trim, ":")
			if !ok {
				return Manifest{}, fmt.Errorf("line %d: expected generated secret key: value", lineNo)
			}
			value = strings.TrimSpace(value)
			generation := m.Secrets.Required[secretIndex].Generate
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
