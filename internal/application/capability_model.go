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
	for _, resource := range resources {
		workload := "application/" + contract.Application
		if resource.Kind == capability.ExposureHTTP {
			for _, exposure := range contract.Exposures {
				if exposure.Name == resource.Name {
					workload = "service/" + exposure.Service
					break
				}
			}
		}
		bindings = append(bindings, capability.Binding{Resource: resource, Workload: workload})
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
	default:
		return capability.Provider{}, fmt.Errorf("unsupported application capability %q", kind)
	}
}
