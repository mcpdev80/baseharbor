package capability

import (
	"fmt"
	"strings"
)

// Kind identifies portable application behavior. It describes what an
// application needs, never which product or runtime satisfies that need.
type Kind string

const (
	SQL      Kind = "database.sql"
	KeyValue Kind = "cache.key-value"
	Secrets  Kind = "secrets"
)

// Requirement is one application-owned logical capability request.
type Requirement struct {
	Kind Kind   `json:"kind"`
	Name string `json:"name"`
}

// ProviderKind identifies a capability implementation. Capability providers are
// independent from runtime providers such as Compose.
type ProviderKind string

const (
	ProviderPostgreSQL ProviderKind = "postgresql"
	ProviderValkey     ProviderKind = "valkey"
	ProviderOpenBao    ProviderKind = "openbao"
)

// Provider describes the capability surface of one provider implementation.
// Placement, ownership and shared/dedicated/external scope deliberately remain
// outside this v0.4.1 foundation and are introduced by the provider registry.
type Provider struct {
	Kind         ProviderKind `json:"kind"`
	Capabilities []Kind       `json:"capabilities"`
}

func (p Provider) Supports(kind Kind) bool {
	for _, supported := range p.Capabilities {
		if supported == kind {
			return true
		}
	}
	return false
}

// Resource is the provider-backed representation of one stable logical
// application resource. Provider-specific object names, ports and credentials
// are intentionally absent.
type Resource struct {
	Application string       `json:"application"`
	Kind        Kind         `json:"kind"`
	Name        string       `json:"name"`
	Provider    ProviderKind `json:"provider"`
}

// Resolve validates capability negotiation before any provider mutation occurs.
// It intentionally performs no discovery or provider selection policy; those
// belong to the later registry layer.
func Resolve(application string, requirement Requirement, provider Provider) (Resource, error) {
	application = strings.TrimSpace(application)
	if application == "" {
		return Resource{}, fmt.Errorf("capability resolution: application is required")
	}
	if requirement.Kind == "" {
		return Resource{}, fmt.Errorf("capability resolution for %s: capability kind is required", application)
	}
	if strings.TrimSpace(requirement.Name) == "" {
		return Resource{}, fmt.Errorf("capability resolution for %s/%s: logical resource name is required", application, requirement.Kind)
	}
	if provider.Kind == "" {
		return Resource{}, fmt.Errorf("capability resolution for %s/%s/%s: provider is required", application, requirement.Kind, requirement.Name)
	}
	if !provider.Supports(requirement.Kind) {
		return Resource{}, fmt.Errorf(
			"capability provider %s does not support required capability %q",
			provider.Kind,
			requirement.Kind,
		)
	}

	return Resource{
		Application: application,
		Kind:        requirement.Kind,
		Name:        requirement.Name,
		Provider:    provider.Kind,
	}, nil
}
