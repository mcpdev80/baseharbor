package application

import (
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func providerPlacementEnvToken(provider capability.ProviderKind) string {
	token := strings.ToUpper(string(provider))
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_")
	return replacer.Replace(token)
}

func ProviderScopeEnv(provider capability.ProviderKind) string {
	return "BASEHARBOR_PROVIDER_" + providerPlacementEnvToken(provider) + "_SCOPE"
}

func ProviderSharingBoundaryEnv(provider capability.ProviderKind) string {
	return "BASEHARBOR_PROVIDER_" + providerPlacementEnvToken(provider) + "_SHARING_BOUNDARY"
}

func ProviderExternalReferenceEnv(provider capability.ProviderKind) string {
	return "BASEHARBOR_PROVIDER_" + providerPlacementEnvToken(provider) + "_EXTERNAL_REFERENCE"
}

func DefaultProviderPlacement(provider capability.ProviderKind) (capability.ProviderPlacement, error) {
	switch provider {
	case capability.ProviderPostgreSQL, capability.ProviderValkey, capability.ProviderCaddy:
		return capability.ProviderPlacement{
			Scope:     capability.ScopeApplication,
			Ownership: capability.OwnershipBaseHarbor,
		}, nil
	case capability.ProviderOpenBao, capability.ProviderSeaweedFS, capability.ProviderOTelCollector, capability.ProviderPrometheus:
		return capability.ProviderPlacement{
			Scope:     capability.ScopeShared,
			Ownership: capability.OwnershipBaseHarbor,
		}, nil
	case capability.ProviderExternalOTLP:
		return capability.ProviderPlacement{
			Scope:             capability.ScopeExternal,
			Ownership:         capability.OwnershipExternal,
			ExternalReference: "default",
		}, nil
	default:
		return capability.ProviderPlacement{}, fmt.Errorf("no default provider placement for %q", provider)
	}
}

// ResolveProviderPlacement resolves deployment/operator placement for a provider.
// Portable application intent never participates in this decision. Defaults are
// deliberately simple; advanced users can override supported placement fields
// through the generic provider policy namespace.
func ResolveProviderPlacement(_ Manifest, provider capability.ProviderKind) (capability.ProviderPlacement, error) {
	placement, err := DefaultProviderPlacement(provider)
	if err != nil {
		return capability.ProviderPlacement{}, err
	}

	if raw := strings.TrimSpace(os.Getenv(ProviderScopeEnv(provider))); raw != "" {
		placement.Scope = capability.ProviderScope(strings.ToLower(raw))
		switch placement.Scope {
		case capability.ScopeShared, capability.ScopeApplication:
			placement.Ownership = capability.OwnershipBaseHarbor
			placement.ExternalReference = ""
		case capability.ScopeExternal:
			placement.Ownership = capability.OwnershipExternal
		default:
			return capability.ProviderPlacement{}, fmt.Errorf("%s must be shared, application, or external", ProviderScopeEnv(provider))
		}
	}

	if raw := strings.TrimSpace(os.Getenv(ProviderSharingBoundaryEnv(provider))); raw != "" {
		placement.SharingBoundary = raw
	}
	if raw := strings.TrimSpace(os.Getenv(ProviderExternalReferenceEnv(provider))); raw != "" {
		placement.ExternalReference = raw
	}

	descriptor, err := capability.ReferenceIntegration(provider)
	if err != nil {
		return capability.ProviderPlacement{}, err
	}
	if err := capability.ValidateProviderPlacement(descriptor, placement); err != nil {
		return capability.ProviderPlacement{}, fmt.Errorf("resolve provider placement for %s: %w", provider, err)
	}
	return placement, nil
}
