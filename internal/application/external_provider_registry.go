package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const externalProviderRegistryFile = "external-providers.json"

func externalProviderStore() (externalprovider.Store, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return externalprovider.Store{}, err
	}
	return externalProviderStoreAt(dataDir)
}

func externalProviderStoreAt(dataDir string) (externalprovider.Store, error) {
	dataDir = filepath.Clean(dataDir)
	if dataDir == "." || dataDir == "" {
		return externalprovider.Store{}, fmt.Errorf("external provider data root is required")
	}
	return externalprovider.Store{Path: filepath.Join(dataDir, externalProviderRegistryFile)}, nil
}

func RegisterExternalProvider(reg externalprovider.Registration) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return RegisterExternalProviderAt(dataDir, reg)
}

func RegisterExternalProviderAt(dataDir string, reg externalprovider.Registration) error {
	if err := reg.Validate(); err != nil {
		return err
	}
	store, err := externalProviderStoreAt(dataDir)
	if err != nil {
		return err
	}
	registryStore, err := referenceProviderRegistryStoreAt(dataDir)
	if err != nil {
		return err
	}
	instance := capability.ProviderInstance{
		ID:               externalProviderInstanceID(reg.Provider.Kind, reg.ID),
		ProviderID:       reg.ProviderID,
		ProviderVersion:  reg.ProviderVersion,
		ProviderProtocol: reg.ProviderProtocol,
		Provider:         reg.Provider,
		Scope:            capability.ScopeExternal,
		Ownership:        capability.OwnershipExternal,
		Reference:        reg.ID,
	}
	_, inspectErr := store.Inspect(reg.ID)
	existed := inspectErr == nil
	if inspectErr != nil && !strings.Contains(inspectErr.Error(), "not found") {
		return inspectErr
	}
	if err := store.Register(reg); err != nil {
		return err
	}
	if err := registryStore.Update(func(registry *capability.Registry) error {
		return registry.Register(instance)
	}); err != nil {
		if !existed {
			_ = store.Remove(reg.ID)
		}
		return err
	}
	return nil
}

func ListExternalProviders() ([]externalprovider.Registration, error) {
	store, err := externalProviderStore()
	if err != nil {
		return nil, err
	}
	return store.List()
}

func InspectExternalProvider(id string) (externalprovider.Registration, error) {
	store, err := externalProviderStore()
	if err != nil {
		return externalprovider.Registration{}, err
	}
	return store.Inspect(id)
}

func RemoveExternalProvider(id string) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return RemoveExternalProviderAt(dataDir, id)
}

func RemoveExternalProviderAt(dataDir, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("external provider id is required")
	}
	store, err := externalProviderStoreAt(dataDir)
	if err != nil {
		return err
	}
	reg, err := store.Inspect(id)
	if err != nil {
		return err
	}
	registryStore, err := referenceProviderRegistryStoreAt(dataDir)
	if err != nil {
		return err
	}
	instanceID := externalProviderInstanceID(reg.Provider.Kind, id)
	if err := registryStore.Update(func(registry *capability.Registry) error {
		return registry.UnregisterExternal(instanceID)
	}); err != nil {
		return err
	}
	if err := store.Remove(id); err != nil {
		return err
	}
	return nil
}

func VerifyExternalProvider(ctx context.Context, id string) (externalprovider.Verification, error) {
	store, err := externalProviderStore()
	if err != nil {
		return externalprovider.Verification{}, err
	}
	reg, err := store.Inspect(id)
	if err != nil {
		return externalprovider.Verification{}, err
	}
	return externalprovider.Verify(ctx, reg)
}
