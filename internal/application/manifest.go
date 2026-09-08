package application

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

const CurrentVersion = 1

// Manifest is the declarative application backend request understood by BaseHarbor.
type Manifest struct {
	Version     int
	Name        string
	Environment string
	Services    Services
	Secrets     SecretContract
}

type Services struct {
	Postgres bool
	Redis    bool
	Secrets  bool
}

// SecretContract declares secret names the application requires. It describes
// what the workload needs, not how a runtime provider delivers the values.
type SecretContract struct {
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

func WithRequiredSecrets(m Manifest, names ...string) Manifest {
	for _, name := range names {
		m.Secrets.Required = append(m.Secrets.Required, SecretRequirement{Name: name})
	}
	if len(names) > 0 {
		m.Services.Secrets = true
	}
	return m
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
	if !m.Services.Postgres && !m.Services.Redis && !m.Services.Secrets {
		return fmt.Errorf("at least one backend service must be enabled")
	}
	if len(m.Secrets.Required) > 0 && !m.Services.Secrets {
		return fmt.Errorf("required secrets need services.secrets enabled")
	}
	seen := make(map[string]struct{}, len(m.Secrets.Required))
	for _, requirement := range m.Secrets.Required {
		if err := validateSecretName(requirement.Name); err != nil {
			return err
		}
		if _, exists := seen[requirement.Name]; exists {
			return fmt.Errorf("required secret %q is declared more than once", requirement.Name)
		}
		seen[requirement.Name] = struct{}{}
	}
	return nil
}

func RequiredSecretNames(m Manifest) []string {
	names := make([]string, 0, len(m.Secrets.Required))
	for _, requirement := range m.Secrets.Required {
		names = append(names, requirement.Name)
	}
	return names
}

func validateSecretName(value string) error {
	if value == "" {
		return fmt.Errorf("required secret name is required")
	}
	if len(value) > 128 {
		return fmt.Errorf("required secret name %q must be at most 128 characters", value)
	}
	if value == "_baseharbor" || strings.HasPrefix(value, "__baseharbor_") {
		return fmt.Errorf("required secret name %q uses a reserved BaseHarbor name", value)
	}
	for i, r := range value {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.'
		if !valid {
			return fmt.Errorf("required secret name %q contains unsupported characters", value)
		}
		if i == 0 && (r == '-' || r == '.') {
			return fmt.Errorf("required secret name %q must not start with a hyphen or dot", value)
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

func (m Manifest) YAML() string {
	var b strings.Builder
	fmt.Fprintf(&b, "version: %d\napp:\n  name: %s\n  environment: %s\nservices:\n  postgres:\n    enabled: %t\n  redis:\n    enabled: %t\n  secrets:\n    enabled: %t\n", m.Version, m.Name, m.Environment, m.Services.Postgres, m.Services.Redis, m.Services.Secrets)
	if len(m.Secrets.Required) > 0 {
		b.WriteString("secrets:\n  required:\n")
		for _, requirement := range m.Secrets.Required {
			fmt.Fprintf(&b, "    - name: %s\n", requirement.Name)
		}
	}
	return b.String()
}

// ParseYAML parses the intentionally small v1 manifest grammar without adding a runtime dependency.
func ParseYAML(input string) (Manifest, error) {
	var m Manifest
	section := ""
	service := ""
	secretSection := ""
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
			secretSection = ""
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
				secretSection = "required"
				continue
			}
			return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
		case 4:
			if section == "services" && service != "" {
				key, value, ok := strings.Cut(trim, ":")
				if !ok || key != "enabled" {
					return Manifest{}, fmt.Errorf("line %d: expected enabled: true|false", lineNo)
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
			if section == "secrets" && secretSection == "required" && strings.HasPrefix(trim, "- name:") {
				name := strings.TrimSpace(strings.TrimPrefix(trim, "- name:"))
				m.Secrets.Required = append(m.Secrets.Required, SecretRequirement{Name: name})
				continue
			}
			return Manifest{}, fmt.Errorf("line %d: invalid manifest structure", lineNo)
		default:
			return Manifest{}, fmt.Errorf("line %d: indentation must use 0, 2 or 4 spaces", lineNo)
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
