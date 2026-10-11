package application

import (
	"fmt"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const FootprintVersion = "baseharbor.footprint/v1"

// CoreRequirement is derived from selected provider declarations, never from
// HTTPS, SQL's capability name, a profile, or guessed container counts.
type CoreRequirement string

const (
	CoreNotRequired CoreRequirement = "not-required"
	CoreRequired    CoreRequirement = "required"
	CoreUnknown     CoreRequirement = "unknown"
)

type ProviderState string

const (
	ProviderRunning      ProviderState = "running"
	ProviderStoppedOwned ProviderState = "stopped-but-owned"
	ProviderNewRequired  ProviderState = "newly-required"
	ProviderUnverifiable ProviderState = "unverifiable"
	ProviderForeign      ProviderState = "foreign"
)

// Quantity is evidence for a single provider realization. Unknown has no value;
// measured/estimated require a source, and estimates are supplied by adapters.
type Quantity struct {
	Value          *int64 `json:"value,omitempty"`
	Classification string `json:"classification"`
	Source         string `json:"source,omitempty"`
}

func (q Quantity) Validate() error {
	switch q.Classification {
	case "unknown":
		if q.Value != nil {
			return fmt.Errorf("unknown quantity cannot have a value")
		}
	case "measured", "estimated":
		if q.Value == nil || *q.Value < 0 || q.Source == "" {
			return fmt.Errorf("quantity requires a nonnegative value and evidence source")
		}
	default:
		return fmt.Errorf("invalid evidence classification %q", q.Classification)
	}
	return nil
}

type ProviderFact struct {
	// Target must match the selected snapshot, including dependencies.
	Target string        `json:"target"`
	State  ProviderState `json:"state"`
	// Core comes from the existing provider/placement declaration.
	Core CoreRequirement `json:"core"`
	// Dependencies are actual selected realization IDs from existing plans.
	// Nil is an unknown dependency inventory; [] means verified no dependencies.
	Dependencies []string `json:"dependencies"`
	MemoryBytes  Quantity `json:"memory_bytes"`
	Containers   Quantity `json:"containers"`
}

// ResolutionSnapshot is an ephemeral view of authoritative Target state.
// Registry is the existing capability.Registry; Facts are observer/plan input.
type ResolutionSnapshot struct {
	Target   string                  `json:"target"`
	Registry capability.Registry     `json:"registry"`
	Facts    map[string]ProviderFact `json:"facts"`
}

type ProviderPreference struct {
	Requirement capability.Requirement `json:"requirement"`
	// Empty means automatic. Preference is deployment intent, never manifest.
	InstanceID string `json:"instance_id,omitempty"`
}

type RequirementResolution struct {
	Requirement capability.Requirement `json:"requirement"`
	Binding     string                 `json:"binding"`
	InstanceID  string                 `json:"instance_id,omitempty"`
	State       ProviderState          `json:"state,omitempty"`
}

type ProviderFootprint struct {
	InstanceID  string                       `json:"instance_id"`
	Scope       capability.ProviderScope     `json:"scope"`
	Ownership   capability.ProviderOwnership `json:"ownership"`
	Shared      bool                         `json:"shared"`
	Consumers   int                          `json:"consumers"`
	State       ProviderState                `json:"state"`
	Core        CoreRequirement              `json:"core"`
	MemoryBytes Quantity                     `json:"memory_bytes"`
	Containers  Quantity                     `json:"containers"`
}

type Footprint struct {
	Version       string                  `json:"version"`
	Target        string                  `json:"target"`
	ApplicationID string                  `json:"application_id,omitempty"`
	Requirements  []RequirementResolution `json:"requirements"`
	Existing      []ProviderFootprint     `json:"existing"`
	Additional    []ProviderFootprint     `json:"additional"`
	Unused        []string                `json:"unused"`
	Core          CoreRequirement         `json:"core"`
	Complete      bool                    `json:"complete"`
}

type ResolutionError struct {
	Code        string
	InstanceID  string
	Requirement capability.Requirement
}

func (e *ResolutionError) Error() string {
	return fmt.Sprintf("provider resolution %s for %s/%s (instance %q)", e.Code, e.Requirement.Kind, e.Requirement.Name, e.InstanceID)
}

func requestedCapabilities(m Manifest) ([]capability.Requirement, error) {
	c, err := PortableContractFromManifest(m)
	if err != nil {
		return nil, err
	}
	reqs := append([]capability.Requirement(nil), c.Capabilities...)
	if c.Secrets.Managed {
		reqs = append(reqs, capability.Requirement{Kind: capability.Secrets, Name: "default"})
	}
	return reqs, nil
}

// ResolveFootprint performs no filesystem, registry, discovery or runtime writes.
// Missing automatic selection stays requested/unbound. It is not permission to
// provision Core; Session 2 must negotiate a supported provider via existing
// plans and rerun this view with its verified declaration/snapshot.
func ResolveFootprint(m Manifest, snapshot ResolutionSnapshot, preferences []ProviderPreference) (Footprint, error) {
	result := Footprint{Version: FootprintVersion, Target: snapshot.Target, ApplicationID: m.ApplicationID,
		Requirements: []RequirementResolution{}, Existing: []ProviderFootprint{}, Additional: []ProviderFootprint{}, Unused: []string{}, Core: CoreNotRequired, Complete: true}
	reqs, err := requestedCapabilities(m)
	if err != nil {
		return Footprint{}, err
	}
	if snapshot.Target == "" {
		return Footprint{}, fmt.Errorf("selected Target is required")
	}
	if err := snapshot.Registry.Validate(); err != nil {
		return Footprint{}, fmt.Errorf("Target registry: %w", err)
	}
	known := map[capability.Requirement]bool{}
	for _, req := range reqs {
		known[req] = true
	}
	prefs := map[capability.Requirement]string{}
	for _, pref := range preferences {
		if !known[pref.Requirement] {
			return Footprint{}, fmt.Errorf("provider preference for unrequested capability")
		}
		if _, exists := prefs[pref.Requirement]; exists {
			return Footprint{}, fmt.Errorf("duplicate provider preference")
		}
		prefs[pref.Requirement] = pref.InstanceID
	}
	instances := map[string]capability.ProviderInstance{}
	for _, instance := range snapshot.Registry.Instances {
		instances[instance.ID] = instance
	}
	selected := map[string]bool{}
	visiting := map[string]bool{}
	var include func(string) (ProviderState, error)
	include = func(id string) (ProviderState, error) {
		instance, exists := instances[id]
		if !exists {
			return "", &ResolutionError{Code: "missing-dependency", InstanceID: id}
		}
		if visiting[id] {
			return "", &ResolutionError{Code: "dependency-cycle", InstanceID: id}
		}
		if !eligibleInstance(instance, m) {
			return "", &ResolutionError{Code: "foreign", InstanceID: id}
		}
		fact, observed := snapshot.Facts[id]
		if observed && fact.Target != snapshot.Target {
			return "", &ResolutionError{Code: "foreign-target", InstanceID: id}
		}
		if !observed {
			fact = ProviderFact{State: ProviderUnverifiable, Core: CoreUnknown}
		}
		if fact.State == ProviderUnverifiable || fact.Dependencies == nil {
			result.Complete = false
		}
		if fact.State == ProviderForeign {
			return "", &ResolutionError{Code: "foreign", InstanceID: id}
		}
		switch fact.State {
		case ProviderRunning, ProviderStoppedOwned, ProviderNewRequired, ProviderUnverifiable:
		default:
			return "", fmt.Errorf("invalid provider state for %q", id)
		}
		if instance.Ownership == capability.OwnershipExternal {
			if fact.State == ProviderNewRequired || fact.State == ProviderStoppedOwned {
				return "", fmt.Errorf("external provider cannot require owned provisioning or restart")
			}
			fact.Core = CoreNotRequired
		}
		if selected[id] {
			return fact.State, nil
		}
		visiting[id] = true
		if fact.Core == "" {
			fact.Core = CoreUnknown
		}
		switch fact.Core {
		case CoreRequired:
			result.Core = CoreRequired
		case CoreUnknown:
			result.Complete = false
			if result.Core != CoreRequired {
				result.Core = CoreUnknown
			}
		case CoreNotRequired:
		default:
			return "", fmt.Errorf("invalid Core requirement for %q", id)
		}
		if fact.Dependencies == nil && result.Core != CoreRequired {
			result.Core = CoreUnknown
		}
		for _, dependency := range fact.Dependencies {
			if _, err := include(dependency); err != nil {
				return "", err
			}
		}
		delete(visiting, id)
		selected[id] = true
		if fact.MemoryBytes.Classification == "" {
			fact.MemoryBytes = Quantity{Classification: "unknown"}
		}
		if fact.Containers.Classification == "" {
			fact.Containers = Quantity{Classification: "unknown"}
		}
		if err := fact.MemoryBytes.Validate(); err != nil {
			return "", err
		}
		if err := fact.Containers.Validate(); err != nil {
			return "", err
		}
		fp := ProviderFootprint{InstanceID: id, Scope: instance.Scope, Ownership: instance.Ownership,
			Shared: instance.Scope == capability.ScopeShared, Consumers: len(snapshot.Registry.BindingsForProviderInstance(id)),
			State: fact.State, Core: fact.Core, MemoryBytes: fact.MemoryBytes, Containers: fact.Containers}
		if fact.State == ProviderNewRequired {
			result.Additional = append(result.Additional, fp)
		} else {
			result.Existing = append(result.Existing, fp)
		}
		return fact.State, nil
	}
	for _, req := range reqs {
		id := prefs[req]
		bound := ""
		for _, binding := range snapshot.Registry.Bindings {
			if m.ApplicationID != "" && binding.ApplicationID == m.ApplicationID &&
				(binding.Environment == m.Environment || binding.Environment == "") &&
				binding.Resource.Kind == req.Kind && binding.Resource.Name == req.Name {
				if bound != "" && bound != binding.ProviderInstanceID {
					return Footprint{}, &ResolutionError{Code: "ambiguous", Requirement: req}
				}
				bound = binding.ProviderInstanceID
			}
		}
		if bound != "" {
			if id != "" && id != bound {
				return Footprint{}, &ResolutionError{Code: "binding-conflict", Requirement: req, InstanceID: id}
			}
			id = bound
		}
		if id == "" {
			result.Complete = false
			for _, instance := range snapshot.Registry.Instances {
				if instance.Provider.Supports(req.Kind) && eligibleInstance(instance, m) {
					if id != "" {
						return Footprint{}, &ResolutionError{Code: "ambiguous", Requirement: req}
					}
					id = instance.ID
				}
			}
		}
		entry := RequirementResolution{Requirement: req, Binding: "requested/unbound"}
		if id == "" {
			if result.Core != CoreRequired {
				result.Core = CoreUnknown
			}
			result.Requirements = append(result.Requirements, entry)
			continue
		}
		instance, exists := instances[id]
		if !exists {
			return Footprint{}, &ResolutionError{Code: "missing", Requirement: req, InstanceID: id}
		}
		if !instance.Provider.Supports(req.Kind) {
			return Footprint{}, &ResolutionError{Code: "incompatible", Requirement: req, InstanceID: id}
		}
		state, err := include(id)
		if err != nil {
			return Footprint{}, err
		}
		entry.InstanceID, entry.State = id, state
		if bound != "" {
			entry.Binding = "bound"
		} else {
			entry.Binding = "requested/unbound"
		}
		result.Requirements = append(result.Requirements, entry)
	}
	// These existing intents are not represented by PortableContract's service
	// projection. Their lifecycle/consumption dependency plan must be verified
	// before reporting a Core-free result; never infer no dependency from absence.
	if len(m.Runtime.Permissions) > 0 || len(m.Consumes) > 0 {
		result.Complete = false
		if result.Core != CoreRequired {
			result.Core = CoreUnknown
		}
	}
	for _, instance := range snapshot.Registry.Instances {
		if !selected[instance.ID] && len(snapshot.Registry.BindingsForProviderInstance(instance.ID)) == 0 {
			result.Unused = append(result.Unused, instance.ID)
		}
	}
	sort.Strings(result.Unused)
	sort.Slice(result.Existing, func(i, j int) bool { return result.Existing[i].InstanceID < result.Existing[j].InstanceID })
	sort.Slice(result.Additional, func(i, j int) bool { return result.Additional[i].InstanceID < result.Additional[j].InstanceID })
	return result, nil
}

func eligibleInstance(instance capability.ProviderInstance, m Manifest) bool {
	switch instance.Scope {
	case capability.ScopeShared:
		return instance.Ownership == capability.OwnershipBaseHarbor
	case capability.ScopeExternal:
		return instance.Ownership == capability.OwnershipExternal
	case capability.ScopeApplication:
		return m.ApplicationID != "" && instance.OwnerApplicationID == m.ApplicationID &&
			(instance.OwnerEnvironment == "" || instance.OwnerEnvironment == m.Environment)
	default:
		return false
	}
}

// ManagementCoreRequirements preserves #810 independently of Application needs.
func ManagementCoreRequirements() []capability.Kind {
	return []capability.Kind{capability.SQL, capability.Secrets, capability.Identity}
}
