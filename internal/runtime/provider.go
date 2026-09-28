package runtime

import (
	"context"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

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

type DockerProvider struct{ Compose }
type PodmanProvider struct{ Compose }

func (DockerProvider) Kind() ProviderKind { return ProviderDocker }
func (PodmanProvider) Kind() ProviderKind { return ProviderPodman }

var referenceProviderCapabilities = ProviderCapabilities{
	WorkloadLifecycle: true,
	ServiceExec:       true,
	PublishedPorts:    true,
	ResourceOwnership: true,
}

var dockerProviderDescriptor = ProviderDescriptor{
	Kind:            ProviderDocker,
	ContractVersion: RuntimeProviderContractVersion,
	ProviderVersion: "0.4.17",
	Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
	WorkloadSources: []string{"compose-spec"},
	Realization:     "docker-compose",
	Capabilities:    referenceProviderCapabilities,
}

var podmanProviderDescriptor = ProviderDescriptor{
	Kind:            ProviderPodman,
	ContractVersion: RuntimeProviderContractVersion,
	ProviderVersion: "0.4.17",
	Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
	WorkloadSources: []string{"compose-spec"},
	Realization:     "podman-quadlet-systemd-user",
	Capabilities:    referenceProviderCapabilities,
}

func NewDockerProvider(ctx context.Context) (RuntimeProvider, error) {
	compose, err := detectDockerCompose(ctx)
	if err != nil {
		return nil, err
	}
	return DockerProvider{Compose: compose}, nil
}

func NewPodmanProvider(ctx context.Context) (RuntimeProvider, error) {
	runtime, err := detectPodmanRuntime(ctx)
	if err != nil {
		return nil, err
	}
	return PodmanProvider{Compose: runtime}, nil
}

func DockerProviderDescriptor() ProviderDescriptor {
	return cloneProviderDescriptor(dockerProviderDescriptor)
}

func PodmanProviderDescriptor() ProviderDescriptor {
	return cloneProviderDescriptor(podmanProviderDescriptor)
}

func cloneProviderDescriptor(descriptor ProviderDescriptor) ProviderDescriptor {
	descriptor.Standards = append([]string(nil), descriptor.Standards...)
	descriptor.WorkloadSources = append([]string(nil), descriptor.WorkloadSources...)
	return descriptor
}

func (DockerProvider) Descriptor() ProviderDescriptor { return DockerProviderDescriptor() }
func (PodmanProvider) Descriptor() ProviderDescriptor { return PodmanProviderDescriptor() }

func (DockerProvider) PreferredLocalHTTPSPort() int { return 443 }
func (PodmanProvider) PreferredLocalHTTPSPort() int { return 8443 }
func (Compose) PreferredLocalHTTPSPort() int        { return 443 }

func (DockerProvider) Capabilities() ProviderCapabilities { return referenceProviderCapabilities }
func (PodmanProvider) Capabilities() ProviderCapabilities { return referenceProviderCapabilities }
