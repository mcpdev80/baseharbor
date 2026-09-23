package application

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/provider/builtin"
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
	case capability.ProviderOpenBao, capability.ProviderSeaweedFS, capability.ProviderOTelCollector, capability.ProviderPrometheus, capability.ProviderLoki, capability.ProviderTempo:
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
	if placement.SharingBoundary != "" && provider != capability.ProviderPrometheus && provider != capability.ProviderLoki {
		return capability.ProviderPlacement{}, fmt.Errorf("%s is not supported by the current %s adapter; named shared boundaries are implemented for Prometheus and Loki only", ProviderSharingBoundaryEnv(provider), provider)
	}
	if raw := strings.TrimSpace(os.Getenv(ProviderExternalReferenceEnv(provider))); raw != "" {
		placement.ExternalReference = raw
	}

	bundled, err := builtin.Lookup(provider)
	if err != nil {
		return capability.ProviderPlacement{}, err
	}
	if err := capability.ValidateProviderPlacement(bundled.Integration, placement); err != nil {
		return capability.ProviderPlacement{}, fmt.Errorf("resolve provider placement for %s: %w", provider, err)
	}
	return placement, nil
}

func ProviderPlacementNameToken(value string) string {
	value = strings.TrimSpace(value)
	sum := sha256.Sum256([]byte(value))
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	base := strings.Trim(b.String(), "-_.")
	if base == "" {
		base = "boundary"
	}
	if len(base) > 32 {
		base = base[:32]
	}
	return fmt.Sprintf("%s-%x", base, sum[:4])
}
