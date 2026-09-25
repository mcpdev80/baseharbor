package application

import (
	"fmt"
	"sort"
	"strings"
)

func (m Manifest) YAML() string {
	var b strings.Builder
	fmt.Fprintf(&b, "version: %d\napp:\n  name: %s\n  environment: %s\n", m.Version, m.Name, m.Environment)
	if hasManifestServices(m.Services) {
		b.WriteString("services:\n")
		if m.Services.SQL || len(m.Services.SQLInstances) > 0 {
			writeServiceYAML(&b, "sql", m.Services.SQL, m.Services.SQLInstances)
		}
		if m.Services.Cache || len(m.Services.CacheInstances) > 0 {
			writeServiceYAML(&b, "cache", m.Services.Cache, m.Services.CacheInstances)
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
	return services.SQL || services.Cache || services.Secrets || services.ObjectStorage ||
		len(services.SQLInstances) > 0 || len(services.CacheInstances) > 0 || len(services.ObjectStorageBuckets) > 0
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

func secretRequirementPointer(m *Manifest, field string, index int) *SecretRequirement {
	if index < 0 {
		return nil
	}
	switch field {
	case "required":
		if index >= len(m.Secrets.Required) {
			return nil
		}
		return &m.Secrets.Required[index]
	case "optional":
		if index >= len(m.Secrets.Optional) {
			return nil
		}
		return &m.Secrets.Optional[index]
	default:
		return nil
	}
}

// ParseYAML parses the intentionally small v1 manifest grammar without adding a runtime dependency.
