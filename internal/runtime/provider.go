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
	ProviderCompose ProviderKind = "compose"
)

// RuntimeCapability names portable runtime behavior that orchestration may
// require from a provider. These capabilities describe the runtime substrate,
// not application capabilities such as PostgreSQL or object storage.
type RuntimeCapability string

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
	Capabilities() ProviderCapabilities
}

func ParseProviderKind(value string) (ProviderKind, error) {
	kind := ProviderKind(strings.TrimSpace(strings.ToLower(value)))
	if kind == "" {
		kind = ProviderCompose
	}
	switch kind {
	case ProviderCompose:
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
	caps := provider.Capabilities()
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

// DetectProvider is the single runtime-provider selection seam. v0.4 currently
// selects only the existing Compose implementation. Kubernetes/OpenShift must
// later be selected explicitly from deployment/environment configuration rather
// than inferred from the application contract.
func DetectProvider(ctx context.Context) (Provider, error) {
	return detectCompose(ctx)
}
