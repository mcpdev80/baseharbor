package delivery

import (
	"fmt"
	"strings"
)

const ContractVersion = "baseharbor.delivery/v1"

type Mode string

const (
	ModeDirect    Mode = "direct"
	ModeDelegated Mode = "delegated"
)

type ReconciliationOwner string

const (
	OwnerBaseHarbor ReconciliationOwner = "baseharbor"
	OwnerExternal   ReconciliationOwner = "external"
)

type Placement string

const (
	PlacementApplication Placement = "application"
	PlacementShared      Placement = "shared"
	PlacementExternal    Placement = "external"
)

type Selection struct {
	Mode       Mode                `json:"mode"`
	Provider   string              `json:"provider"`
	Owner      ReconciliationOwner `json:"owner"`
	Placement  Placement           `json:"placement,omitempty"`
	Revision   string              `json:"revision,omitempty"`
	Provenance string              `json:"provenance,omitempty"`
}

func Direct() Selection {
	return Selection{
		Mode:      ModeDirect,
		Provider:  "direct",
		Owner:     OwnerBaseHarbor,
		Placement: PlacementApplication,
	}
}

func (s Selection) Normalize() Selection {
	out := s
	out.Mode = Mode(strings.ToLower(strings.TrimSpace(string(out.Mode))))
	out.Provider = strings.ToLower(strings.TrimSpace(out.Provider))
	out.Owner = ReconciliationOwner(strings.ToLower(strings.TrimSpace(string(out.Owner))))
	out.Placement = Placement(strings.ToLower(strings.TrimSpace(string(out.Placement))))
	out.Revision = strings.TrimSpace(out.Revision)
	out.Provenance = strings.TrimSpace(out.Provenance)
	if out.Mode == "" {
		out.Mode = ModeDirect
	}
	if out.Provider == "" && out.Mode == ModeDirect {
		out.Provider = "direct"
	}
	if out.Owner == "" && out.Mode == ModeDirect {
		out.Owner = OwnerBaseHarbor
	}
	if out.Placement == "" {
		out.Placement = PlacementApplication
	}
	return out
}

func (s Selection) Validate() error {
	s = s.Normalize()
	switch s.Mode {
	case ModeDirect, ModeDelegated:
	default:
		return fmt.Errorf("unsupported delivery mode %q", s.Mode)
	}
	if s.Provider == "" {
		return fmt.Errorf("delivery provider is required")
	}
	switch s.Owner {
	case OwnerBaseHarbor, OwnerExternal:
	default:
		return fmt.Errorf("unsupported reconciliation owner %q", s.Owner)
	}
	switch s.Placement {
	case PlacementApplication, PlacementShared, PlacementExternal:
	default:
		return fmt.Errorf("unsupported delivery placement %q", s.Placement)
	}
	if s.Mode == ModeDirect && s.Owner != OwnerBaseHarbor {
		return fmt.Errorf("direct delivery requires BaseHarbor reconciliation ownership")
	}
	if s.Mode == ModeDelegated && s.Owner != OwnerExternal {
		return fmt.Errorf("delegated delivery requires external reconciliation ownership")
	}
	if s.Mode == ModeDirect && s.Provider != "direct" {
		return fmt.Errorf("direct delivery provider must be %q", "direct")
	}
	if s.Mode == ModeDelegated && s.Provider == "direct" {
		return fmt.Errorf("delegated delivery cannot use direct provider")
	}
	return nil
}
