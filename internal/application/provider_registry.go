package application

import (
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const providerRegistryFile = "provider-registry.json"
const runtimeMetricsRegistryResource = "@runtime-sources"

func referenceProviderRegistryStore() (capability.RegistryStore, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return capability.RegistryStore{}, err
	}
	return capability.RegistryStore{Path: filepath.Join(dataDir, providerRegistryFile)}, nil
}

// CheckReferenceProviderRegistry validates the protected provider registry and
// simulates the desired application reconciliation without writing it. Lifecycle
// callers use this during preflight so invalid provider metadata fails before
// any runtime or workload mutation.
func CheckReferenceProviderRegistry(m Manifest) error {
	store, err := referenceProviderRegistryStore()
	if err != nil {
		return err
	}
	registry, err := store.Load()
	if err != nil {
		return err
	}
	registry.ReleaseManagedApplication(m.Name)
	if err := registerReferenceProviders(&registry, m); err != nil {
		return err
	}
	return registry.Validate()
}

func ReconcileReferenceProviderRegistry(m Manifest) error {
	store, err := referenceProviderRegistryStore()
	if err != nil {
		return err
	}
	return store.Update(func(registry *capability.Registry) error {
		registry.ReleaseManagedApplication(m.Name)
		return registerReferenceProviders(registry, m)
	})
}

func CheckControlPlaneDestroySafe() error {
	store, err := referenceProviderRegistryStore()
	if err != nil {
		return err
	}
	registry, err := store.Load()
	if err != nil {
		return err
	}
	if len(registry.Bindings) != 0 {
		return fmt.Errorf("provider registry still contains %d application binding(s); destroy managed applications before the global control plane", len(registry.Bindings))
	}
	for _, instance := range registry.Instances {
		if instance.Scope == capability.ScopeApplication {
			return fmt.Errorf("provider registry still contains application-scoped provider %q; destroy managed applications before the global control plane", instance.ID)
		}
	}
	return nil
}

func RegisteredProviderPlacement(m Manifest, provider capability.ProviderKind) (capability.ProviderPlacement, bool, error) {
	store, err := referenceProviderRegistryStore()
	if err != nil {
		return capability.ProviderPlacement{}, false, err
	}
	registry, err := store.Load()
	if err != nil {
		return capability.ProviderPlacement{}, false, err
	}

	var matched *capability.ProviderInstance
	for _, binding := range registry.Bindings {
		if binding.Resource.Application != m.Name || binding.Resource.Provider != provider {
			continue
		}
		var instance *capability.ProviderInstance
		for i := range registry.Instances {
			if registry.Instances[i].ID == binding.ProviderInstanceID {
				instance = &registry.Instances[i]
				break
			}
		}
		if instance == nil {
			return capability.ProviderPlacement{}, false, fmt.Errorf("provider registry binding references missing provider instance %q", binding.ProviderInstanceID)
		}
		if matched != nil && matched.ID != instance.ID {
			return capability.ProviderPlacement{}, false, fmt.Errorf("application %q has ambiguous provider placement for %q", m.Name, provider)
		}
		copy := *instance
		matched = &copy
	}
	if matched == nil {
		return capability.ProviderPlacement{}, false, nil
	}

	placement := capability.ProviderPlacement{
		Scope:             matched.Scope,
		SharingBoundary:   matched.SharingBoundary,
		Ownership:         matched.Ownership,
		ExternalReference: matched.Reference,
	}
	if err := placement.Validate(); err != nil {
		return capability.ProviderPlacement{}, false, fmt.Errorf("registered provider placement for %s: %w", provider, err)
	}
	return placement, true, nil
}

func ReleaseApplicationProviderRegistry(m Manifest) error {
	store, err := referenceProviderRegistryStore()
	if err != nil {
		return err
	}
	return store.Update(func(registry *capability.Registry) error {
		registry.ReleaseApplication(m.Name)
		return nil
	})
}

