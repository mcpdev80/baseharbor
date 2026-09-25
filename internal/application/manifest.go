package application

import (
	"sort"
	"strings"
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
	SQL                  bool
	Cache                bool
	Secrets              bool
	ObjectStorage        bool
	SQLInstances         map[string]ServiceInstance
	CacheInstances       map[string]ServiceInstance
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

func New(name, environment string, sql, cache, secrets bool) Manifest {
	if environment == "" {
		environment = "dev"
	}
	if !sql && !cache && !secrets {
		sql = true
	}
	return Manifest{Version: CurrentVersion, Name: name, Environment: environment, Services: Services{SQL: sql, Cache: cache, Secrets: secrets}}
}

func WithSQLInstances(m Manifest, names ...string) Manifest {
	if len(names) == 0 {
		return m
	}
	if m.Services.SQLInstances == nil {
		m.Services.SQLInstances = make(map[string]ServiceInstance, len(names))
	}
	for _, name := range names {
		m.Services.SQLInstances[name] = ServiceInstance{}
	}
	m.Services.SQL = true
	return m
}

func WithCacheInstances(m Manifest, names ...string) Manifest {
	if len(names) == 0 {
		return m
	}
	if m.Services.CacheInstances == nil {
		m.Services.CacheInstances = make(map[string]ServiceInstance, len(names))
	}
	for _, name := range names {
		m.Services.CacheInstances[name] = ServiceInstance{}
	}
	m.Services.Cache = true
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

func SQLInstanceNames(m Manifest) []string {
	return serviceInstanceNames(m.Services.SQL, m.Services.SQLInstances)
}

func CacheInstanceNames(m Manifest) []string {
	return serviceInstanceNames(m.Services.Cache, m.Services.CacheInstances)
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
