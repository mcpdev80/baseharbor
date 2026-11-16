package main

import (
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// An external Identity binding never requests a local Keycloak or local login
// authority. Reuse the existing deployment provider/placement decisions.
func hasManagedDevelopmentIdentity(m application.Manifest) bool {
	if !application.HasIdentity(m) {
		return false
	}
	provider, err := application.IdentityProviderForDeployment()
	return err == nil && provider.Kind == capability.ProviderKeycloak
}

func requiresManagedDevelopmentOperatorRealm(m application.Manifest) bool {
	if !hasManagedDevelopmentIdentity(m) {
		return false
	}
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderKeycloak)
	return err == nil && placement.Ownership == capability.OwnershipBaseHarbor && placement.Scope == capability.ScopeShared
}
