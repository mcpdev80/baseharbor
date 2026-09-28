package runtime

import (
	"context"
	"fmt"
	"regexp"
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

// Provider is the minimal runtime-provider seam exposed to orchestration.
type Provider interface {
	Kind() ProviderKind
	Descriptor() ProviderDescriptor
	Capabilities() ProviderCapabilities
}

type ProviderFactory func(context.Context) (Provider, error)

type ProviderRegistration struct {
	Descriptor ProviderDescriptor
	Factory    ProviderFactory
}

type ProviderRegistry struct {
	registrations map[ProviderKind]ProviderRegistration
}

var providerKindPattern = regexp.MustCompile("^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?(?:/[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?)*$")

func ParseProviderKind(value string) (ProviderKind, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return ProviderDocker, nil
	}
	if !providerKindPattern.MatchString(normalized) {
		return "", fmt.Errorf("invalid runtime provider %q", value)
	}
	return ProviderKind(normalized), nil
}

func NewProviderRegistry(registrations ...ProviderRegistration) (*ProviderRegistry, error) {
	registry := &ProviderRegistry{registrations: make(map[ProviderKind]ProviderRegistration, len(registrations))}
	for _, registration := range registrations {
		if err := registry.Register(registration); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *ProviderRegistry) Register(registration ProviderRegistration) error {
	if r == nil {
		return fmt.Errorf("runtime provider registry is required")
	}
	if registration.Descriptor.Kind == "" {
		return fmt.Errorf("runtime provider descriptor kind is required")
	}
	kind, err := ParseProviderKind(string(registration.Descriptor.Kind))
	if err != nil {
		return err
	}
	if kind != registration.Descriptor.Kind {
		return fmt.Errorf("runtime provider descriptor kind %q is not normalized", registration.Descriptor.Kind)
	}
	if registration.Descriptor.ContractVersion != RuntimeProviderContractVersion {
		return fmt.Errorf("runtime provider %q uses contract %q, require %q", kind, registration.Descriptor.ContractVersion, RuntimeProviderContractVersion)
	}
	if strings.TrimSpace(registration.Descriptor.ProviderVersion) == "" {
		return fmt.Errorf("runtime provider %q has no provider version", kind)
	}
	if registration.Factory == nil {
		return fmt.Errorf("runtime provider %q has no factory", kind)
	}
	if r.registrations == nil {
		r.registrations = map[ProviderKind]ProviderRegistration{}
	}
	if _, exists := r.registrations[kind]; exists {
		return fmt.Errorf("runtime provider %q is already registered", kind)
	}
	registration.Descriptor.Standards = append([]string(nil), registration.Descriptor.Standards...)
	r.registrations[kind] = registration
	return nil
}

func (r *ProviderRegistry) Descriptor(kind ProviderKind) (ProviderDescriptor, error) {
	if r == nil {
		return ProviderDescriptor{}, fmt.Errorf("runtime provider registry is required")
	}
	normalized, err := ParseProviderKind(string(kind))
	if err != nil {
		return ProviderDescriptor{}, err
	}
	registration, ok := r.registrations[normalized]
	if !ok {
		return ProviderDescriptor{}, fmt.Errorf("runtime provider %q is not registered", normalized)
	}
	descriptor := registration.Descriptor
	descriptor.Standards = append([]string(nil), descriptor.Standards...)
	return descriptor, nil
}

func (r *ProviderRegistry) Resolve(ctx context.Context, kind ProviderKind) (Provider, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime provider registry is required")
	}
	normalized, err := ParseProviderKind(string(kind))
	if err != nil {
		return nil, err
	}
	registration, ok := r.registrations[normalized]
	if !ok {
		return nil, fmt.Errorf("runtime provider %q is not registered", normalized)
	}
	provider, err := registration.Factory(ctx)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("runtime provider %q factory returned nil", normalized)
	}
	descriptor := provider.Descriptor()
	if descriptor.Kind != registration.Descriptor.Kind ||
		descriptor.ContractVersion != registration.Descriptor.ContractVersion ||
		descriptor.ProviderVersion != registration.Descriptor.ProviderVersion ||
		descriptor.Capabilities != registration.Descriptor.Capabilities {
		return nil, fmt.Errorf("runtime provider %q descriptor does not match registry declaration", normalized)
	}
	return provider, nil
}

func (r *ProviderRegistry) ResolveRuntimeProvider(ctx context.Context, kind ProviderKind) (RuntimeProvider, error) {
	provider, err := r.Resolve(ctx, kind)
	if err != nil {
		return nil, err
	}
	runtimeProvider, ok := provider.(RuntimeProvider)
	if !ok {
		return nil, fmt.Errorf("runtime provider %q does not implement the BaseHarbor runtime contract", provider.Kind())
	}
	return runtimeProvider, nil
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

var defaultRuntimeProviderRegistry = mustProviderRegistry(
	ProviderRegistration{
		Descriptor: ProviderDescriptor{
			Kind:            ProviderDocker,
			ContractVersion: RuntimeProviderContractVersion,
			ProviderVersion: "0.4.17",
			Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
			Capabilities:    referenceProviderCapabilities,
		},
		Factory: func(ctx context.Context) (Provider, error) {
			compose, err := detectDockerCompose(ctx)
			if err != nil {
				return nil, err
			}
			return DockerProvider{Compose: compose}, nil
		},
	},
	ProviderRegistration{
		Descriptor: ProviderDescriptor{
			Kind:            ProviderPodman,
			ContractVersion: RuntimeProviderContractVersion,
			ProviderVersion: "0.4.17",
			Standards:       []string{"OCI Image Specification", "OCI Distribution Specification", "OCI Runtime Specification", "Compose Specification"},
			Capabilities:    referenceProviderCapabilities,
		},
		Factory: func(ctx context.Context) (Provider, error) {
			runtime, err := detectPodmanRuntime(ctx)
			if err != nil {
				return nil, err
			}
			return PodmanProvider{Compose: runtime}, nil
		},
	},
)

func mustProviderRegistry(registrations ...ProviderRegistration) *ProviderRegistry {
	registry, err := NewProviderRegistry(registrations...)
	if err != nil {
		panic(err)
	}
	return registry
}

func ProviderDescriptorForKind(kind ProviderKind) (ProviderDescriptor, error) {
	return defaultRuntimeProviderRegistry.Descriptor(kind)
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

func (DockerProvider) Capabilities() ProviderCapabilities { return referenceProviderCapabilities }
func (PodmanProvider) Capabilities() ProviderCapabilities { return referenceProviderCapabilities }

// DetectProviderForKind resolves the explicitly selected deployment runtime.
// Provider selection belongs to deployment/environment state and must never be
// inferred from the portable application contract.
func DetectProviderForKind(ctx context.Context, kind ProviderKind) (Provider, error) {
	return defaultRuntimeProviderRegistry.Resolve(ctx, kind)
}

// ResolveRuntimeProviderForKind resolves an executable runtime implementation
// behind the provider-neutral orchestration contract.
func ResolveRuntimeProviderForKind(ctx context.Context, kind ProviderKind) (RuntimeProvider, error) {
	return defaultRuntimeProviderRegistry.ResolveRuntimeProvider(ctx, kind)
}

func ResolveRuntimeProvider(ctx context.Context) (RuntimeProvider, error) {
	return ResolveRuntimeProviderForKind(ctx, ProviderDocker)
}

func DetectProvider(ctx context.Context) (Provider, error) {
	return DetectProviderForKind(ctx, ProviderDocker)
}
