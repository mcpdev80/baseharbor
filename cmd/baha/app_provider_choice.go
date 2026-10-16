package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appProviderPreference wraps the existing deployment preference with its Target.
type appProviderPreference struct {
	Target string
	application.ProviderPreference
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
	BoundInstanceID     string
}

// The authoritative footprint owns automatic resolution and binding precedence.
// This adapter only displays the eligible registry inventory and collects intent.
func appProviderChoices(manifest application.Manifest, snapshot application.ResolutionSnapshot, selection appCapabilitySelectionModel) ([]appProviderChoice, error) {
	if strings.TrimSpace(snapshot.Target) == "" {
		return nil, fmt.Errorf("Target is required")
	}
	if err := snapshot.Registry.Validate(); err != nil {
		return nil, fmt.Errorf("Target registry: %w", err)
	}
	requirements := selection.Requirements()
	choices := make([]appProviderChoice, 0, len(requirements))
	for _, req := range requirements {
		c := appProviderChoice{Target: snapshot.Target, Requirement: req}
		for _, instance := range snapshot.Registry.Instances {
			if !instance.Provider.Supports(req.Kind) {
				continue
			}
			// These are visibility constraints, not a replacement resolver.
			switch instance.Scope {
			case capability.ScopeApplication:
				if manifest.ApplicationID == "" || instance.OwnerApplicationID != manifest.ApplicationID ||
					(instance.OwnerEnvironment != "" && instance.OwnerEnvironment != manifest.Environment) {
					continue
				}
			case capability.ScopeShared:
				if instance.Ownership != capability.OwnershipBaseHarbor {
					continue
				}
			case capability.ScopeExternal:
				if instance.Ownership != capability.OwnershipExternal {
					continue
				}
			default:
				continue
			}
			c.Candidates = append(c.Candidates, appProviderCandidate{
				Instance: instance,
				Reason:   fmt.Sprintf("%s supports %s (scope: %s)", instance.Provider.Kind, req.Kind, instance.Scope),
			})
		}
		sort.Slice(c.Candidates, func(i, j int) bool { return c.Candidates[i].Instance.ID < c.Candidates[j].Instance.ID })
		for _, binding := range snapshot.Registry.Bindings {
			if binding.ApplicationID == manifest.ApplicationID &&
				(binding.Environment == manifest.Environment || binding.Environment == "") &&
				binding.Resource.Kind == req.Kind && binding.Resource.Name == req.Name {
				if c.BoundInstanceID != "" && c.BoundInstanceID != binding.ProviderInstanceID {
					return nil, fmt.Errorf("ambiguous existing binding for %s/%s", req.Kind, req.Name)
				}
				c.BoundInstanceID = binding.ProviderInstanceID
			}
		}
		choices = append(choices, c)
	}
	// Ask the existing resolver for the proposed selection, including its
	// binding-conflict, ownership, and ambiguity semantics.
	footprint, err := application.ResolveFootprint(manifest, snapshot, nil)
	if err != nil {
		// Ambiguity is a valid choice state; other resolver failures are not.
		var resolution *application.ResolutionError
		if !errors.As(err, &resolution) || resolution.Code != "ambiguous" {
			return nil, fmt.Errorf("resolve Target providers: %w", err)
		}
	}
	resolved := make(map[capability.Requirement]string)
	for _, item := range footprint.Requirements {
		if item.InstanceID != "" {
			resolved[item.Requirement] = item.InstanceID
		}
	}
	for i := range choices {
		c := &choices[i]
		if id := resolved[c.Requirement]; id != "" {
			c.ProposedInstanceID = id
		} else if c.BoundInstanceID != "" {
			c.ProposedInstanceID = c.BoundInstanceID
		}
		switch {
		case c.ProposedInstanceID != "" && c.BoundInstanceID != "":
			c.Reason = "Automatic: preserved existing application/environment binding"
		case c.ProposedInstanceID != "":
			c.Reason = "Automatic: resolved by existing application authority"
		case len(c.Candidates) == 0:
			c.Reason = "No compatible provider instance is available on this Target"
		default:
			c.Reason = "Automatic: unresolved or ambiguous; choose an explicit compatible instance"
		}
	}
	return choices, nil
}

func (c appProviderChoice) Select(id string) (appProviderChoice, error) {
	if id == "" {
		c.PreferredInstanceID = ""
		return c, nil
	}
	if c.BoundInstanceID != "" && id != c.BoundInstanceID {
		return c, fmt.Errorf("existing binding to %q conflicts with requested instance %q", c.BoundInstanceID, id)
	}
	for _, candidate := range c.Candidates {
		if candidate.Instance.ID == id {
			c.PreferredInstanceID = id
			return c, nil
		}
	}
	return c, fmt.Errorf("provider instance %q is unavailable or incompatible for %s/%s on Target %q", id, c.Requirement.Kind, c.Requirement.Name, c.Target)
}

func (c appProviderChoice) Preference() appProviderPreference {
	return appProviderPreference{Target: c.Target, ProviderPreference: application.ProviderPreference{
		Requirement: c.Requirement, InstanceID: c.PreferredInstanceID,
	}}
}

func (c appProviderChoice) Effective() (capability.ProviderInstance, error) {
	id := c.PreferredInstanceID
	if id == "" {
		id = c.ProposedInstanceID
	}
	if id == "" {
		return capability.ProviderInstance{}, fmt.Errorf("%s/%s on Target %q: %s", c.Requirement.Kind, c.Requirement.Name, c.Target, c.Reason)
	}
	for _, candidate := range c.Candidates {
		if candidate.Instance.ID == id {
			return candidate.Instance, nil
		}
	}
	return capability.ProviderInstance{}, fmt.Errorf("resolved provider instance %q is not an eligible Target candidate", id)
}

func (c appProviderChoice) View() string {
	var b strings.Builder
	mode := "Automatic"
	id, reason := c.ProposedInstanceID, c.Reason
	if c.PreferredInstanceID != "" {
		mode, id = "Explicit", c.PreferredInstanceID
		reason = "Explicit: requested compatible instance for this Target"
	}
	fmt.Fprintf(&b, "%s/%s on Target %s — %s\n", c.Requirement.Kind, c.Requirement.Name, c.Target, mode)
	fmt.Fprintf(&b, "Selected: %s\nReason: %s\n", id, reason)
	for _, candidate := range c.Candidates {
		mark := " "
		if candidate.Instance.ID == id {
			mark = "x"
		}
		fmt.Fprintf(&b, "[%s] %s (%s): %s\n", mark, candidate.Instance.ID, candidate.Instance.Provider.Kind, candidate.Reason)
	}
	return b.String()
}
