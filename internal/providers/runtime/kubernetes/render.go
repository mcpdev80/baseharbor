package kubernetes

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
	"go.yaml.in/yaml/v3"
	"net/url"
)

type Binding = runtimemodel.Binding
type Plan = runtimemodel.WorkloadPlan

func Render(plan Plan) ([]byte, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(plan.Application) == "" {
		return nil, errors.New("Kubernetes application name is required")
	}
	if strings.TrimSpace(plan.Environment) == "" {
		return nil, errors.New("Kubernetes environment is required")
	}
	if strings.TrimSpace(plan.Target.Scope) == "" {
		return nil, errors.New("Kubernetes runtime target scope (namespace) is required")
	}
	if len(plan.Services) == 0 {
		return nil, errors.New("Kubernetes workload contains no services")
	}

	var documents []string
	for _, service := range plan.Services {
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

func renderService(plan Plan, service runtimemodel.Service) ([]map[string]any, error) {
	if strings.TrimSpace(service.Name) == "" {
		return nil, errors.New("Kubernetes workload service name is required")
	}

	image := strings.TrimSpace(service.Image)

	name := dnsLabel(plan.Application + "-" + service.Name)
	labels := ownershipLabels(plan.Application, plan.Environment)
	labels["app.kubernetes.io/name"] = name
	labels["baseharbor.io/workload-service"] = workloadServiceLabel(service.Name)
	annotations := map[string]string{
		"baseharbor.io/workload-service-name": service.Name,
	}

	configData, secretData, err := resolvedEnvironment(service.Environment, plan.Bindings, plan.Target.Scope)
	if err != nil {
		return nil, fmt.Errorf("service %s environment: %w", service.Name, err)
	}

	configName := name + "-config"

	var objects []map[string]any
	if len(configData) > 0 {
		objects = append(objects, map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":        configName,
				"namespace":   plan.Target.Scope,
				"labels":      cloneMap(labels),
				"annotations": cloneMap(annotations),
			},
			"data": sortedMap(configData),
		})
	}

	container := map[string]any{
		"name":            dnsLabel(service.Name),
		"image":           image,
		"imagePullPolicy": "IfNotPresent",
	}
	if len(service.Args) > 0 {
		container["args"] = append([]string(nil), service.Args...)
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
		keys := make([]string, 0, len(secretData))
		for key := range secretData {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var env []any
		for _, key := range keys {
			env = append(env, map[string]any{"name": key, "valueFrom": map[string]any{"secretKeyRef": secretData[key]}})
		}
		container["env"] = env
	}

	if len(envFrom) > 0 {
		container["envFrom"] = envFrom
	}

	deployment := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":        name,
			"namespace":   plan.Target.Scope,
			"labels":      cloneMap(labels),
			"annotations": cloneMap(annotations),
		},
		"spec": map[string]any{
			"replicas": 1,
			"selector": map[string]any{
				"matchLabels": map[string]any{"app.kubernetes.io/name": name},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels":      cloneMap(labels),
					"annotations": cloneMap(annotations),
				},
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
				"name":        name,
				"namespace":   plan.Target.Scope,
				"labels":      cloneMap(labels),
				"annotations": cloneMap(annotations),
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

func resolvedEnvironment(source []runtimemodel.EnvironmentVariable, bindings []Binding, namespace string) (map[string]string, map[string]map[string]string, error) {
	config := map[string]string{}
	secret := map[string]map[string]string{}
	for _, env := range source {
		if strings.Contains(env.Value, "${") {
			return nil, nil, fmt.Errorf("environment %s is unresolved", env.Name)
		}
		config[env.Name] = env.Value
	}
	for _, binding := range bindings {
		if binding.SecretRef == "" {
			config[binding.Name] = binding.PublicValue
			continue
		}
		ref, err := nativeSecretReference(binding.SecretRef, namespace)
		if err != nil {
			return nil, nil, err
		}
		delete(config, binding.Name)
		secret[binding.Name] = ref
	}
	return config, secret, nil
}

// The runtime binding is opaque outside the provider. Native references use
// Kubernetes' existing namespaced Secret/key mechanism; values are never read
// into the portable plan or printed as rendered Secret objects.
func nativeSecretReference(reference, namespace string) (map[string]string, error) {
	u, err := url.Parse(reference)
	if err != nil || u.Scheme != "kubernetes" || u.Host != namespace || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("Kubernetes binding requires a supported same-namespace Secret reference")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || dnsLabel(parts[0]) != parts[0] {
		return nil, fmt.Errorf("invalid Kubernetes Secret reference")
	}
	for _, r := range parts[1] {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return nil, fmt.Errorf("invalid Kubernetes Secret key reference")
		}
	}
	return map[string]string{"name": parts[0], "key": parts[1]}, nil
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

func workloadServiceLabel(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.'
		if valid {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-_.")
	if len(result) > 63 {
		result = strings.Trim(result[:63], "-_.")
	}
	if result == "" {
		return "workload"
	}
	return result
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
