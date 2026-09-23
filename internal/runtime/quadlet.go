package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type QuadletWorkload struct {
	ServiceName   string
	UnitBase      string
	Build         string
	Container     string
	Environment   string
	ServiceUnit   string
	ContainerName string
}

type composeWorkloadFile struct {
	Services map[string]composeWorkloadService `yaml:"services"`
}

type composeWorkloadService struct {
	Image       string            `yaml:"image"`
	Build       composeBuild      `yaml:"build"`
	Ports       []string          `yaml:"ports"`
	Environment composeEnv        `yaml:"environment"`
	Restart     string            `yaml:"restart"`
	Command     any               `yaml:"command"`
	Volumes     []string          `yaml:"volumes"`
	Networks    any               `yaml:"networks"`
	Profiles    []string          `yaml:"profiles"`
}

type composeBuild struct {
	Context    string
	Dockerfile string
}

func (b *composeBuild) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		b.Context = node.Value
		return nil
	case yaml.MappingNode:
		type plain composeBuild
		var value plain
		if err := node.Decode(&value); err != nil {
			return err
		}
		*b = composeBuild(value)
		return nil
	case 0:
		return nil
	default:
		return errors.New("unsupported Compose build syntax")
	}
}

type composeEnv map[string]string

func (e *composeEnv) UnmarshalYAML(node *yaml.Node) error {
	result := map[string]string{}
	switch node.Kind {
	case 0:
		*e = result
		return nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := strings.TrimSpace(node.Content[i].Value)
			if key == "" {
				continue
			}
			var value string
			if node.Content[i+1].Tag == "!!null" {
				value = os.Getenv(key)
			} else {
				value = node.Content[i+1].Value
			}
			result[key] = value
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			key, value, found := strings.Cut(item.Value, "=")
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if !found {
				value = os.Getenv(key)
			}
			result[key] = value
		}
	default:
		return errors.New("unsupported Compose environment syntax")
	}
	*e = result
	return nil
}

var composeDefaultEnv = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)[:-]-(.*)\}$`)
var safeUnitName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func RenderComposeServiceQuadlet(composePath, serviceName, unitPrefix string) (QuadletWorkload, error) {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return QuadletWorkload{}, err
	}
	var model composeWorkloadFile
	if err := yaml.Unmarshal(data, &model); err != nil {
		return QuadletWorkload{}, fmt.Errorf("decode Compose workload: %w", err)
	}
	service, ok := model.Services[serviceName]
	if !ok {
		return QuadletWorkload{}, fmt.Errorf("Compose service %q not found", serviceName)
	}
	if len(service.Volumes) > 0 || service.Networks != nil || service.Command != nil {
		return QuadletWorkload{}, fmt.Errorf("Compose service %q uses PoC-unsupported workload features", serviceName)
	}

	unitBase := sanitizeQuadletName(strings.TrimSpace(unitPrefix) + "-" + serviceName)
	if unitBase == "" {
		return QuadletWorkload{}, errors.New("Quadlet unit name is empty")
	}

	var build string
	image := strings.TrimSpace(service.Image)
	if strings.TrimSpace(service.Build.Context) != "" {
		contextDir := service.Build.Context
		if !filepath.IsAbs(contextDir) {
			contextDir = filepath.Join(filepath.Dir(composePath), contextDir)
		}
		contextDir, err = filepath.Abs(contextDir)
		if err != nil {
			return QuadletWorkload{}, err
		}
		dockerfile := strings.TrimSpace(service.Build.Dockerfile)
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		build = fmt.Sprintf("[Unit]\nDescription=BaseHarbor Quadlet build for %s\n\n[Build]\nImageTag=localhost/%s:quadlet-poc\nSetWorkingDirectory=%s\nFile=%s\n\n[Service]\nTimeoutStartSec=900\n", serviceName, unitBase, systemdEscapeValue(contextDir), systemdEscapeValue(dockerfile))
		image = unitBase + ".build"
	}
	if image == "" {
		return QuadletWorkload{}, fmt.Errorf("Compose service %q has neither image nor build", serviceName)
	}

	envKeys := make([]string, 0, len(service.Environment))
	for key := range service.Environment {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	var env strings.Builder
	for _, key := range envKeys {
		fmt.Fprintf(&env, "%s=%s\n", key, resolveComposeEnvValue(service.Environment[key]))
	}

	var container strings.Builder
	fmt.Fprintf(&container, "[Unit]\nDescription=BaseHarbor Quadlet workload %s\n\n[Container]\nImage=%s\nContainerName=%s\n", serviceName, image, unitBase)
	if len(envKeys) > 0 {
		container.WriteString("EnvironmentFile=./" + unitBase + ".env\n")
	}
	for _, port := range service.Ports {
		fmt.Fprintf(&container, "PublishPort=%s\n", strings.TrimSpace(port))
	}
	container.WriteString("\n[Service]\n")
	switch strings.ToLower(strings.TrimSpace(service.Restart)) {
	case "always", "unless-stopped":
		container.WriteString("Restart=always\n")
	case "on-failure":
		container.WriteString("Restart=on-failure\n")
	case "", "no":
		container.WriteString("Restart=no\n")
	default:
		return QuadletWorkload{}, fmt.Errorf("Compose restart policy %q is unsupported", service.Restart)
	}
	container.WriteString("TimeoutStartSec=900\n")

	return QuadletWorkload{
		ServiceName: serviceName,
		UnitBase: unitBase,
		Build: build,
		Container: container.String(),
		Environment: env.String(),
		ServiceUnit: unitBase + ".service",
		ContainerName: unitBase,
	}, nil
}

func WriteQuadletWorkload(dir string, workload QuadletWorkload) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if workload.Build != "" {
		if err := os.WriteFile(filepath.Join(dir, workload.UnitBase+".build"), []byte(workload.Build), 0o600); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, workload.UnitBase+".container"), []byte(workload.Container), 0o600); err != nil {
		return err
	}
	if workload.Environment != "" {
		if err := os.WriteFile(filepath.Join(dir, workload.UnitBase+".env"), []byte(workload.Environment), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeQuadletName(value string) string {
	value = safeUnitName.ReplaceAllString(strings.TrimSpace(value), "-")
	return strings.Trim(value, "-.")
}

func resolveComposeEnvValue(value string) string {
	value = strings.TrimSpace(value)
	match := composeDefaultEnv.FindStringSubmatch(value)
	if len(match) == 3 {
		if current, ok := os.LookupEnv(match[1]); ok && current != "" {
			return current
		}
		return match[2]
	}
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		key := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
		return os.Getenv(key)
	}
	return value
}

func systemdEscapeValue(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}
