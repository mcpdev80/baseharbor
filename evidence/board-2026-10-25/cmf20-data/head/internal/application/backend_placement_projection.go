package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/provider/builtin"
)

// WithBackendPlacementProjection pins a canonical runtime owner's placement
// while inspecting retained intent. It does not select or persist a provider,
// and the projection never enters the portable manifest or its JSON encoding.
func WithBackendPlacementProjection(m Manifest, placements map[capability.ProviderKind]capability.ProviderPlacement) (Manifest, error) {
	projection := map[capability.ProviderKind]capability.ProviderPlacement{}
	for kind, placement := range m.backendPlacements {
		projection[kind] = placement
	}
	for kind, placement := range placements {
		if kind != capability.ProviderPostgreSQL && kind != capability.ProviderValkey {
			return Manifest{}, fmt.Errorf("unsupported native backend placement projection")
		}
		provider, err := builtin.Lookup(kind)
		if err != nil {
			return Manifest{}, err
		}
		if err := capability.ValidateProviderPlacement(provider.Integration, placement); err != nil {
			return Manifest{}, err
		}
		projection[kind] = placement
	}
	m.backendPlacements = projection
	return m, nil
}
