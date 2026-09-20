package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const defaultCapabilityResource = "default"

func ResolveCapabilityResources(contract PortableContract) ([]capability.Resource, error) {
	resources := make([]capability.Resource, 0, len(contract.Capabilities)+1)
	for _, requirement := range contract.Capabilities {
		provider, err := referenceCapabilityProvider(requirement.Kind)
		if err != nil {
			return nil, err
		}
		resource, err := capability.Resolve(contract.Application, requirement, provider)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	if contract.Secrets.Managed {
		resource, err := capability.Resolve(contract.Application, capability.Requirement{
			Kind: capability.Secrets,
			Name: defaultCapabilityResource,
		}, capability.OpenBao)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func CapabilityBindings(m Manifest) ([]capability.Binding, error) {
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		return nil, err
	}
	resources, err := ResolveCapabilityResources(contract)
	if err != nil {
		return nil, err
	}
	bindings := make([]capability.Binding, 0, len(resources))
	metricsEnabled, err := MetricsCollectionEnabled(m)
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		if resource.Kind == capability.Metrics && !metricsEnabled {
			continue
		}
		workload := "application/" + contract.Application
		binding := capability.Binding{Resource: resource, Workload: workload}
		if resource.Kind == capability.ObjectStorageS3 {
			binding.ObjectStorageS3 = &capability.ObjectStorageS3Binding{Bucket: resource.Name}
			security := ObjectStorageSecureBinding(m, resource.Name)
			if err := security.Validate(); err != nil {
				return nil, fmt.Errorf("build object-storage secure binding: %w", err)
			}
			binding.Security = &security
		}
		if resource.Kind == capability.TelemetryOTLP && m.Telemetry.OTLP != nil {
			binding.TelemetryOTLP = &capability.OTLPTelemetryBinding{Direction: "export", Protocol: "http/protobuf", Signals: append([]string(nil), m.Telemetry.OTLP.Signals...)}
		}
		if resource.Kind == capability.Metrics {
			for _, source := range m.Metrics.Sources {
				if source.Name == resource.Name {
					binding.Workload = "service/" + source.Service
					binding.Metrics = &capability.MetricsBinding{
						Direction: "provide",
						Format:    "openmetrics",
						Service:   source.Service,
						Port:      source.Port,
						Path:      source.Path,
					}
					break
				}
			}
		}
		if resource.Kind == capability.Logs {
			binding.Workload = "service/" + resource.Name
			binding.Logs = &capability.LogsBinding{
				Direction: "collect",
				Format:    "syslog-rfc5424",
				Service:   resource.Name,
			}
		}
		if resource.Kind == capability.Secrets {
			security := ManagedSecretsSecureBinding(m)
			if err := security.Validate(); err != nil {
				return nil, fmt.Errorf("build managed-secrets secure binding: %w", err)
			}
			binding.Security = &security
		}
		if resource.Kind == capability.ExposureHTTP {
			for _, exposure := range contract.Exposures {
				if exposure.Name == resource.Name {
					binding.Workload = "service/" + exposure.Service
					binding.HTTPExposure = &capability.HTTPExposureBinding{
						Service: exposure.Service, TargetPort: exposure.Port,
						Protocol: exposure.Protocol, Visibility: normalizedExposureVisibility(exposure.Visibility),
					}
					break
				}
			}
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func referenceCapabilityProvider(kind capability.Kind) (capability.Provider, error) {
	switch kind {
	case capability.SQL:
		return capability.PostgreSQL, nil
	case capability.KeyValue:
		return capability.Valkey, nil
	case capability.ExposureHTTP:
		return capability.Caddy, nil
	case capability.ObjectStorageS3:
		return capability.SeaweedFS, nil
	case capability.TelemetryOTLP:
		return TelemetryProviderForDeployment(), nil
	case capability.Metrics:
		return capability.Prometheus, nil
	case capability.Logs:
		return capability.Loki, nil
	default:
		return capability.Provider{}, fmt.Errorf("unsupported application capability %q", kind)
	}
}
