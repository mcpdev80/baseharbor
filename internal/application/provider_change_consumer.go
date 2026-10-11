package application

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// ConsumerTransition is an observed operation of an existing consumer adapter,
// explicitly selected by ID in the request. It is not a generic auth fallback.
// Before/After contain references only; no password comparison or regeneration
// occurs in this read-only projection. The adapter verifies retained values.
type ConsumerTransition struct {
	Outcome               string                   `json:"outcome"`
	Dependencies          []string                 `json:"dependencies"`
	ID                    string                   `json:"id"`
	Consumer              string                   `json:"consumer"`
	InstanceID            string                   `json:"instance_id"`
	Function              string                   `json:"function"`
	ModeBefore            string                   `json:"mode_before"`
	ModeAfter             string                   `json:"mode_after"`
	OwnerBefore           string                   `json:"owner_before"`
	OwnerAfter            string                   `json:"owner_after"`
	Before                capability.SecureBinding `json:"before"`
	After                 capability.SecureBinding `json:"after"`
	Locations             []ResourceLocation       `json:"locations"`
	SynchronizationBefore string                   `json:"synchronization_before"`
	SynchronizationAfter  string                   `json:"synchronization_after"`
	RotationBefore        string                   `json:"rotation_before"`
	RotationAfter         string                   `json:"rotation_after"`
	ChangeCredentials     bool                     `json:"change_credentials"`
	Supported             bool                     `json:"supported"`
	Verified              bool                     `json:"verified"`
	Evidence              string                   `json:"evidence,omitempty"`
}

func validateConsumerTransition(a ConsumerTransition) error {
	switch a.Outcome {
	case "remains", "changed", "unavailable", "unknown":
	default:
		return fmt.Errorf("invalid consumer action outcome")
	}
	if a.ID == "" || a.Consumer == "" || a.InstanceID == "" || a.Function == "" || a.ModeBefore == "" || a.ModeAfter == "" || a.OwnerBefore == "" || a.OwnerAfter == "" {
		return fmt.Errorf("consumer action requires actual adapter, modes and owners")
	}
	if a.Verified && (!a.Supported || a.Evidence == "") {
		return fmt.Errorf("verified consumer action requires supported operation and evidence")
	}
	if err := a.Before.Validate(); err != nil {
		return fmt.Errorf("invalid consumer before references")
	}
	if err := a.After.Validate(); err != nil {
		return fmt.Errorf("invalid consumer after references")
	}
	for _, l := range a.Locations {
		if err := validateLocation(l); err != nil {
			return err
		}
	}
	if !a.ChangeCredentials && !reflect.DeepEqual(a.Before.Credentials, a.After.Credentials) {
		return fmt.Errorf("consumer action must preserve credential references unless change explicitly selected")
	}
	return nil
}

func validateConsumerTransitions(o ProviderImpactObservation) error {
	if err := validateExpiry(o.ObservedAt); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, a := range o.ConsumerActions {
		if seen[a.ID] {
			return fmt.Errorf("duplicate consumer action")
		}
		seen[a.ID] = true
		if err := validateConsumerTransition(a); err != nil {
			return err
		}
	}
	return nil
}

func validateSelectedConsumerActions(s ResolutionSnapshot, requests []ProviderChangeRequest, o ProviderImpactObservation) error {
	for _, r := range requests {
		seen := map[string]bool{}
		functions := map[string]bool{}
		for _, id := range r.ConsumerActions {
			found := false
			for _, a := range o.ConsumerActions {
				if a.ID == id && dependencyPath(s, a.InstanceID, r.InstanceID, map[string]bool{}) != nil && (len(r.Consumers) == 0 || containsString(r.Consumers, a.Consumer)) {
					key := a.Consumer + "/" + a.InstanceID + "/" + a.Function
					if functions[key] {
						return &ProviderChangeError{Code: "conflicting-consumer-actions"}
					}
					functions[key] = true
					for _, f := range o.Functions {
						if f.Consumer == a.Consumer && f.InstanceID == a.InstanceID && f.Function == a.Function {
							found = true
						}
					}
				}
			}
			if !found || seen[id] {
				return &ProviderChangeError{Code: "invalid-selected-consumer-action"}
			}
			seen[id] = true
		}
	}
	return nil
}

func describeConsumerTransition(f *FunctionConsequence, use ProviderFunctionUse, o ProviderImpactObservation, r ProviderChangeRequest, changes map[string]ProviderChangeRequest, snapshot ResolutionSnapshot) (bool, bool) {
	known, attention := true, false
	if use.ValidUntil != "" {
		if o.ObservedAt == "" {
			known, attention = false, true
		} else {
			at, _ := time.Parse(time.RFC3339, o.ObservedAt)
			until, _ := time.Parse(time.RFC3339, use.ValidUntil)
			seconds := int64(until.Sub(at).Seconds())
			if seconds < 0 {
				seconds = 0
			}
			f.RemainingValiditySeconds = &seconds
			if !until.After(at) && interruptsProvider(r.Operation) {
				f.Outcome = "unavailable"
			}
		}
	}
	for _, a := range o.ConsumerActions {
		if !containsString(r.ConsumerActions, a.ID) || a.Consumer != use.Consumer || a.InstanceID != use.InstanceID || a.Function != use.Function {
			continue
		}
		action := a
		f.Transition = &action
		if !a.Supported || !a.Verified || a.Evidence == "" || a.Locations == nil || a.Dependencies == nil || a.SynchronizationBefore == "" || a.SynchronizationAfter == "" || a.RotationBefore == "" || a.RotationAfter == "" {
			f.Outcome = "unknown"
			known, attention = false, true
		} else {
			f.Outcome = a.Outcome
			if a.Outcome == "unknown" {
				known = false
			}
			if a.Outcome == "unknown" || a.Outcome == "unavailable" {
				attention = true
			}
		}
		for _, dep := range a.Dependencies {
			if !consumerDependencyAvailable(snapshot, changes, dep, map[string]bool{}) {
				f.Outcome = "unknown"
				known, attention = false, true
			}
		}
	}
	return known, attention
}

// Verify continuation against the SAME observed dependency graph, including
// aggregate changes. This selects no provider and modifies no union or owner.
func consumerDependencyAvailable(snapshot ResolutionSnapshot, changes map[string]ProviderChangeRequest, id string, visiting map[string]bool) bool {
	fact, exists := snapshot.Facts[id]
	if !exists || visiting[id] || fact.Target != snapshot.Target || fact.State != ProviderRunning || fact.Dependencies == nil {
		return false
	}
	if change, exists := changes[id]; exists && interruptsProvider(change.Operation) {
		return false
	}
	visiting[id] = true
	defer delete(visiting, id)
	for _, dependency := range fact.Dependencies {
		if !consumerDependencyAvailable(snapshot, changes, dependency, visiting) {
			return false
		}
	}
	return true
}

func sameConsumerTransition(expected, actual *ConsumerTransition) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	if !actual.Supported || !actual.Verified || actual.Evidence == "" {
		return false
	}
	a, b := *expected, *actual
	// Execution supplies fresh evidence, while all selected before/after effects
	// and secure references must match the approved plan.
	a.Evidence, b.Evidence = "", ""
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
