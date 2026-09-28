package runtime

import (
	"context"
	"fmt"
	"strings"
)

// ProviderKind identifies the runtime implementation selected for application
// workloads. It is deployment configuration, not part of the portable
// application contract.
type ProviderKind string

const (
	ProviderDocker     ProviderKind = "docker"
	ProviderPodman     ProviderKind = "podman"
	ProviderKubernetes ProviderKind = "kubernetes"
	ProviderOpenShift  ProviderKind = "openshift"
)

// RuntimeCapability names portable runtime behavior that orchestration may
// require from a provider. These capabilities describe the runtime substrate,
// not application capabilities such as PostgreSQL or object storage.
type RuntimeCapability string

const RuntimeProviderContractVersion = "baseharbor.runtime/v1"

const (
	CapabilityWorkloadLifecycle RuntimeCapability = "workload-lifecycle"
	CapabilityServiceExec       RuntimeCapability = "service-exec"
	CapabilityPublishedPorts    RuntimeCapability = "published-ports"
	CapabilityResourceOwnership RuntimeCapability = "resource-ownership"
)

// ProviderCapabilities describes runtime behavior that orchestration may rely
// on. Capability providers such as PostgreSQL, Valkey or object storage are a
// separate axis and must not be encoded here.
type ProviderCapabilities struct {
	WorkloadLifecycle bool
	ServiceExec       bool
	PublishedPorts    bool
	ResourceOwnership bool
}

// ProviderDescriptor is the versioned, product-neutral declaration used for
// runtime discovery and capability negotiation. Product implementation details
// remain behind the provider factory.
type ProviderDescriptor struct {
	Kind            ProviderKind
	ContractVersion string
	ProviderVersion string
	Standards       []string
	Capabilities    ProviderCapabilities
}

func (c ProviderCapabilities) Supports(capability RuntimeCapability) bool {
	switch capability {
	case CapabilityWorkloadLifecycle:
		return c.WorkloadLifecycle
	case CapabilityServiceExec:
		return c.ServiceExec
	case CapabilityPublishedPorts:
		return c.PublishedPorts
	case CapabilityResourceOwnership:
		return c.ResourceOwnership
	default:
		return false
	}
}

// Provider is the minimal runtime-provider seam. It deliberately exposes only
// provider identity and runtime capabilities at this stage; provider-specific
// lifecycle inputs stay behind the concrete implementation until a portable
// operation has a demonstrated second implementation.
type Provider interface {
	Kind() ProviderKind
	Descriptor() ProviderDescriptor
	Capabilities() ProviderCapabilities
}

type providerFactory func(context.Context) (Provider, error)

type providerRegistration struct {
	descriptor ProviderDescriptor
	factory    providerFactory
}

func ParseProviderKind(value string) (ProviderKind, error) {
	kind := ProviderKind(strings.TrimSpace(strings.ToLower(value)))
	if kind == "" {
		kind = ProviderDocker
	}
	switch kind {
	case ProviderDocker, ProviderPodman, ProviderKubernetes, ProviderOpenShift:
		return kind, nil
	default:
		return "", fmt.Errorf("unsupported runtime provider %q", value)
	}
}

// RequireCapabilities validates runtime requirements before orchestration
// starts. Providers must satisfy every requested capability; BaseHarbor never
// silently downgrades runtime behavior because a provider lacks a feature.
func RequireCapabilities(provider Provider, required ...RuntimeCapability) error {
	if provider == nil {
		return fmt.Errorf("runtime provider is required")
	}
	descriptor := provider.Descriptor()
	if descriptor.ContractVersion != RuntimeProviderContractVersion {
		return fmt.Errorf("runtime provider %s uses contract %q, require %q", provider.Kind(), descriptor.ContractVersion, RuntimeProviderContractVersion)
	}
	if descriptor.Kind != provider.Kind() {
		return fmt.Errorf("runtime provider descriptor kind %q does not match provider %q", descriptor.Kind, provider.Kind())
	}
	caps := descriptor.Capabilities
	for _, capability := range required {
		if capability == "" {
			return fmt.Errorf("runtime provider %s: empty capability requirement", provider.Kind())
		}
		if !caps.Supports(capability) {
			return fmt.Errorf("runtime provider %s does not support required capability %q", provider.Kind(), capability)
		}
	}
	return nil
}

