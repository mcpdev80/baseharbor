package contract

import (
	"fmt"
	"strings"
)

type ProviderKind string

const (
	ProviderCompose    ProviderKind = "compose"
	ProviderKubernetes ProviderKind = "kubernetes"
)

type RuntimeCapability string

const (
	CapabilityWorkloadLifecycle RuntimeCapability = "workload-lifecycle"
	CapabilityServiceExec       RuntimeCapability = "service-exec"
	CapabilityPublishedPorts    RuntimeCapability = "published-ports"
	CapabilityResourceOwnership RuntimeCapability = "resource-ownership"
)

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
	case ProviderCompose, ProviderKubernetes:
		return kind, nil
	default:
		return "", fmt.Errorf("unsupported runtime provider %q", value)
	}
}

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
