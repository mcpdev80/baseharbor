package runtime

import runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"

type ProviderKind = runtimecontract.ProviderKind

const (
	ProviderDocker     = runtimecontract.ProviderDocker
	ProviderPodman     = runtimecontract.ProviderPodman
	ProviderKubernetes = runtimecontract.ProviderKubernetes
	ProviderOpenShift  = runtimecontract.ProviderOpenShift
)

type RuntimeCapability = runtimecontract.RuntimeCapability

const RuntimeProviderContractVersion = runtimecontract.RuntimeProviderContractVersion

const (
	CapabilityWorkloadLifecycle = runtimecontract.CapabilityWorkloadLifecycle
	CapabilityServiceExec       = runtimecontract.CapabilityServiceExec
	CapabilityPublishedPorts    = runtimecontract.CapabilityPublishedPorts
	CapabilityResourceOwnership = runtimecontract.CapabilityResourceOwnership
)

type ProviderCapabilities = runtimecontract.ProviderCapabilities
type ProviderDescriptor = runtimecontract.ProviderDescriptor
type Provider = runtimecontract.Provider
type ProviderFactory = runtimecontract.ProviderFactory
type ProviderRegistration = runtimecontract.ProviderRegistration
type ProviderRegistry = runtimecontract.ProviderRegistry

func ParseProviderKind(value string) (ProviderKind, error) {
	return runtimecontract.ParseProviderKind(value)
}

func NewProviderRegistry(registrations ...ProviderRegistration) (*ProviderRegistry, error) {
	return runtimecontract.NewProviderRegistry(registrations...)
}

func RequireCapabilities(provider Provider, required ...RuntimeCapability) error {
	return runtimecontract.RequireCapabilities(provider, required...)
}
