package application

import (
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const providerRegistryFile = "provider-registry.json"

func ReconcileReferenceProviderRegistry(m Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	store := capability.RegistryStore{Path: filepath.Join(dataDir, providerRegistryFile)}
	return store.Update(func(registry *capability.Registry) error {
		registry.ReleaseManagedApplication(m.Name)
		return registerReferenceProviders(registry, m)
	})
}

func ReleaseApplicationProviderRegistry(m Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	store := capability.RegistryStore{Path: filepath.Join(dataDir, providerRegistryFile)}
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
		case capability.ProviderOpenBao:
			instance = capability.ProviderInstance{
				ID:        "openbao/control-plane",
				Provider:  capability.OpenBao,
				Scope:     capability.ScopeShared,
				Ownership: capability.OwnershipBaseHarbor,
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
