package kubernetes

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/workload"
	"gopkg.in/yaml.v3"
)

type Binding struct {
	Value     string
	Sensitive bool
}

type Plan struct {
	Application string
	Environment string
	Namespace   string
	Workload    workload.Model
	Images      map[string]string
	Bindings    map[string]Binding
}

func Render(plan Plan) ([]byte, error) {
	if strings.TrimSpace(plan.Application) == "" {
		return nil, errors.New("Kubernetes application name is required")
	}
	if strings.TrimSpace(plan.Environment) == "" {
		return nil, errors.New("Kubernetes environment is required")
	}
	if strings.TrimSpace(plan.Namespace) == "" {
		return nil, errors.New("Kubernetes namespace is required")
	}
	if len(plan.Workload.Services) == 0 {
		return nil, errors.New("Kubernetes workload contains no services")
	}

	var documents []string
	for _, service := range plan.Workload.Services {
		rendered, err := renderService(plan, service)
		if err != nil {
			return nil, err
		}
		for _, object := range rendered {
			data, err := yaml.Marshal(object)
			if err != nil {
				return nil, fmt.Errorf("marshal Kubernetes resource: %w", err)
			}
			documents = append(documents, string(data))
		}
	}
	return []byte(strings.Join(documents, "---\n")), nil
}

func renderService(plan Plan, service workload.Service) ([]map[string]any, error) {
	if strings.TrimSpace(service.Name) == "" {
		return nil, errors.New("Kubernetes workload service name is required")
	}

	image := strings.TrimSpace(plan.Images[service.Name])
	if image == "" {
		image = strings.TrimSpace(service.Image)
	}
	if image == "" {
		if service.Build != nil {
			return nil, fmt.Errorf(
				"workload service %q is source/build-backed; resolve it to an OCI image artifact before Kubernetes realization",
				service.Name,
			)
		}
		return nil, fmt.Errorf("workload service %q does not define an OCI image artifact", service.Name)
	}

	name := dnsLabel(plan.Application + "-" + service.Name)
	labels := ownershipLabels(plan.Application, plan.Environment)
	labels["app.kubernetes.io/name"] = name
	labels["baseharbor.io/workload-service"] = service.Name

	configData, secretData, err := resolvedEnvironment(service.Environment, plan.Bindings)
	if err != nil {
		return nil, fmt.Errorf("service %s environment: %w", service.Name, err)
	}

	configName := name + "-config"
	secretName := name + "-bindings"

	var objects []map[string]any
	if len(configData) > 0 {
		objects = append(objects, map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      configName,
				"namespace": plan.Namespace,
				"labels":    cloneMap(labels),
			},
			"data": sortedMap(configData),
		})
	}
	if len(secretData) > 0 {
		objects = append(objects, map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "Opaque",
			"metadata": map[string]any{
				"name":      secretName,
				"namespace": plan.Namespace,
				"labels":    cloneMap(labels),
			},
			"stringData": sortedMap(secretData),
		})
	}

	container := map[string]any{
		"name":            dnsLabel(service.Name),
		"image":           image,
		"imagePullPolicy": "IfNotPresent",
	}
	if len(service.Command) > 0 {
		container["command"] = append([]string(nil), service.Command...)
	}
	if len(service.Ports) > 0 {
		var ports []any
		for _, port := range service.Ports {
			protocol := strings.ToUpper(strings.TrimSpace(port.Protocol))
			if protocol == "" {
				protocol = "TCP"
			}
			ports = append(ports, map[string]any{
				"name":          portName(port.Container, protocol),
				"containerPort": port.Container,
				"protocol":      protocol,
			})
		}
		container["ports"] = ports
	}
	var envFrom []any
	if len(configData) > 0 {
		envFrom = append(envFrom, map[string]any{"configMapRef": map[string]any{"name": configName}})
	}
	if len(secretData) > 0 {
		envFrom = append(envFrom, map[string]any{"secretRef": map[string]any{"name": secretName}})
	}
	if len(envFrom) > 0 {
		container["envFrom"] = envFrom
	}

	deployment := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": plan.Namespace,
			"labels":    cloneMap(labels),
		},
		"spec": map[string]any{
			"replicas": 1,
			"selector": map[string]any{
				"matchLabels": map[string]any{"app.kubernetes.io/name": name},
			},
			"template": map[string]any{
				"metadata": map[string]any{"labels": cloneMap(labels)},
				"spec": map[string]any{"containers": []any{container}},
			},
		},
	}
	objects = append(objects, deployment)

	if len(service.Ports) > 0 {
		var servicePorts []any
		for _, port := range service.Ports {
			protocol := strings.ToUpper(strings.TrimSpace(port.Protocol))
			if protocol == "" {
				protocol = "TCP"
			}
			servicePorts = append(servicePorts, map[string]any{
				"name":       portName(port.Container, protocol),
				"port":       port.Container,
				"targetPort": port.Container,
				"protocol":   protocol,
			})
		}
		objects = append(objects, map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]any{
				"name":      name,
				"namespace": plan.Namespace,
				"labels":    cloneMap(labels),
			},
			"spec": map[string]any{
				"selector": map[string]any{"app.kubernetes.io/name": name},
				"ports":    servicePorts,
				"type":     "ClusterIP",
			},
		})
	}

	return objects, nil
}

func resolvedEnvironment(source map[string]string, bindings map[string]Binding) (map[string]string, map[string]string, error) {
	config := map[string]string{}
	secret := map[string]string{}

	keys := make([]string, 0, len(source)+len(bindings))
	seen := map[string]struct{}{}
	for key := range source {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range bindings {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	for _, key := range keys {
		if binding, ok := bindings[key]; ok {
			if binding.Sensitive {
				secret[key] = binding.Value
			} else {
				config[key] = binding.Value
			}
			continue
		}
		value, err := expandComposeDefault(source[key])
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", key, err)
		}
		config[key] = value
	}
	return config, secret, nil
}

func expandComposeDefault(value string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return "", fmt.Errorf("unsupported Compose interpolation %q", value)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
	if _, fallback, ok := strings.Cut(inner, ":-"); ok {
		return fallback, nil
	}
	if _, fallback, ok := strings.Cut(inner, "-"); ok {
		return fallback, nil
	}
	return "", fmt.Errorf("unresolved Compose interpolation %q requires a runtime binding", value)
}

func ownershipLabels(application, environment string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "baseharbor",
		"baseharbor.io/application":    dnsLabel(application),
		"baseharbor.io/environment":    dnsLabel(environment),
	}
}

func dnsLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 63 {
		result = strings.Trim(result[:63], "-")
	}
	if result == "" {
		return "baseharbor"
	}
	return result
}

func portName(port int, protocol string) string {
	return dnsLabel(strings.ToLower(protocol) + "-" + strconv.Itoa(port))
}

func sortedMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = values[key]
	}
	return result
}

func cloneMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
