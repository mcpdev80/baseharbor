package application

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
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
	case capability.ProviderPostgreSQL, capability.ProviderValkey:
		return capability.ProviderPlacement{
			Scope:     capability.ScopeShared,
			Ownership: capability.OwnershipBaseHarbor,
		}, nil
	case capability.ProviderCaddy:
		return capability.ProviderPlacement{
			Scope:     capability.ScopeApplication,
			Ownership: capability.OwnershipBaseHarbor,
		}, nil
	case capability.ProviderRabbitMQ:
		return capability.ProviderPlacement{
			Scope:     capability.ScopeApplication,
			Ownership: capability.OwnershipBaseHarbor,
		}, nil
	case capability.ProviderMongoDB:
		return capability.ProviderPlacement{
			Scope:     capability.ScopeApplication,
			Ownership: capability.OwnershipBaseHarbor,
		}, nil
	case capability.ProviderOpenBao, capability.ProviderSeaweedFS, capability.ProviderOTelCollector, capability.ProviderPrometheus, capability.ProviderLoki, capability.ProviderTempo, capability.ProviderKeycloak:
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
	case capability.ProviderExternalOIDC:
		issuer := strings.TrimSpace(os.Getenv("BASEHARBOR_EXTERNAL_OIDC_ISSUER"))
		if issuer == "" {
			return capability.ProviderPlacement{}, fmt.Errorf("BASEHARBOR_EXTERNAL_OIDC_ISSUER is required when external OIDC is selected")
		}
		return capability.ProviderPlacement{
			Scope:             capability.ScopeExternal,
			Ownership:         capability.OwnershipExternal,
			ExternalReference: issuer,
		}, nil
	default:
		return capability.ProviderPlacement{}, fmt.Errorf("no default provider placement for %q", provider)
	}
}

// ResolveProviderPlacement resolves deployment/operator placement for a provider.
// Portable application intent never participates in this decision. Defaults are
// deliberately simple; advanced users can override supported placement fields
// through the generic provider policy namespace.
func ResolveProviderPlacement(m Manifest, provider capability.ProviderKind) (capability.ProviderPlacement, error) {
	placement, err := DefaultProviderPlacement(provider)
	if err != nil {
		return capability.ProviderPlacement{}, err
	}
	if organizationPlacement, ok, err := resolveOrganizationProviderPlacement(m, provider); err != nil {
		return capability.ProviderPlacement{}, err
	} else if ok {
		placement = organizationPlacement
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
	if placement.Scope == capability.ScopeShared &&
		placement.SharingBoundary == "" &&
		(provider == capability.ProviderPostgreSQL || provider == capability.ProviderValkey) {
		environment := strings.ToLower(strings.TrimSpace(m.Environment))
		if environment == "" {
			environment = "dev"
		}
		placement.SharingBoundary = "environment:" + environment
	}
	if placement.SharingBoundary != "" &&
		provider != capability.ProviderPrometheus &&
		provider != capability.ProviderLoki &&
		provider != capability.ProviderKeycloak &&
		provider != capability.ProviderPostgreSQL &&
		provider != capability.ProviderValkey {
		return capability.ProviderPlacement{}, fmt.Errorf("%s is not supported by the current %s adapter", ProviderSharingBoundaryEnv(provider), provider)
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

func resolveOrganizationProviderPlacement(m Manifest, provider capability.ProviderKind) (capability.ProviderPlacement, bool, error) {
	state, ok, err := orgconfig.LoadActiveOptional()
	if err != nil || !ok {
		return capability.ProviderPlacement{}, false, err
	}
	effective, err := orgconfig.ResolveEffective(state, m.Environment)
	if err != nil {
		return capability.ProviderPlacement{}, false, fmt.Errorf("resolve organization provider defaults: %w", err)
	}
	var keys []string
	switch provider {
	case capability.ProviderPostgreSQL:
		keys = []string{string(capability.SQL)}
	case capability.ProviderValkey:
		keys = []string{string(capability.DurableKeyValue), string(capability.KeyValue)}
	case capability.ProviderRabbitMQ:
		keys = []string{string(capability.MessagingQueue), string(capability.MessagingPubSub), string(capability.MessagingStream)}
	case capability.ProviderMongoDB:
		keys = []string{string(capability.DocumentDatabase)}
	default:
		return capability.ProviderPlacement{}, false, nil
	}
	var selected orgconfig.EffectiveProvider
	found := false
	for _, key := range keys {
		if candidate, exists := effective.Providers[key]; exists {
			selected, found = candidate, true
			break
		}
	}
	if !found {
		return capability.ProviderPlacement{}, false, nil
	}
	scope := capability.ProviderScope(strings.TrimSpace(selected.Scope))
	if scope == "" {
		scope = capability.ScopeExternal
	}
	placement := capability.ProviderPlacement{Scope: scope}
	switch scope {
	case capability.ScopeExternal:
		const prefix = "external-provider:"
		reference := strings.TrimSpace(selected.Reference)
		if !strings.HasPrefix(reference, prefix) || strings.TrimSpace(strings.TrimPrefix(reference, prefix)) == "" {
			return capability.ProviderPlacement{}, false, fmt.Errorf("organization provider default %q must reference %sexternal-id for external placement", selected.Provider, prefix)
		}
		placement.Ownership = capability.OwnershipExternal
		placement.ExternalReference = strings.TrimSpace(strings.TrimPrefix(reference, prefix))
	case capability.ScopeShared, capability.ScopeApplication:
		placement.Ownership = capability.OwnershipBaseHarbor
	default:
		return capability.ProviderPlacement{}, false, fmt.Errorf("organization provider default %q has unsupported scope %q", selected.Provider, scope)
	}
	return placement, true, nil
}
