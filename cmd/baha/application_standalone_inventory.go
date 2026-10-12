package main

import (
	"errors"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	tracesprovider "github.com/mcpdev80/baseharbor/internal/traces"
)

type standaloneProviderInventory struct {
	project, dir string
	resources    []bhruntime.ProjectResource
	dependencies []string
}

func standaloneProviderKind(kind capability.ProviderKind) bool {
	switch kind {
	case capability.ProviderPrometheus, capability.ProviderLoki, capability.ProviderTempo, capability.ProviderOTelCollector, capability.ProviderSeaweedFS:
		return true
	}
	return false
}

// Consume the retained deployment, registry placement and existing provider
// renderer. This adapter does not resolve new offers or persist another owner.
func existingStandaloneProviderInventory(resolved resolvedApplication, provider bhruntime.RuntimeProvider, instance capability.ProviderInstance, registry capability.Registry) (standaloneProviderInventory, error) {
	result := standaloneProviderInventory{dependencies: []string{}}
	if instance.Ownership != capability.OwnershipBaseHarbor || instance.Scope == capability.ScopeExternal {
		return result, errors.New("standalone provider owner is external or unverified")
	}
	m := resolved.Manifest
	if instance.Scope == capability.ScopeApplication && (instance.OwnerApplicationID != m.ApplicationID || instance.OwnerEnvironment != m.Environment) {
		return result, errors.New("standalone provider differs from retained application owner")
	}
	root, namespace := resolved.TargetStateRoot, resolved.Target.Name
	p := providerInstancePlacement(instance)
	var err error
	switch instance.Provider.Kind {
	case capability.ProviderPrometheus:
		files, placement, resources, inventoryErr := metricsprovider.ExistingRuntimeInventoryAt(root, namespace, m, p)
		result.project, result.dir, result.resources, err = placement.Project, files.Dir, resources, inventoryErr
	case capability.ProviderLoki:
		files, placement, resources, inventoryErr := logsprovider.ExistingRuntimeInventoryAt(root, namespace, m, p, provider.LogCollectionMode())
		result.project, result.dir, result.resources, err = placement.Project, files.Dir, resources, inventoryErr
	case capability.ProviderTempo:
		files, placement, resources, inventoryErr := tracesprovider.ExistingRuntimeInventoryAt(root, namespace, m, p)
		result.project, result.dir, result.resources, err = placement.Project, files.Dir, resources, inventoryErr
	case capability.ProviderSeaweedFS:
		// The supported native storage adapter has one shared placement.
		if p.Scope != capability.ScopeShared {
			return result, errors.New("SeaweedFS native placement is not shared")
		}
		files, resources, inventoryErr := objectstorage.ExistingRuntimeInventoryAt(root, namespace)
		result.project, result.dir, result.resources, err = files.Project, files.Dir, resources, inventoryErr
	case capability.ProviderOTelCollector:
		if p.Scope != capability.ScopeShared {
			return result, errors.New("Collector native placement is not shared")
		}
		traceNetwork, dependency, bindingErr := retainedCollectorTraceBinding(resolved, instance, registry)
		if bindingErr != nil {
			return result, bindingErr
		}
		files, resources, inventoryErr := telemetry.ExistingRuntimeInventoryAt(root, namespace, traceNetwork)
		result.project, result.dir, result.resources, err = files.Project, files.Dir, resources, inventoryErr
		if dependency != "" {
			result.dependencies = append(result.dependencies, dependency)
		}
	default:
		return result, errors.New("standalone inventory adapter is unavailable")
	}
	if err != nil {
		return result, err
	}
	if instance.Provider.Kind == capability.ProviderLoki || instance.Provider.Kind == capability.ProviderTempo {
		services, err := providertopology.ServiceNames(filepath.Join(result.dir, "compose.yaml"))
		if err != nil {
			return result, err
		}
		_, lokiHA := services["loki-1"]
		_, tempoHA := services["tempo-distributor-1"]
		if lokiHA || tempoHA {
			// HA renders an external shared storage network. Associate it only
			// with a unique existing registered owner, never a kind-only guess.
			id := ""
			for _, candidate := range registry.Instances {
				if candidate.Provider.Kind == capability.ProviderSeaweedFS && candidate.Scope == capability.ScopeShared && candidate.Ownership == capability.OwnershipBaseHarbor {
					if id != "" {
						return result, errors.New("HA storage dependency is ambiguous")
					}
					id = candidate.ID
				}
			}
			if id == "" {
				return result, errors.New("HA storage has no authoritative registered consumer dependency")
			}
			result.dependencies = append(result.dependencies, id)
		}
	}
	return result, nil
}

func retainedCollectorTraceBinding(resolved resolvedApplication, collector capability.ProviderInstance, registry capability.Registry) (string, string, error) {
	selected := map[string]bool{}
	for _, consumer := range registry.BindingsForProviderInstance(collector.ID) {
		for _, binding := range registry.Bindings {
			if binding.ApplicationID == consumer.ApplicationID && binding.Environment == consumer.Environment && binding.Resource.Kind == capability.Traces {
				selected[binding.ProviderInstanceID] = true
			}
		}
	}
	if len(selected) > 1 {
		return "", "", errors.New("shared Collector trace consumers have conflicting bindings")
	}
	for _, instance := range registry.Instances {
		if !selected[instance.ID] {
			continue
		}
		if instance.Provider.Kind != capability.ProviderTempo {
			return "", "", errors.New("Collector trace continuation adapter is unavailable")
		}
		m := resolved.Manifest
		if instance.Scope == capability.ScopeApplication {
			m = application.Manifest{Version: application.CurrentVersion, ApplicationID: instance.OwnerApplicationID, Name: instance.OwnerApplication, Environment: instance.OwnerEnvironment}
		}
		_, placement, _, err := tracesprovider.ExistingRuntimeInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, m, providerInstancePlacement(instance))
		return placement.Network, instance.ID, err
	}
	if len(selected) != 0 {
		return "", "", errors.New("Collector trace binding has no retained owner")
	}
	// Released bindings do not erase protected runtime configuration. For a
	// retained applied/retired intent, recover only a UNIQUE exact canonical
	// Collector -> Tempo network match, without selecting or persisting an offer.
	enabled, err := application.TracesCollectionEnabled(resolved.Manifest)
	if err != nil {
		return "", "", err
	}
	if !enabled {
		return "", "", nil
	}
	network, dependency := "", ""
	for _, instance := range registry.Instances {
		if instance.Provider.Kind != capability.ProviderTempo || instance.Ownership != capability.OwnershipBaseHarbor {
			continue
		}
		if instance.Scope == capability.ScopeApplication && (instance.OwnerApplicationID != resolved.Manifest.ApplicationID || instance.OwnerEnvironment != resolved.Manifest.Environment) {
			continue
		}
		_, placement, _, err := tracesprovider.ExistingRuntimeInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, resolved.Manifest, providerInstancePlacement(instance))
		if err != nil {
			continue
		}
		if _, _, err := telemetry.ExistingRuntimeInventoryAt(resolved.TargetStateRoot, resolved.Target.Name, placement.Network); err != nil {
			continue
		}
		if dependency != "" {
			return "", "", errors.New("retained Collector trace network owner is ambiguous")
		}
		network, dependency = placement.Network, instance.ID
	}
	if dependency == "" {
		return "", "", errors.New("retained Collector trace export has no verifiable canonical owner")
	}
	return network, dependency, nil
}
