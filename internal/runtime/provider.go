package runtime

import (
	"context"
	"fmt"

	kubernetesprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/kubernetes"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

// Compatibility aliases keep the current v0.4 call sites stable while the
// provider-neutral contract moves out of the legacy Compose implementation.
type ProviderKind = runtimecontract.ProviderKind
type RuntimeCapability = runtimecontract.RuntimeCapability
type ProviderCapabilities = runtimecontract.ProviderCapabilities
type Provider = runtimecontract.Provider

const (
	ProviderCompose    = runtimecontract.ProviderCompose
	ProviderKubernetes = runtimecontract.ProviderKubernetes

	CapabilityWorkloadLifecycle = runtimecontract.CapabilityWorkloadLifecycle
	CapabilityServiceExec       = runtimecontract.CapabilityServiceExec
	CapabilityPublishedPorts    = runtimecontract.CapabilityPublishedPorts
	CapabilityResourceOwnership = runtimecontract.CapabilityResourceOwnership
)

func ParseProviderKind(value string) (ProviderKind, error) {
	return runtimecontract.ParseProviderKind(value)
}

func RequireCapabilities(provider Provider, required ...RuntimeCapability) error {
	return runtimecontract.RequireCapabilities(provider, required...)
}

func (Compose) Kind() ProviderKind {
	return ProviderCompose
}

func (Compose) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		WorkloadLifecycle: true,
		ServiceExec:       true,
		PublishedPorts:    true,
		ResourceOwnership: true,
	}
}

// DetectProviderForKind resolves the explicitly selected deployment runtime.
// Provider selection belongs to deployment/environment state and must never be
// inferred from the portable application contract.
func DetectProviderForKind(ctx context.Context, kind ProviderKind) (Provider, error) {
	normalized, err := ParseProviderKind(string(kind))
	if err != nil {
		return nil, err
	}
	switch normalized {
	case ProviderCompose:
		return detectCompose(ctx)
	case ProviderKubernetes:
		return kubernetesprovider.Detect(ctx)
	default:
		return nil, fmt.Errorf("runtime provider %q is not implemented", normalized)
	}
}

// DetectProvider preserves the v0.3/v0.4 Compose-default compatibility path.
// Deployment-aware callers should use DetectProviderForKind with the explicit
// provider stored in protected deployment state.
func DetectProvider(ctx context.Context) (Provider, error) {
	return DetectProviderForKind(ctx, ProviderCompose)
}
