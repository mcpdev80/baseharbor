package application

import (
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const providerRegistryFile = "provider-registry.json"

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
	for _, resource := range resources {
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
	return nil
}
