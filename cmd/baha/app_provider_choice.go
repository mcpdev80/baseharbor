package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// appProviderPreference wraps the existing deployment preference with its Target.
type appProviderPreference struct {
	Target string
	application.ProviderPreference
}

type appProviderCandidate struct {
	Instance capability.ProviderInstance `json:"instance"`
	Reason   string                      `json:"reason,omitempty"`
}

type appProviderChoice struct {
	Target              string                 `json:"target"`
	Requirement         capability.Requirement `json:"requirement"`
	Candidates          []appProviderCandidate `json:"candidates,omitempty"`
	PreferredInstanceID string                 `json:"preferred_instance_id,omitempty"`
	ProposedInstanceID  string                 `json:"proposed_instance_id,omitempty"`
	Reason              string                 `json:"reason,omitempty"`
	BoundInstanceID     string                 `json:"bound_instance_id,omitempty"`
}

// appProviderChoices builds the authoritative choice surface for one
// application. The authoritative footprint owns automatic resolution and
// binding precedence; this adapter only displays the eligible registry
// inventory and collects intent.
func appProviderChoices(manifest application.Manifest, snapshot application.ResolutionSnapshot, selection appCapabilitySelectionModel) ([]appProviderChoice, error) {
	return appProviderChoicesWithView(manifest, snapshot, selection, nil, false)
}

// appProviderChoicesWithPreferences is the wizard/edit choice surface over the
// same authority. It additionally offers the live instances named by the given
// explicit preferences when the snapshot projection lost them, so registry
// drift never hides the instance the consumer must re-resolve against.
func appProviderChoicesWithPreferences(manifest application.Manifest, snapshot application.ResolutionSnapshot, selection appCapabilitySelectionModel, preferences []application.ProviderPreference) ([]appProviderChoice, error) {
	return appProviderChoicesWithView(manifest, snapshot, selection, preferences, false)
}

