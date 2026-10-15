package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// RuntimeDependencyRoots projects the existing native broker realization into
// the common resolver. Consumption bindings remain unknown until their actual
// delivery dependencies are available; no capability or provider is inferred.
func RuntimeDependencyRoots(m Manifest, registry capability.Registry) ([]string, error) {
	if len(m.Consumes) > 0 {
		return nil, nil
	}
	roots := []string{}
	if !RequiresRuntimeBroker(m) {
		return roots, nil
	}
	resource := capability.Resource{Application: m.Name, Kind: capability.Secrets, Name: "default", Provider: capability.ProviderOpenBao}
	expected, err := referenceProviderInstance(m, resource)
	if err != nil {
		return nil, err
	}
	for _, instance := range registry.Instances {
		if instance.ID == expected.ID && instance.Provider.Kind == capability.ProviderOpenBao && instance.Ownership == capability.OwnershipBaseHarbor {
			return append(roots, instance.ID), nil
		}
	}
	return nil, fmt.Errorf("runtime broker dependency %q is not registered", expected.ID)
}
