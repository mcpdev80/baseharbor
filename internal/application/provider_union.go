package application

import (
	"fmt"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const ProviderPlanVersion = "baseharbor.provider-plan/v1"

// ConsumerIntent is an ephemeral projection of an existing authoritative owner.
// Management/Console consumers supply their actual security requirements through
// the same Manifest capability projection; no mandatory bundle is inferred.
// Inactive entries release bindings but retain identity for safe removal/retry.
type ConsumerIntent struct {
	Key             string               `json:"key"`
	Active          bool                 `json:"active"`
	Manifest        Manifest             `json:"manifest"`
	DependencyRoots []string             `json:"dependency_roots"`
	Preferences     []ProviderPreference `json:"preferences,omitempty"`
	// RetainedIntents are existing canonical pending/applied records of THIS
	// active app/environment owner, never another consumer or authored identity.
	RetainedIntents []Manifest `json:"retained_intents,omitempty"`
}

type ProviderChange struct {
	InstanceID string                       `json:"instance_id"`
	Action     string                       `json:"action"`
	Reason     string                       `json:"reason"`
	Ownership  capability.ProviderOwnership `json:"ownership"`
	Consumers  []string                     `json:"consumers"`
	// Runtime convergence never authorizes persistent-data deletion.
	PreserveData bool `json:"preserve_data"`
}

type ProviderPlan struct {
	ConsumerActivity map[string]bool              `json:"consumer_activity,omitempty"`
	Impact           *ProviderImpactPlan          `json:"impact,omitempty"`
	Version          string                       `json:"version"`
	Target           string                       `json:"target"`
	Complete         bool                         `json:"complete"`
	Consumers        map[string]Footprint         `json:"consumers"`
	Required         []string                     `json:"required"`
	Changes          []ProviderChange             `json:"changes"`
	ReleasedBindings []capability.ProviderBinding `json:"released_bindings"`
}

// ResolveProviderPlan unions ResolveFootprint results from ALL authoritative
// consumers on one Target. inventoryComplete must be proven by the existing
// state owner. This is read-only, not a second resolver or persistence registry.
// Unknown/partial inventory never authorizes stop/remove. Session 2 applies
// changes only through existing plan/preflight/confirm/apply/verify authority.
func ResolveProviderPlan(snapshot ResolutionSnapshot, consumers []ConsumerIntent, inventoryComplete bool) (ProviderPlan, error) {
	p := ProviderPlan{Version: ProviderPlanVersion, Target: snapshot.Target, Complete: inventoryComplete,
		ConsumerActivity: map[string]bool{}, Consumers: map[string]Footprint{}, Required: []string{}, Changes: []ProviderChange{}, ReleasedBindings: []capability.ProviderBinding{}}
	if snapshot.Target == "" {
		return ProviderPlan{}, fmt.Errorf("selected Target is required")
	}
	if err := snapshot.Registry.Validate(); err != nil {
		return ProviderPlan{}, err
	}
	instances, err := indexResolutionInstances(snapshot)
	if err != nil {
		return ProviderPlan{}, err
	}
	seen := map[string]bool{}
	owners := map[string]bool{}
	required := map[string][]string{}
	released := map[int]bool{}
	for _, consumer := range consumers {
		if consumer.Key == "" || seen[consumer.Key] {
			return ProviderPlan{}, fmt.Errorf("missing or duplicate consumer key %q", consumer.Key)
		}
		seen[consumer.Key] = true
		p.ConsumerActivity[consumer.Key] = consumer.Active
		m := consumer.Manifest
		if err := m.ValidateIntent(); err != nil {
			return ProviderPlan{}, fmt.Errorf("consumer %s: %w", consumer.Key, err)
		}
		if m.ApplicationID == "" {
			p.Complete = false
		} else {
			owner := m.ApplicationID + "/" + m.Environment
			if owners[owner] {
				return ProviderPlan{}, fmt.Errorf("duplicate consumer identity %q", owner)
			}
			owners[owner] = true
		}
		wanted := map[capability.Requirement]bool{}
		if err := validateRetainedConsumerIntents(consumer); err != nil {
			return ProviderPlan{}, fmt.Errorf("consumer %s: %w", consumer.Key, err)
		}
		if consumer.Active {
			fp, requirements, err := resolveRetainedConsumerFootprint(snapshot, consumer)
			if err != nil {
				return ProviderPlan{}, fmt.Errorf("consumer %s: %w", consumer.Key, err)
			}
			wanted = requirements
			p.Consumers[consumer.Key] = fp
			if !fp.Complete {
				p.Complete = false
			}
			for _, group := range [][]ProviderFootprint{fp.Existing, fp.Additional} {
				for _, provider := range group {
					required[provider.InstanceID] = append(required[provider.InstanceID], consumer.Key)
				}
			}
		}
		for index, binding := range snapshot.Registry.Bindings {
			if binding.ApplicationID == m.ApplicationID && binding.Environment == m.Environment && !wanted[capability.Requirement{Kind: binding.Resource.Kind, Name: binding.Resource.Name}] {
				released[index] = true
			}
		}
	}
	// Every persisted binding must be accounted for by a scoped owner, including
	// explicitly inactive consumers. Missing consumer inventory fails closed.
	for index, binding := range snapshot.Registry.Bindings {
		if !owners[binding.ApplicationID+"/"+binding.Environment] {
			p.Complete = false
		}
		if released[index] {
			p.ReleasedBindings = append(p.ReleasedBindings, binding)
		}
	}
	for id := range required {
		p.Required = append(p.Required, id)
	}
	sort.Strings(p.Required)
	for _, instance := range orderedResolutionInstances(instances) {
		fact, observed := snapshot.Facts[instance.ID]
		refs := append([]string{}, required[instance.ID]...)
		sort.Strings(refs)
		action, reason := "preserve", "inventory or ownership is not verified"
		if instance.Ownership == capability.OwnershipExternal {
			reason = "external/BYO resource"
		} else if len(refs) > 0 {
			action, reason = "reuse", "required by active consumer or dependency"
			switch fact.State {
			case ProviderNewRequired:
				action = "add"
			case ProviderStoppedOwned:
				action = "restart"
			}
		} else if p.Complete && observed && fact.Target == snapshot.Target && fact.Dependencies != nil {
			switch fact.State {
			case ProviderRunning:
				action, reason = "stop", "last active consumer released; preserve persistent data"
			case ProviderStoppedOwned:
				action, reason = "remove-runtime", "unneeded owned stopped runtime; preserve persistent data"
			case ProviderNewRequired:
				reason = "unmaterialized and not required"
			}
		}
		verifiedState := fact.State == ProviderRunning || fact.State == ProviderStoppedOwned || fact.State == ProviderNewRequired
		// External runtime is never a native removal target. A registered
		// reference with known local dependencies does not fabricate running
		// state, yet must not veto verified retirement of unrelated own modules.
		externalReference := instance.Scope == capability.ScopeExternal && instance.Ownership == capability.OwnershipExternal && fact.State == ProviderUnverifiable
		if !observed || (!verifiedState && !externalReference) || fact.Target != snapshot.Target || fact.Dependencies == nil || (instance.Scope == capability.ScopeApplication && !owners[instance.OwnerApplicationID+"/"+instance.OwnerEnvironment]) {
			action = "preserve"
			reason = "runtime ownership/state is unverifiable"
			p.Complete = false
		}
		p.Changes = append(p.Changes, ProviderChange{InstanceID: instance.ID, Action: action, Reason: reason, Ownership: instance.Ownership, Consumers: refs, PreserveData: true})
	}
	// A later unknown fact invalidates earlier removal decisions too.
	if !p.Complete {
		for i := range p.Changes {
			if p.Changes[i].Action == "stop" || p.Changes[i].Action == "remove-runtime" {
				p.Changes[i].Action = "preserve"
				p.Changes[i].Reason = "incomplete Target inventory"
			}
		}
	}
	sort.Slice(p.Changes, func(i, j int) bool { return p.Changes[i].InstanceID < p.Changes[j].InstanceID })
	return p, nil
}
