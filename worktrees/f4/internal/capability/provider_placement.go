package capability

import (
	"fmt"
	"strings"
)

// ProviderPlacement describes deployment/operator intent for how a provider
// instance is placed. It is deliberately separate from portable application
// intent and from runtime-specific isolation/topology.
type ProviderPlacement struct {
	Scope             ProviderScope     `json:"scope"`
	SharingBoundary   string            `json:"sharing_boundary,omitempty"`
	Ownership         ProviderOwnership `json:"ownership"`
	ExternalReference string            `json:"external_reference,omitempty"`
}

func (p ProviderPlacement) Validate() error {
	p.SharingBoundary = strings.TrimSpace(p.SharingBoundary)
	p.ExternalReference = strings.TrimSpace(p.ExternalReference)

	switch p.Scope {
	case ScopeShared:
		if p.Ownership != OwnershipBaseHarbor {
			return fmt.Errorf("shared provider placement must be BaseHarbor-owned")
		}
		if p.ExternalReference != "" {
			return fmt.Errorf("shared provider placement cannot use an external reference")
		}
	case ScopeApplication:
		if p.Ownership != OwnershipBaseHarbor {
			return fmt.Errorf("application-scoped provider placement must be BaseHarbor-owned")
		}
		if p.SharingBoundary != "" {
			return fmt.Errorf("application-scoped provider placement cannot define a sharing boundary")
		}
		if p.ExternalReference != "" {
			return fmt.Errorf("application-scoped provider placement cannot use an external reference")
		}
	case ScopeExternal:
		if p.Ownership != OwnershipExternal {
			return fmt.Errorf("external provider placement must be externally owned")
		}
		if p.SharingBoundary != "" {
			return fmt.Errorf("external provider placement cannot define a sharing boundary")
		}
		if p.ExternalReference == "" {
			return fmt.Errorf("external provider placement requires a non-secret reference")
		}
	default:
		return fmt.Errorf("unsupported provider scope %q", p.Scope)
	}
	return nil
}

func (d IntegrationDescriptor) SupportsScope(scope ProviderScope) bool {
	for _, supported := range d.SupportedScopes {
		if supported == scope {
			return true
		}
	}
	return false
}

func ValidateProviderPlacement(descriptor IntegrationDescriptor, placement ProviderPlacement) error {
	if err := descriptor.Validate(); err != nil {
		return err
	}
	if err := placement.Validate(); err != nil {
		return err
	}
	if !descriptor.SupportsScope(placement.Scope) {
		return fmt.Errorf("provider %q does not support placement scope %q", descriptor.Provider.Kind, placement.Scope)
	}
	return nil
}
