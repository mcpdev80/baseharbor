package application

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

// A Target projection may consume an explicitly registered global external
// offer. It never copies registration/material stores or mutates foreign state.
func projectExternalProviderOfferAt(dataDir string, registry *capability.Registry, provider capability.ProviderKind, placement capability.ProviderPlacement, applicationID string) error {
	if placement.Scope != capability.ScopeExternal {
		return nil
	}
	for _, instance := range registry.Instances {
		if instance.Provider.Kind == provider && instance.Scope == capability.ScopeExternal && instance.Reference == placement.ExternalReference {
			_, err := registry.ResolvePlacement(provider, placement, applicationID)
			return err
		}
	}
	root, ok := canonicalGlobalProviderRoot(dataDir)
	if !ok {
		return nil // Arbitrary paths cannot borrow another installation's owner.
	}
	store, err := referenceProviderRegistryStoreAt(root)
	if err != nil {
		return err
	}
	global, err := store.Load()
	if err != nil {
		return err
	}
	instance, err := global.ResolvePlacement(provider, placement, applicationID)
	if err != nil {
		return err
	}
	registrationStore, err := externalProviderStoreAt(root)
	if err != nil {
		return err
	}
	registration, err := registrationStore.Inspect(instance.Reference)
	if err != nil {
		return err
	}
	if !externalRegistrationMatchesInstance(registration, instance) {
		return errors.New("external registration and registered offer disagree")
	}
	return registry.Register(instance)
}

// Resolve through the existing placement and registered offer, then read its
// actual endpoint/material references. A reference ID is never an issuer URL.
func ResolveExternalProviderRegistrationAt(dataDir string, m Manifest, provider capability.ProviderKind) (externalprovider.Registration, error) {
	placement, err := ResolveProviderPlacement(m, provider)
	if err != nil {
		return externalprovider.Registration{}, err
	}
	if placement.Scope != capability.ScopeExternal {
		return externalprovider.Registration{}, errors.New("selected provider is not external")
	}
	store, err := referenceProviderRegistryStoreAt(dataDir)
	if err != nil {
		return externalprovider.Registration{}, err
	}
	registry, err := store.Load()
	if err != nil {
		return externalprovider.Registration{}, err
	}
	if err := projectExternalProviderOfferAt(dataDir, &registry, provider, placement, m.ApplicationID); err != nil {
		return externalprovider.Registration{}, err
	}
	instance, err := registry.ResolvePlacement(provider, placement, m.ApplicationID)
	if err != nil {
		return externalprovider.Registration{}, err
	}
	registrationRoot := dataDir
	_, ownerErr := os.Lstat(filepath.Join(dataDir, externalProviderRegistryFile))
	if errors.Is(ownerErr, os.ErrNotExist) {
		if root, ok := canonicalGlobalProviderRoot(dataDir); ok {
			registrationRoot = root
		}
	} else if ownerErr != nil {
		return externalprovider.Registration{}, ownerErr
	}
	registrationStore, err := externalProviderStoreAt(registrationRoot)
	if err != nil {
		return externalprovider.Registration{}, err
	}
	registration, err := registrationStore.Inspect(instance.Reference)
	if err != nil {
		return externalprovider.Registration{}, err
	}
	if !externalRegistrationMatchesInstance(registration, instance) {
		return externalprovider.Registration{}, errors.New("external registration and selected offer disagree")
	}
	return registration, nil
}

func externalRegistrationMatchesInstance(r externalprovider.Registration, i capability.ProviderInstance) bool {
	return i.Ownership == capability.OwnershipExternal && i.Scope == capability.ScopeExternal && r.ID == i.Reference && r.ProviderID == i.ProviderID && r.ProviderVersion == i.ProviderVersion && r.ProviderProtocol == i.ProviderProtocol && reflect.DeepEqual(r.Provider, i.Provider)
}

func canonicalGlobalProviderRoot(dataDir string) (string, bool) {
	root, err := deployment.DataRoot()
	if err != nil {
		return "", false
	}
	target := filepath.Base(filepath.Clean(dataDir))
	expected, err := deployment.TargetStateRoot(target)
	if err != nil {
		return "", false
	}
	actualAbs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", false
	}
	expectedAbs, err := filepath.Abs(expected)
	if err != nil || actualAbs != expectedAbs {
		return "", false
	}
	for dir := actualAbs; ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) || err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return "", false
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return root, true
}
