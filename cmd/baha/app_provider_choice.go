package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

type appProviderPreference struct {
	Target     string
	Kind       capability.Kind
	InstanceID string
}

type appProviderCandidate struct {
	Instance capability.ProviderInstance
	Reason   string
}

type appProviderChoice struct {
	Target              string
	Requirement         capability.Requirement
	Candidates          []appProviderCandidate
	PreferredInstanceID string
	ProposedInstanceID  string
	Reason              string
}

func appProviderChoices(target, applicationID string, selection appCapabilitySelectionModel, registry capability.Registry) ([]appProviderChoice, error) {
	if strings.TrimSpace(target) == "" {
		return nil, fmt.Errorf("Target is required")
	}
	choices := make([]appProviderChoice, 0)
	for _, req := range selection.Requirements() {
		c := appProviderChoice{Target: target, Requirement: req}
		for _, instance := range registry.Instances {
			if !instance.Provider.Supports(req.Kind) {
				continue
			}
			switch instance.Scope {
			case capability.ScopeApplication:
				if instance.OwnerApplicationID != applicationID {
					continue
				}
			case capability.ScopeShared, capability.ScopeExternal:
			default:
				continue
			}
			reason := fmt.Sprintf("%s supports %s (scope: %s)", instance.Provider.Kind, req.Kind, instance.Scope)
			c.Candidates = append(c.Candidates, appProviderCandidate{Instance: instance, Reason: reason})
		}
		sort.Slice(c.Candidates, func(i, j int) bool { return c.Candidates[i].Instance.ID < c.Candidates[j].Instance.ID })
		switch len(c.Candidates) {
		case 0:
			c.Reason = "No compatible provider instance is available on this Target"
		case 1:
			c.ProposedInstanceID = c.Candidates[0].Instance.ID
			c.Reason = "Automatic: " + c.Candidates[0].Reason
		default:
			c.Reason = "Automatic: multiple compatible instances; explicit selection is required before binding"
		}
		choices = append(choices, c)
	}
	return choices, nil
}

func (c appProviderChoice) Select(instanceID string) (appProviderChoice, error) {
	if instanceID == "" {
		c.PreferredInstanceID = ""
		return c, nil
	}
	for _, candidate := range c.Candidates {
		if candidate.Instance.ID == instanceID {
			c.PreferredInstanceID = instanceID
			return c, nil
		}
	}
	return c, fmt.Errorf("provider instance %q is unavailable or incompatible for %s on Target %q", instanceID, c.Requirement.Kind, c.Target)
}

func (c appProviderChoice) Preference() appProviderPreference {
	return appProviderPreference{Target: c.Target, Kind: c.Requirement.Kind, InstanceID: c.PreferredInstanceID}
}

func (c appProviderChoice) Effective() (capability.ProviderInstance, error) {
	id := c.PreferredInstanceID
	if id == "" {
		id = c.ProposedInstanceID
	}
	if id == "" {
		return capability.ProviderInstance{}, fmt.Errorf("%s on Target %q: %s", c.Requirement.Kind, c.Target, c.Reason)
	}
	for _, candidate := range c.Candidates {
		if candidate.Instance.ID == id {
			return candidate.Instance, nil
		}
	}
	return capability.ProviderInstance{}, fmt.Errorf("selected provider instance %q is no longer compatible", id)
}

func (c appProviderChoice) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s on Target %s — Automatic (default)\n", c.Requirement.Kind, c.Target)
	fmt.Fprintf(&b, "Proposed: %s\nReason: %s\n", c.ProposedInstanceID, c.Reason)
	for _, candidate := range c.Candidates {
		mark := " "
		if candidate.Instance.ID == c.PreferredInstanceID {
			mark = "x"
		}
		fmt.Fprintf(&b, "[%s] %s (%s): %s\n", mark, candidate.Instance.ID, candidate.Instance.Provider.Kind, candidate.Reason)
	}
	return b.String()
}
