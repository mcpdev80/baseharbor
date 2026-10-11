package application

import (
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/stableid"
	"sort"
)

// indexResolutionInstances observes existing runtime-owned ancillary realizations
// in the SAME dependency graph. It creates no registry entry or capability offer.
// Ancillary IDs and owner references come from existing runtime/route owners.
func indexResolutionInstances(s ResolutionSnapshot) (map[string]capability.ProviderInstance, error) {
	instances := map[string]capability.ProviderInstance{}
	for _, i := range s.Registry.Instances {
		instances[i.ID] = i
	}
	for _, i := range s.Ancillary {
		f, observed := s.Facts[i.ID]
		if i.ID == "" || i.Provider.Kind == "" || len(i.Provider.Capabilities) != 0 || !observed || !f.Ancillary || f.OwnerReference == "" {
			return nil, fmt.Errorf("ancillary observation requires existing identity/owner and no capability offer")
		}
		if _, duplicate := instances[i.ID]; duplicate {
			return nil, fmt.Errorf("duplicate ancillary/provider instance identity")
		}
		if err := safeImpactText(f.OwnerReference); err != nil {
			return nil, err
		}
		if i.Ownership != capability.OwnershipBaseHarbor {
			return nil, fmt.Errorf("ancillary runtime must have proven BaseHarbor ownership")
		}
		switch i.Scope {
		case capability.ScopeShared:
			if i.OwnerApplicationID != "" || i.OwnerApplication != "" {
				return nil, fmt.Errorf("shared ancillary uses its existing installation owner, not an application identity")
			}
		case capability.ScopeApplication:
			if err := stableid.ValidateUUIDv4("application", i.OwnerApplicationID); err != nil {
				return nil, err
			}
			if i.OwnerApplication == "" {
				return nil, fmt.Errorf("application ancillary requires its actual existing application owner")
			}
		default:
			return nil, fmt.Errorf("unsupported ancillary ownership scope")
		}
		instances[i.ID] = i
	}
	return instances, nil
}

func orderedResolutionInstances(instances map[string]capability.ProviderInstance) []capability.ProviderInstance {
	list := make([]capability.ProviderInstance, 0, len(instances))
	for _, i := range instances {
		list = append(list, i)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

func observationOwner(i capability.ProviderInstance, s ResolutionSnapshot) string {
	if s.Facts[i.ID].Ancillary {
		return s.Facts[i.ID].OwnerReference
	}
	return i.OwnerApplicationID
}