func (Compose) Kind() ProviderKind { return ProviderDocker }

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

var runtimeProviderRegistry = map[ProviderKind]providerRegistration{
	ProviderDocker: {
		descriptor: ProviderDescriptor{
			Kind:            ProviderDocker,
			ContractVersion: RuntimeProviderContractVersion,
			ProviderVersion: "0.4.17",
			Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
			Capabilities:    referenceProviderCapabilities,
		},
		factory: func(ctx context.Context) (Provider, error) {
			compose, err := detectDockerCompose(ctx)
			if err != nil {
				return nil, err
			}
			return DockerProvider{Compose: compose}, nil
		},
	},
	ProviderPodman: {
		descriptor: ProviderDescriptor{
			Kind:            ProviderPodman,
			ContractVersion: RuntimeProviderContractVersion,
			ProviderVersion: "0.4.17",
			Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
			Capabilities:    referenceProviderCapabilities,
		},
		factory: func(ctx context.Context) (Provider, error) {
			compose, err := detectPodmanCompose(ctx)
			if err != nil {
				return nil, err
			}
			return PodmanProvider{Compose: compose}, nil
		},
	},
}

func ProviderDescriptorForKind(kind ProviderKind) (ProviderDescriptor, error) {
	normalized, err := ParseProviderKind(string(kind))
	if err != nil {
		return ProviderDescriptor{}, err
	}
	registration, ok := runtimeProviderRegistry[normalized]
	if !ok {
		return ProviderDescriptor{}, fmt.Errorf("runtime provider %q is not registered", normalized)
	}
	return registration.descriptor, nil
}

func (Compose) Descriptor() ProviderDescriptor {
	descriptor, _ := ProviderDescriptorForKind(ProviderDocker)
	return descriptor
}

func (DockerProvider) Descriptor() ProviderDescriptor {
	descriptor, _ := ProviderDescriptorForKind(ProviderDocker)
	return descriptor
}

func (PodmanProvider) Descriptor() ProviderDescriptor {
	descriptor, _ := ProviderDescriptorForKind(ProviderPodman)
	return descriptor
}

func (DockerProvider) PreferredLocalHTTPSPort() int { return 443 }
func (PodmanProvider) PreferredLocalHTTPSPort() int { return 8443 }
func (Compose) PreferredLocalHTTPSPort() int        { return 443 }

func (Compose) Capabilities() ProviderCapabilities {
	return referenceProviderCapabilities
}

// DetectProviderForKind resolves the explicitly selected deployment runtime.
// Provider selection belongs to deployment/environment state and must never be
// inferred from the portable application contract.
func DetectProviderForKind(ctx context.Context, kind ProviderKind) (Provider, error) {
	normalized, err := ParseProviderKind(string(kind))
	if err != nil {
		return nil, err
	}
	registration, ok := runtimeProviderRegistry[normalized]
	if !ok {
		return nil, fmt.Errorf("runtime provider %q is not registered", normalized)
	}
	provider, err := registration.factory(ctx)
	if err != nil {
		return nil, err
	}
	descriptor := provider.Descriptor()
	if descriptor.ContractVersion != registration.descriptor.ContractVersion ||
		descriptor.Kind != registration.descriptor.Kind {
		return nil, fmt.Errorf("runtime provider %q descriptor does not match registry declaration", normalized)
	}
	return provider, nil
}

// ResolveRuntimeProviderForKind resolves an executable runtime implementation
// behind the provider-neutral orchestration contract.
func ResolveRuntimeProviderForKind(ctx context.Context, kind ProviderKind) (RuntimeProvider, error) {
	provider, err := DetectProviderForKind(ctx, kind)
	if err != nil {
		return nil, err
	}
	runtimeProvider, ok := provider.(RuntimeProvider)
	if !ok {
		return nil, fmt.Errorf("runtime provider %q does not implement the BaseHarbor runtime contract", provider.Kind())
	}
	return runtimeProvider, nil
}

func ResolveRuntimeProvider(ctx context.Context) (RuntimeProvider, error) {
	return ResolveRuntimeProviderForKind(ctx, ProviderDocker)
}

func DetectProvider(ctx context.Context) (Provider, error) {
	return DetectProviderForKind(ctx, ProviderDocker)
}
