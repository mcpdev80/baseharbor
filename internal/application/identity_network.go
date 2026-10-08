package application

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func IdentityProviderNetworkName(m Manifest, namespace string) (string, error) {
	if !m.Services.Identity {
		return "", nil
	}
	provider, err := referenceCapabilityProvider(capability.Identity)
	if err != nil {
		return "", err
	}
	if provider.Kind == capability.ProviderExternalOIDC {
		return "", nil
	}
	placement, err := ResolveProviderPlacement(m, capability.ProviderKeycloak)
	if err != nil {
		return "", err
	}
	return IdentityProviderNetworkNameForPlacement(m, namespace, placement)
}

func IdentityProviderNetworkNameForPlacement(m Manifest, namespace string, placement capability.ProviderPlacement) (string, error) {
	base := "baseharbor-identity"
	switch placement.Scope {
	case capability.ScopeShared:
		if boundary := strings.TrimSpace(placement.SharingBoundary); boundary != "" {
			base += "-" + boundary
		}
	case capability.ScopeApplication:
		base += "-" + m.Name + "-" + m.Environment
	default:
		return "", fmt.Errorf("managed identity provider requires shared or application placement")
	}
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	if namespace != "" {
		base += "-" + namespace
	}
	return base, nil
}
