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
	rules, err := LoadConnectivityRules()
	if err != nil {
		return err
	}
	if len(rules) != 0 {
		return fmt.Errorf("connectivity policy still contains %d cross-application rule(s); disconnect them before the global control plane", len(rules))
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

	var metricsPolicy MetricsDeploymentPolicy
	metricsPolicyResolved := false
	resolveMetricsPolicy := func() (MetricsDeploymentPolicy, error) {
		if metricsPolicyResolved {
			return metricsPolicy, nil
		}
		metricsPolicy, err = MetricsPolicy(m)
		if err != nil {
			return MetricsDeploymentPolicy{}, err
		}
		metricsPolicyResolved = true
		return metricsPolicy, nil
	}

	for _, resource := range resources {
		if resource.Kind == capability.Metrics {
			policy, err := resolveMetricsPolicy()
			if err != nil {
				return err
			}
			if !policy.Enabled || !policy.Collect[MetricsSourceApplication] {
				continue
			}
		}

		instance, err := referenceProviderInstance(m, resource)
		if err != nil {
			return err
		}
		if err := registry.Register(instance); err != nil {
			return err
		}
		if err := registry.Bind(resource, instance.ID); err != nil {
			return err
		}
	}

	if HasRuntimeMetricsPermissions(m) {
		policy, err := resolveMetricsPolicy()
		if err != nil {
			return err
		}
		if policy.Enabled && policy.Collect[MetricsSourceApplication] {
			resource := capability.Resource{
				Application: m.Name,
				Kind:        capability.Metrics,
				Name:        runtimeMetricsRegistryResource,
				Provider:    capability.ProviderPrometheus,
			}
			instance, err := referenceProviderInstance(m, resource)
			if err != nil {
				return err
			}
			if err := registry.Register(instance); err != nil {
				return err
			}
			if err := registry.Bind(resource, instance.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func referenceProviderInstance(m Manifest, resource capability.Resource) (capability.ProviderInstance, error) {
	placement, err := ResolveProviderPlacement(m, resource.Provider)
	if err != nil {
		return capability.ProviderInstance{}, err
	}

	instance := capability.ProviderInstance{
		Provider:         providerDescriptor(resource.Provider),
		Scope:            placement.Scope,
		SharingBoundary:  placement.SharingBoundary,
		Ownership:        placement.Ownership,
		Reference:        placement.ExternalReference,
	}
	if instance.Provider.Kind == "" {
		return capability.ProviderInstance{}, fmt.Errorf("no reference provider registry mapping for %q", resource.Provider)
	}

	switch placement.Scope {
	case capability.ScopeShared:
		instance.ID = sharedProviderInstanceID(resource.Provider, placement.SharingBoundary)
	case capability.ScopeApplication:
		instance.ID = applicationProviderInstanceID(resource.Provider, m, resource.Name)
		instance.OwnerApplication = m.Name
	case capability.ScopeExternal:
		instance.ID = externalProviderInstanceID(resource.Provider, m, resource.Name, placement.ExternalReference)
	default:
		return capability.ProviderInstance{}, fmt.Errorf("unsupported provider scope %q", placement.Scope)
	}
	return instance, nil
}

func providerDescriptor(provider capability.ProviderKind) capability.Provider {
	switch provider {
	case capability.ProviderPostgreSQL:
		return capability.PostgreSQL
	case capability.ProviderValkey:
		return capability.Valkey
	case capability.ProviderOpenBao:
		return capability.OpenBao
	case capability.ProviderCaddy:
		return capability.Caddy
	case capability.ProviderSeaweedFS:
		return capability.SeaweedFS
	case capability.ProviderOTelCollector:
		return capability.OTelCollector
	case capability.ProviderExternalOTLP:
		return capability.ExternalOTLP
	case capability.ProviderPrometheus:
		return capability.Prometheus
	default:
		return capability.Provider{}
	}
}

func sharedProviderInstanceID(provider capability.ProviderKind, boundary string) string {
	if boundary == "" {
		switch provider {
		case capability.ProviderOpenBao:
			return "openbao/control-plane"
		default:
			return string(provider) + "/shared"
		}
	}
	return fmt.Sprintf("%s/shared/%s", provider, ProviderPlacementNameToken(boundary))
}

func applicationProviderInstanceID(provider capability.ProviderKind, m Manifest, resourceName string) string {
	switch provider {
	case capability.ProviderPostgreSQL, capability.ProviderValkey:
		return fmt.Sprintf("%s/%s/%s/%s", provider, m.Name, m.Environment, resourceName)
	default:
		return fmt.Sprintf("%s/%s/%s", provider, m.Name, m.Environment)
	}
}

func externalProviderInstanceID(provider capability.ProviderKind, m Manifest, resourceName, reference string) string {
	if provider == capability.ProviderExternalOTLP && reference == "default" {
		return "external-otlp/default"
	}
	return fmt.Sprintf("%s/external/%s/%s/%s", provider, m.Name, m.Environment, ProviderPlacementNameToken(resourceName+"-"+reference))
}