// appProviderChoicesWithView is the wizard/edit choice surface over the same
// authority. In the selection view a requirement whose resolution fails on
// THIS requirement's kind or its bound instance (an unmaterialized explicit
// provider kind, an ambiguous or conflicting binding) stays unresolved with
// the concrete reason visible on its own line instead of aborting the whole
// selection; a failure anywhere else, especially a dependency edge of any
// record on the Target, keeps aborting with the typed error, as the
// plan/apply authorities do.
func appProviderChoicesWithView(manifest application.Manifest, snapshot application.ResolutionSnapshot, selection appCapabilitySelectionModel, preferences []application.ProviderPreference, selectionView bool) ([]appProviderChoice, error) {
	if strings.TrimSpace(snapshot.Target) == "" {
		return nil, fmt.Errorf("Target is required")
	}
	if err := snapshot.Registry.Validate(); err != nil {
		return nil, fmt.Errorf("Target registry: %w", err)
	}
	requirements := selection.Requirements()
	offered, err := liveCanonicalChoiceInstances(preferences)
	if err != nil {
		return nil, err
	}
	choicePreferences := make([]application.ProviderPreference, 0, len(preferences))
	for _, preference := range preferences {
		if len(offered[preference.Requirement]) > 0 {
			choicePreferences = append(choicePreferences, preference)
		}
	}
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
		for _, offer := range offered[req] {
			duplicate := false
			for _, candidate := range c.Candidates {
				duplicate = duplicate || candidate.Instance.ID == offer.ID
			}
			if duplicate {
				continue
			}
			c.Candidates = append(c.Candidates, appProviderCandidate{
				Instance: offer,
				Reason:   fmt.Sprintf("%s supports %s (scope: %s, re-resolved from the live Target registry)", offer.Provider.Kind, req.Kind, offer.Scope),
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
	// Resolve each currently selected requirement independently through the
	// existing authority, so one ambiguity cannot suppress other proposals.
	resolved := make(map[capability.Requirement]string)
	unresolved := make(map[capability.Requirement]error)
	for _, req := range requirements {
		projected, err := appProviderChoiceProjection(manifest, req)
		if err != nil {
			return nil, err
		}
		// The choice surface resolves requirements independently through the
		// existing authority, so one ambiguity cannot suppress other
		// proposals. In the selection view an unmaterialized explicit provider
		// kind of THIS requirement additionally remains an unresolved choice
		// line instead of aborting the whole selection; every other failure —
		// a dependency edge, a foreign or wrong-Target instance — keeps
		// aborting with the concrete typed error, exactly as the
		// authoritative plan/apply surfaces do.
		footprint, footprintErr := application.ResolveFootprint(projected, snapshot, nil)
		if footprintErr != nil {
			var resolution *application.ResolutionError
			if errors.As(footprintErr, &resolution) && resolution.Requirement == req {
				switch resolution.Code {
				case "ambiguous":
					continue
				case "missing":
					if selectionView {
						unresolved[req] = footprintErr
					}
					continue
				}
			}
			return nil, fmt.Errorf("resolve %s/%s: %w", req.Kind, req.Name, footprintErr)
		}
		for _, item := range footprint.Requirements {
			if item.Requirement == req {
				resolved[req] = item.InstanceID
			}
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
		case unresolved[c.Requirement] != nil:
			var resolution *application.ResolutionError
			if errors.As(unresolved[c.Requirement], &resolution) && resolution.Requirement == c.Requirement && len(c.Candidates) == 0 {
				// A requirement nothing on this Target can satisfy: name the
				// provider kind and the concrete way out.
				c.Reason = fmt.Sprintf("Automatic: no %s provider instance is registered on this Target; choose a compatible instance explicitly", c.Requirement.Kind)
			} else {
				c.Reason = fmt.Sprintf("Unresolved: %v; choose an explicit compatible instance or confirm this need to see the full Target conflict", unresolved[c.Requirement])
			}
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

// appProviderChoiceProjection is ephemeral: it uses the existing manifest
// constructors to represent only the currently selected logical requirement.
func appProviderChoiceProjection(original application.Manifest, req capability.Requirement) (application.Manifest, error) {
	m := application.New(original.Name, original.Environment, false, false, false)
	m.ApplicationID = original.ApplicationID
	// Preserve portable endpoint identity; the constructor detaches the slice.
	m = application.WithWorkloadComponents(m, original.Workload.Components...)
	switch req.Kind {
	case capability.SQL:
		m = application.WithSQLInstances(m, req.Name)
	case capability.KeyValue:
		m = application.WithCacheInstances(m, req.Name)
	case capability.DurableKeyValue:
		m = application.WithKeyValueInstances(m, req.Name)
	case capability.DocumentDatabase:
		m = application.WithDocumentDatabaseInstances(m, req.Name)
	case capability.MessagingQueue:
		m = application.WithMessagingQueueInstances(m, req.Name)
	case capability.MessagingPubSub:
		m = application.WithMessagingPubSubInstances(m, req.Name)
	case capability.MessagingStream:
		m = application.WithMessagingStreamInstances(m, req.Name)
	case capability.ObjectStorageS3:
		m = application.WithObjectStorageBuckets(m, req.Name)
	case capability.Identity:
		m = application.WithIdentity(m)
	case capability.Secrets:
		m.Services.Secrets = true
	case capability.TelemetryOTLP:
		m = application.WithOTLPTelemetry(m, "traces")
	case capability.ExposureHTTP:
		for _, x := range original.Exposures {
			if x.Name == req.Name {
				m.Exposures = append(m.Exposures, x)
			}
		}
		if len(m.Exposures) == 0 {
			return application.Manifest{}, fmt.Errorf("HTTP exposure %q requires a portable declaration", req.Name)
		}
	case capability.Metrics:
		for _, x := range original.Metrics.Sources {
			if x.Name == req.Name {
				m.Metrics.Sources = append(m.Metrics.Sources, x)
			}
		}
		if len(m.Metrics.Sources) == 0 {
			return application.Manifest{}, fmt.Errorf("metrics source %q requires a portable declaration", req.Name)
		}
	case capability.Logs:
		m = application.WithWorkloadComponents(m, req.Name)
		m = application.WithLogsCollection(m, req.Name)
	default:
		return application.Manifest{}, fmt.Errorf("unsupported selected requirement %s/%s", req.Kind, req.Name)
	}
	return m, nil
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

// liveCanonicalChoiceInstances re-offers, per explicitly preferred
// requirement, the exact provider instances the live provider registry of the
// current data root still registers under a BaseHarbor-owned provider that
// supports the requirement. Registry drift (a projection that lost a live
// shared instance) must not hide the instance the consumer re-resolves
// against; an instance that exists nowhere yields nothing and fails closed.
func liveCanonicalChoiceInstances(preferences []application.ProviderPreference) (map[capability.Requirement][]capability.ProviderInstance, error) {
	offered := map[capability.Requirement][]capability.ProviderInstance{}
	if len(preferences) == 0 {
		return offered, nil
	}
	dataRoot, err := deployment.DataRoot()
	if err != nil {
		return nil, err
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(dataRoot, "provider-registry.json")}).Load()
	if err != nil {
		return offered, nil
	}
	for _, preference := range preferences {
		if preference.InstanceID == "" {
			continue
		}
		for _, instance := range registry.Instances {
			if instance.ID != preference.InstanceID || instance.Ownership != capability.OwnershipBaseHarbor ||
				!instance.Provider.Supports(preference.Requirement.Kind) {
				continue
			}
			offered[preference.Requirement] = append(offered[preference.Requirement], instance)
		}
	}
	return offered, nil
}
