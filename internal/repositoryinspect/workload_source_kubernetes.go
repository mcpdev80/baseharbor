package repositoryinspect

import (
	"fmt"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

func normalizeKubernetesSource(snapshot Snapshot, candidate WorkloadSourceCandidate) ([]WorkloadComponent, []WorkloadSourceReference, error) {
	var components []WorkloadComponent
	var opaque []WorkloadSourceReference
	exposureByComponent := map[string][]string{}
	for _, path := range candidate.Evidence {
		data, ok := snapshot.Files[path]
		if !ok {
			continue
		}
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		for docIndex := 0; ; docIndex++ {
			if docIndex >= maxKubernetesDocumentsPerFile {
				return nil, nil, fmt.Errorf("Kubernetes YAML %s exceeds %d documents", path, maxKubernetesDocumentsPerFile)
			}
			var obj map[string]any
			err := dec.Decode(&obj)
			if err != nil {
				if strings.Contains(err.Error(), "EOF") {
					break
				}
				return nil, nil, fmt.Errorf("decode Kubernetes YAML %s: %w", path, err)
			}
			if len(obj) == 0 {
				continue
			}
			kind := stringMapValue(obj, "kind")
			name := nestedString(obj, "metadata", "name")
			if kind == "" || name == "" {
				continue
			}
			resource := kind + "/" + name
			switch kind {
			case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob":
				component := WorkloadComponent{
					ID:     normalizeLogicalComponentID(name),
					Source: []WorkloadSourceReference{{Kind: WorkloadSourceKubernetes, Path: path, Resource: resource}},
				}
				podSpec := kubernetesPodSpec(obj, kind)
				containers, _ := podSpec["containers"].([]any)
				for _, raw := range containers {
					container, _ := raw.(map[string]any)
					if component.Image == "" {
						component.Image = stringMapValue(container, "image")
					}
					if ports, ok := container["ports"].([]any); ok {
						for _, p := range ports {
							pm, _ := p.(map[string]any)
							if value := intLikeString(pm["containerPort"]); value != "" {
								component.Ports = append(component.Ports, value)
							}
						}
					}
					if container["readinessProbe"] != nil || container["livenessProbe"] != nil || container["startupProbe"] != nil {
						component.Health = true
					}
					if envFrom, ok := container["envFrom"].([]any); ok {
						for _, e := range envFrom {
							em, _ := e.(map[string]any)
							if ref := nestedString(em, "configMapRef", "name"); ref != "" {
								component.ConfigRefs = append(component.ConfigRefs, "ConfigMap/"+ref)
							}
							if ref := nestedString(em, "secretRef", "name"); ref != "" {
								component.ConfigRefs = append(component.ConfigRefs, "Secret/"+ref)
							}
						}
					}
					if env, ok := container["env"].([]any); ok {
						for _, e := range env {
							em, _ := e.(map[string]any)
							name := stringMapValue(em, "name")
							if ref := nestedString(em, "valueFrom", "configMapKeyRef", "name"); ref != "" {
								component.ConfigRefs = append(component.ConfigRefs, "ConfigMap/"+ref)
								if name != "" {
									component.EnvironmentRefs = append(component.EnvironmentRefs, name)
								}
							}
							if ref := nestedString(em, "valueFrom", "secretKeyRef", "name"); ref != "" {
								component.ConfigRefs = append(component.ConfigRefs, "Secret/"+ref)
								if name != "" {
									component.EnvironmentRefs = append(component.EnvironmentRefs, name)
								}
							}
						}
					}
				}
				if vols, ok := podSpec["volumes"].([]any); ok {
					for _, v := range vols {
						vm, _ := v.(map[string]any)
						if claim := nestedString(vm, "persistentVolumeClaim", "claimName"); claim != "" {
							component.PersistentStorage = append(component.PersistentStorage, "PersistentVolumeClaim/"+claim)
						}
						if ref := nestedString(vm, "configMap", "name"); ref != "" {
							component.ConfigRefs = append(component.ConfigRefs, "ConfigMap/"+ref)
						}
						if ref := nestedString(vm, "secret", "secretName"); ref != "" {
							component.ConfigRefs = append(component.ConfigRefs, "Secret/"+ref)
						}
					}
				}
				component.InfrastructureClass = classifyInfrastructure(component.ID, component.Image)
				components = append(components, component)
			case "Service", "Ingress", "Gateway", "HTTPRoute", "ConfigMap", "Secret", "PersistentVolumeClaim":
				opaque = append(opaque, WorkloadSourceReference{Kind: WorkloadSourceKubernetes, Path: path, Resource: resource})
				for componentID, evidence := range kubernetesExposureBindings(obj, kind, name) {
					exposureByComponent[componentID] = append(exposureByComponent[componentID], evidence...)
				}
			default:
				opaque = append(opaque, WorkloadSourceReference{Kind: WorkloadSourceKubernetes, Path: path, Resource: resource})
			}
		}
	}
	components = mergeComponents(components)
	for i := range components {
		components[i].ConfigRefs = uniqueSorted(components[i].ConfigRefs)
		components[i].EnvironmentRefs = uniqueSorted(components[i].EnvironmentRefs)
		components[i].PersistentStorage = uniqueSorted(components[i].PersistentStorage)
		components[i].Ports = uniqueSorted(components[i].Ports)
		components[i].Exposure = uniqueSorted(append(components[i].Exposure, exposureByComponent[components[i].ID]...))
	}
	return components, opaque, nil
}

func kubernetesExposureBindings(obj map[string]any, kind, name string) map[string][]string {
	result := map[string][]string{}
	add := func(component, evidence string) {
		component = normalizeLogicalComponentID(component)
		if component == "" || evidence == "" {
			return
		}
		result[component] = append(result[component], evidence)
	}
	switch kind {
	case "Service":
		component := name
		if selector := nestedMap(obj, "spec", "selector"); selector != nil {
			for _, key := range []string{"app.kubernetes.io/name", "app", "component"} {
				if value := stringMapValue(selector, key); value != "" {
					component = value
					break
				}
			}
		}
		var ports []string
		if spec := nestedMap(obj, "spec"); spec != nil {
			if rawPorts, ok := spec["ports"].([]any); ok {
				for _, raw := range rawPorts {
					pm, _ := raw.(map[string]any)
					port := intLikeString(pm["port"])
					if port != "" {
						ports = append(ports, port)
					}
				}
			}
		}
		if len(ports) == 0 {
			add(component, "Service/"+name)
		} else {
			for _, port := range ports {
				add(component, "Service/"+name+":"+port)
			}
		}
	case "Ingress":
		spec := nestedMap(obj, "spec")
		if spec == nil {
			break
		}
		if backend := nestedString(spec, "defaultBackend", "service", "name"); backend != "" {
			add(backend, "Ingress/"+name)
		}
		if rules, ok := spec["rules"].([]any); ok {
			for _, rawRule := range rules {
				rule, _ := rawRule.(map[string]any)
				http := nestedMap(rule, "http")
				if http == nil {
					continue
				}
				paths, _ := http["paths"].([]any)
				for _, rawPath := range paths {
					path, _ := rawPath.(map[string]any)
					if backend := nestedString(path, "backend", "service", "name"); backend != "" {
						add(backend, "Ingress/"+name)
					}
				}
			}
		}
	case "HTTPRoute":
		spec := nestedMap(obj, "spec")
		if spec == nil {
			break
		}
		rules, _ := spec["rules"].([]any)
		for _, rawRule := range rules {
			rule, _ := rawRule.(map[string]any)
			refs, _ := rule["backendRefs"].([]any)
			for _, rawRef := range refs {
				ref, _ := rawRef.(map[string]any)
				if backend := stringMapValue(ref, "name"); backend != "" {
					add(backend, "HTTPRoute/"+name)
				}
			}
		}
	}
	for key := range result {
		result[key] = uniqueSorted(result[key])
	}
	return result
}

func kubernetesPodSpec(obj map[string]any, kind string) map[string]any {
	if kind == "CronJob" {
		if m := nestedMap(obj, "spec", "jobTemplate", "spec", "template", "spec"); m != nil {
			return m
		}
		return map[string]any{}
	}
	if m := nestedMap(obj, "spec", "template", "spec"); m != nil {
		return m
	}
	return map[string]any{}
}


func looksLikeKubernetesYAML(path string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" {
		return false
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var obj map[string]any
		err := dec.Decode(&obj)
		if err != nil {
			return containsKubernetesDocumentMarkers(data)
		}
		if len(obj) == 0 {
			continue
		}
		if stringMapValue(obj, "apiVersion") != "" && stringMapValue(obj, "kind") != "" {
			return true
		}
		return false
	}
}

func containsKubernetesDocumentMarkers(data []byte) bool {
	text := "\n" + strings.ToLower(string(data)) + "\n"
	return strings.Contains(text, "\napiversion:") && strings.Contains(text, "\nkind:")
}

