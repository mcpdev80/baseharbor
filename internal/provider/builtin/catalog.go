package builtin

import (
	"fmt"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Descriptor is the installable/distributable identity of a first-party
// provider implementation. Capability semantics remain owned by the versioned
// capability specifications; provider versioning is independent from the
// BaseHarbor CLI release.
type Descriptor struct {
	ID          string
	Version     string
	Integration capability.IntegrationDescriptor
}

// Lookup returns the bundled first-party provider descriptor for a provider
// kind. New core code should resolve bundled providers through this package
// instead of depending on concrete product packages directly.
func Lookup(kind capability.ProviderKind) (Descriptor, error) {
	integration, err := capability.ReferenceIntegration(kind)
	if err != nil {
		return Descriptor{}, err
	}
	if err := integration.Validate(); err != nil {
		return Descriptor{}, fmt.Errorf("validate bundled provider %q: %w", kind, err)
	}
	return Descriptor{
		ID:          integration.ID,
		Version:     integration.Version,
		Integration: integration,
	}, nil
}

// Catalog returns the deterministic bundled provider catalog.
func Catalog() ([]Descriptor, error) {
	integrations := capability.ReferenceIntegrations()
	result := make([]Descriptor, 0, len(integrations))
	for _, integration := range integrations {
		descriptor, err := Lookup(integration.Provider.Kind)
		if err != nil {
			return nil, err
		}
		result = append(result, descriptor)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, nil
}