func registerReferenceProviders(registry *capability.Registry, m Manifest) error {
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		return err
	}
	resources, err := ResolveCapabilityResources(contract)
	if err != nil {
		return err
	}
	metricsPolicy, err := MetricsPolicy(m)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		if resource.Kind == capability.Metrics && !metricsPolicy.Enabled {
			continue
		}
		var instance capability.ProviderInstance
		switch resource.Provider {
		case capability.ProviderPostgreSQL:
			instance = capability.ProviderInstance{
				ID:               fmt.Sprintf("postgresql/%s/%s/%s", m.Name, m.Environment, resource.Name),
				Provider:         capability.PostgreSQL,
				Scope:            capability.ScopeApplication,
				Ownership:        capability.OwnershipBaseHarbor,
				OwnerApplication: m.Name,
			}
		case capability.ProviderValkey:
			instance = capability.ProviderInstance{
				ID:               fmt.Sprintf("valkey/%s/%s/%s", m.Name, m.Environment, resource.Name),
				Provider:         capability.Valkey,
				Scope:            capability.ScopeApplication,
				Ownership:        capability.OwnershipBaseHarbor,
				OwnerApplication: m.Name,
			}
		case capability.ProviderSeaweedFS:
			instance = capability.ProviderInstance{
				ID:        "seaweedfs/shared",
				Provider:  capability.SeaweedFS,
				Scope:     capability.ScopeShared,
				Ownership: capability.OwnershipBaseHarbor,
			}
		case capability.ProviderOTelCollector:
			instance = capability.ProviderInstance{
				ID:        "opentelemetry-collector/shared",
				Provider:  capability.OTelCollector,
				Scope:     capability.ScopeShared,
				Ownership: capability.OwnershipBaseHarbor,
			}
		case capability.ProviderPrometheus:
			instance, err = prometheusProviderInstance(m)
			if err != nil {
				return err
			}
		case capability.ProviderExternalOTLP:
			instance = capability.ProviderInstance{
				ID:        "external-otlp/default",
				Provider:  capability.ExternalOTLP,
				Scope:     capability.ScopeExternal,
				Ownership: capability.OwnershipExternal,
			}
		case capability.ProviderOpenBao:
			instance = capability.ProviderInstance{
				ID:        "openbao/control-plane",
				Provider:  capability.OpenBao,
				Scope:     capability.ScopeShared,
				Ownership: capability.OwnershipBaseHarbor,
			}
		case capability.ProviderCaddy:
			instance = capability.ProviderInstance{
				ID:               fmt.Sprintf("caddy/%s/%s", m.Name, m.Environment),
				Provider:         capability.Caddy,
				Scope:            capability.ScopeApplication,
				Ownership:        capability.OwnershipBaseHarbor,
				OwnerApplication: m.Name,
			}
		default:
			return fmt.Errorf("no reference provider registry mapping for %q", resource.Provider)
		}
		if err := registry.Register(instance); err != nil {
			return err
		}
		if err := registry.Bind(resource, instance.ID); err != nil {
			return err
		}
	}
	if metricsPolicy.Enabled && HasRuntimeMetricsPermissions(m) {
		instance, err := prometheusProviderInstance(m)
		if err != nil {
			return err
		}
		if err := registry.Register(instance); err != nil {
			return err
		}
		resource := capability.Resource{
			Application: m.Name,
			Kind:        capability.Metrics,
			Name:        runtimeMetricsRegistryResource,
			Provider:    capability.ProviderPrometheus,
		}
		if err := registry.Bind(resource, instance.ID); err != nil {
			return err
		}
	}
	return nil
}

func prometheusProviderInstance(m Manifest) (capability.ProviderInstance, error) {
	placement, err := ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return capability.ProviderInstance{}, err
	}
	switch placement.Scope {
	case capability.ScopeShared:
		id := "prometheus/shared"
		if placement.SharingBoundary != "" {
			id = fmt.Sprintf("prometheus/shared/%s", ProviderPlacementNameToken(placement.SharingBoundary))
		}
		return capability.ProviderInstance{
			ID:              id,
			Provider:        capability.Prometheus,
			Scope:           capability.ScopeShared,
			SharingBoundary: placement.SharingBoundary,
			Ownership:       capability.OwnershipBaseHarbor,
		}, nil
	case capability.ScopeApplication:
		return capability.ProviderInstance{
			ID:               fmt.Sprintf("prometheus/%s/%s", m.Name, m.Environment),
			Provider:         capability.Prometheus,
			Scope:            capability.ScopeApplication,
			Ownership:        capability.OwnershipBaseHarbor,
			OwnerApplication: m.Name,
		}, nil
	case capability.ScopeExternal:
		return capability.ProviderInstance{
			ID:        fmt.Sprintf("prometheus-external/%s/%s", m.Name, m.Environment),
			Provider:  capability.Prometheus,
			Scope:     capability.ScopeExternal,
			Ownership: capability.OwnershipExternal,
			Reference: placement.ExternalReference,
		}, nil
	default:
		return capability.ProviderInstance{}, fmt.Errorf("unsupported provider scope %q", placement.Scope)
	}
}

