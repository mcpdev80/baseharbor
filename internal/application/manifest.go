package application

import (
	"bufio"
	"fmt"
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
}

type Services struct {
	Postgres          bool
	Redis             bool
	Secrets           bool
	PostgresInstances map[string]ServiceInstance
	RedisInstances    map[string]ServiceInstance
}

// ServiceInstance is the stable logical identity of one requested backend
// service. The empty v1 shape is intentional: topology remains a BaseHarbor
// implementation detail and future intent such as availability can evolve here.
type ServiceInstance struct{}

// SecretRequirements declares application-owned secret requirements. Values
// never belong in the manifest, and the contract intentionally does not encode
// a runtime delivery mechanism.
type SecretRequirements struct {
	Required []SecretRequirement
}

type SecretRequirement struct {
	Name string
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

func RequiredSecretNames(m Manifest) []string {
	names := make([]string, 0, len(m.Secrets.Required))
	for _, requirement := range m.Secrets.Required {
		names = append(names, requirement.Name)
	}
	return names
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
	if len(postgres) == 0 && len(redis) == 0 && !m.Services.Secrets {
		return fmt.Errorf("at least one backend service must be enabled")
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
			default:
				return Manifest{}, fmt.Errorf("line %d: unsupported top-level field %q", lineNo, trim)
			}
		case 2:
			serviceField = ""
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
			return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
		case 4:
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
				continue
			}
			return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
		case 6:
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
		default:
			return Manifest{}, fmt.Errorf("line %d: indentation must use 0, 2, 4 or 6 spaces", lineNo)
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
