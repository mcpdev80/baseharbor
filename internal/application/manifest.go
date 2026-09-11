package application

import (
	"bufio"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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
}

type Services struct {
	Postgres          bool
	Redis             bool
	Secrets           bool
	PostgresInstances map[string]ServiceInstance
	RedisInstances    map[string]ServiceInstance
}

// WorkloadConfig optionally disambiguates an existing application Compose
// workload. Empty values keep the common case convention-based: BaseHarbor may
// detect one unambiguous Compose file and attach all of its services.
type WorkloadConfig struct {
	Compose  string
	Services []string
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

func WithWorkload(m Manifest, compose string, services ...string) Manifest {
	m.Workload.Compose = compose
	m.Workload.Services = append([]string(nil), services...)
	return m
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

func WithGeneratedSecret(m Manifest, name, generationType string, size int) Manifest {
	generation := &SecretGeneration{Type: generationType}
	switch generationType {
	case "random":
		generation.Length = size
	case "hex":
		generation.Bytes = size
	}
	m.Secrets.Required = append(m.Secrets.Required, SecretRequirement{Name: name, Generate: generation})
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

func GeneratedSecretRequirements(m Manifest) []SecretRequirement {
	var generated []SecretRequirement
	for _, requirement := range m.Secrets.Required {
		if requirement.Generate != nil {
			generated = append(generated, requirement)
		}
	}
	sort.Slice(generated, func(i, j int) bool { return generated[i].Name < generated[j].Name })
	return generated
}

func SecretRequirementByName(m Manifest, name string) (SecretRequirement, bool) {
	for _, requirement := range m.Secrets.Required {
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
	if len(postgres) == 0 && len(redis) == 0 && !m.Services.Secrets && !HasExplicitWorkload(m) {
		return fmt.Errorf("at least one backend service or explicit Compose workload must be enabled")
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
	if len(m.Secrets.Required) > 0 && !m.Services.Secrets {
		return fmt.Errorf("secrets.required needs services.secrets enabled")
	}
	seen := make(map[string]struct{}, len(m.Secrets.Required))
	for _, requirement := range m.Secrets.Required {
		if err := validateSecretKey(requirement.Name); err != nil {
			return err
		}
		if _, exists := seen[requirement.Name]; exists {
			return fmt.Errorf("duplicate required secret %q", requirement.Name)
		}
		seen[requirement.Name] = struct{}{}
		if err := validateSecretGeneration(requirement.Name, requirement.Generate); err != nil {
			return err
		}
	}
	if err := validateWorkload(m.Workload); err != nil {
		return err
	}
	return nil
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
	fmt.Fprintf(&b, "version: %d\napp:\n  name: %s\n  environment: %s\nservices:\n", m.Version, m.Name, m.Environment)
	writeServiceYAML(&b, "postgres", m.Services.Postgres, m.Services.PostgresInstances)
	writeServiceYAML(&b, "redis", m.Services.Redis, m.Services.RedisInstances)
	fmt.Fprintf(&b, "  secrets:\n    enabled: %t\n", m.Services.Secrets)
	if len(m.Secrets.Required) > 0 {
		requirements := append([]SecretRequirement(nil), m.Secrets.Required...)
		sort.Slice(requirements, func(i, j int) bool { return requirements[i].Name < requirements[j].Name })
		b.WriteString("secrets:\n  required:\n")
		for _, requirement := range requirements {
			fmt.Fprintf(&b, "    - name: %s\n", requirement.Name)
			if requirement.Generate != nil {
				fmt.Fprintf(&b, "      generate:\n        type: %s\n", requirement.Generate.Type)
				switch requirement.Generate.Type {
				case "random":
					fmt.Fprintf(&b, "        length: %d\n", requirement.Generate.Length)
				case "hex":
					fmt.Fprintf(&b, "        bytes: %d\n", requirement.Generate.Bytes)
				}
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
	return b.String()
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
				if service != "postgres" && service != "redis" && service != "secrets" {
					return Manifest{}, fmt.Errorf("line %d: unsupported service %q", lineNo, service)
				}
				continue
			}
			if section == "secrets" && trim == "required:" {
				secretField = "required"
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
				if trim == "instances:" && service != "secrets" {
					serviceField = "instances"
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
			return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
		case 6:
			if section == "secrets" && secretField == "required" && secretIndex >= 0 && trim == "generate:" {
				m.Secrets.Required[secretIndex].Generate = &SecretGeneration{}
				secretGenerate = true
				continue
			}
			if section != "services" || serviceField != "instances" || (service != "postgres" && service != "redis") {
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
			if service == "postgres" {
				if m.Services.PostgresInstances == nil {
					m.Services.PostgresInstances = map[string]ServiceInstance{}
				}
				m.Services.PostgresInstances[name] = ServiceInstance{}
				m.Services.Postgres = true
			} else {
				if m.Services.RedisInstances == nil {
					m.Services.RedisInstances = map[string]ServiceInstance{}
				}
				m.Services.RedisInstances[name] = ServiceInstance{}
				m.Services.Redis = true
			}
		case 8:
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
