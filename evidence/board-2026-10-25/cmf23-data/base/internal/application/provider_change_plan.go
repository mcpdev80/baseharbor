package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// DescribeProviderChanges enriches the EXISTING resolved union. It does not
// select providers, persist observations or schedule changes. Explicit requests
// can intentionally stop needed functionality; consequences require consent,
// not an automatic continuation-policy veto.
func DescribeProviderChanges(snapshot ResolutionSnapshot, union ProviderPlan, requests []ProviderChangeRequest, observation ProviderImpactObservation) (ProviderImpactPlan, error) {
	p := ProviderImpactPlan{Version: ProviderImpactVersion, Target: snapshot.Target, Revision: observation.Revision,
		Complete: union.Complete && observation.InventoryComplete, ConfirmationsRequired: 1, Impacts: []ProviderImpact{}}
	if snapshot.Target == "" || union.Target != snapshot.Target || union.Version != ProviderPlanVersion {
		return p, &ProviderChangeError{Code: "invalid-provider-union"}
	}
	if err := snapshot.Registry.Validate(); err != nil {
		return p, err
	}
	if err := validateImpactObservation(observation); err != nil {
		return p, err
	}
	instances, err := indexResolutionInstances(snapshot)
	if err != nil {
		return p, err
	}
	if err := validateObservationOwners(snapshot, observation); err != nil {
		return p, err
	}
	ordered := append([]ProviderChangeRequest(nil), requests...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].InstanceID < ordered[j].InstanceID })
	changes := map[string]ProviderChangeRequest{}
	for _, r := range ordered {
		if _, duplicate := changes[r.InstanceID]; duplicate {
			return p, &ProviderChangeError{Code: "conflicting-provider-changes"}
		}
		if err := validateChangeRequest(r); err != nil {
			return p, err
		}
		if _, exists := instances[r.InstanceID]; !exists {
			return p, &ProviderChangeError{Code: "provider-instance-missing"}
		}
		if err := validateRequestStrings(r); err != nil {
			return p, err
		}
		changes[r.InstanceID] = r
	}
	if err := validateSelectedConsumerActions(snapshot, ordered, observation); err != nil {
		return p, err
	}
	uses := observedFunctionUses(snapshot, union, observation.Functions)
	for _, r := range ordered {
		impact, known, destructive, attention, err := describeProviderChange(snapshot, union, observation, instances, changes, uses, r)
		if err != nil {
			return p, err
		}
		impact.ContinuationDependencies = describeContinuationDependencies(snapshot, observation, r)
		p.Impacts = append(p.Impacts, impact)
		p.Complete = p.Complete && known
		p.Destructive = p.Destructive || destructive
		if attention {
			p.Attention = "Achtung"
			p.ConfirmationsRequired = 2
		}
	}
	if !p.Complete || p.Destructive {
		p.Attention = "Achtung"
		p.ConfirmationsRequired = 2
	}
	// Hash all relevant authoritative state, including ownership, binding and
	// dependency changes that may not alter a displayed provider count.
	canonical := snapshot.Registry
	canonical.Instances = append([]capability.ProviderInstance(nil), canonical.Instances...)
	canonical.Bindings = append([]capability.ProviderBinding(nil), canonical.Bindings...)
	sort.Slice(canonical.Instances, func(i, j int) bool { return canonical.Instances[i].ID < canonical.Instances[j].ID })
	sort.Slice(canonical.Bindings, func(i, j int) bool {
		a, _ := json.Marshal(canonical.Bindings[i])
		b, _ := json.Marshal(canonical.Bindings[j])
		return string(a) < string(b)
	})
	raw, err := impactDigestInput(p, canonical, snapshot, union)
	if err != nil {
		return p, err
	}
	hash := sha256.Sum256(raw)
	p.PlanID = hex.EncodeToString(hash[:])
	return p, nil
}

func describeProviderChange(s ResolutionSnapshot, union ProviderPlan, o ProviderImpactObservation, instances map[string]capability.ProviderInstance, changes map[string]ProviderChangeRequest, uses []ProviderFunctionUse, r ProviderChangeRequest) (ProviderImpact, bool, bool, bool, error) {
	instance := instances[r.InstanceID]
	before, known := observedRealization(instance, s, o)
	if fact, ok := s.Facts[instance.ID]; ok && (fact.State == ProviderForeign || fact.Target != s.Target) {
		return ProviderImpact{}, false, false, false, &ProviderChangeError{Code: "foreign-provider-target"}
	}
	if instance.Ownership == capability.OwnershipExternal && r.Operation != "unbind" && r.Operation != "disable" && r.Operation != "replace" && r.Operation != "rebind" {
		return ProviderImpact{}, false, false, false, &ProviderChangeError{Code: "external-provider-runtime-not-owned"}
	}
	imp := ProviderImpact{RequestedConsumers: append([]string{}, r.Consumers...), InstanceID: r.InstanceID, Operation: r.Operation, Scenario: "provider-changed", Selection: r.Selection, Before: before,
		Dependencies: appendDependencies(s.Facts[instance.ID].Dependencies), Dependents: []string{}, Functions: []FunctionConsequence{}, Materials: []MaterialConsequence{}, Effects: []OperationalConsequence{}, Notices: []string{}}
	if imp.Selection == "" {
		imp.Selection = "automatic"
	}
	if _, ok := (capability.IntegrationDescriptor{Provider: instance.Provider}).ChangeContract(); !ok {
		known = false
		imp.Notices = append(imp.Notices, "provider change metadata is unknown")
	}
	fact, observed := o.Providers[instance.ID]
	known = known && observed && fact.InventoryComplete && s.Facts[instance.ID].Dependencies != nil
	imp.Effects = append(imp.Effects, fact.Effects...)
	imp.Prerequisites = append([]ProviderPrerequisite{}, fact.Prerequisites...)
	if !observed || !fact.InventoryComplete {
		imp.Notices = append(imp.Notices, "material inventory is incomplete; unlisted material is not claimed preserved or deleted")
	}
	if fact.Effects == nil {
		known = false
		imp.Notices = append(imp.Notices, "operational guarantees have not been verified")
	}
	imp.After = proposedRealization(before, r.Operation)
	replacementReady := false
	if r.ReplacementID != "" {
		replacement, exists := instances[r.ReplacementID]
		if !exists {
			return imp, false, false, false, &ProviderChangeError{Code: "replacement-provider-missing"}
		}
		after, verified := observedRealization(replacement, s, o)
		replacementReady = verified && consumerDependencyAvailable(s, changes, o, replacement.ID, map[string]bool{})
		for _, fp := range union.Consumers {
			for _, req := range fp.Requirements {
				if req.InstanceID == instance.ID && !replacement.Provider.Supports(req.Requirement.Kind) {
					replacementReady = false
				}
			}
		}
		for _, use := range uses {
			if use.InstanceID == instance.ID && !knownProviderFunction(replacement.Provider, use.Function) {
				replacementReady = false
			}
		}
		for _, b := range s.Registry.BindingsForProviderInstance(instance.ID) {
			if !replacement.Provider.Supports(b.Resource.Kind) {
				replacementReady = false
			}
		}
		imp.After = &after
		imp.Scenario = "provider-replaced"
		if !replacementReady {
			known = false
			imp.Notices = append(imp.Notices, "replacement continuation is not verified; requested change may lose functionality")
		}
	}
	attention := false
	for _, e := range imp.Effects {
		if e.Evidence == "" {
			known = false
			attention = true
		}
	}
	for _, q := range fact.Prerequisites {
		if !q.Supported || !q.Verified || q.Evidence == "" {
			known = false
			attention = true
		}
	}
	for _, use := range uses {
		via := dependencyPath(s, use.InstanceID, instance.ID, map[string]bool{})
		if via == nil {
			continue
		}
		if (r.Operation == "unbind" || r.Operation == "disable" || r.Operation == "rebind") && len(r.Consumers) > 0 && !containsString(r.Consumers, use.Consumer) {
			continue
		}
		if r.ReplacementID != "" {
			verified := verifiedFunctionContinuation(o, use, r.ReplacementID)
			imp.Prerequisites = append(imp.Prerequisites, ProviderPrerequisite{Kind: "consumer-rebind", Subject: use.Consumer + "/" + use.Function, Supported: verified, Verified: verified, Consequence: "without verified rebind, consumer continuity is not claimed"})
		}
		f := FunctionConsequence{Consumer: use.Consumer, Function: use.Function, Via: via, Required: use.Required, Outcome: "remains", ValidUntil: use.ValidUntil, Behavior: use.Behavior, Evidence: use.Evidence}
		if interruptsProvider(r.Operation) {
			f.Outcome = "unavailable"
			if replacementReady && len(via) == 1 && verifiedFunctionContinuation(o, use, r.ReplacementID) {
				f.Outcome = "changed"
			}
			// A replacement of a database cannot claim unchanged dependent Identity or
			// broker access without actual handoff/reconnection evidence.
			if replacementReady && len(via) > 1 {
				f.Outcome = "unknown"
				known = false
			}
			if f.Outcome == "unavailable" || f.Outcome == "unknown" {
				attention = true
			}
		} else if r.Operation == "update" || r.Operation == "rebind" || r.Operation == "resume" || r.Operation == "add" {
			f.Outcome = "changed"
		}
		if interruptsProvider(r.Operation) && (use.Function == "identity.validate" || use.Function == "pki.validate") && use.Behavior == "offline-until-expiry" && use.ValidUntil != "" {
			f.Outcome = "changed"
			attention = true
		}
		transitionKnown, transitionAttention := describeConsumerTransition(&f, use, o, r, changes, s)
		known = known && transitionKnown
		attention = attention || transitionAttention
		if !knownProviderFunction(instances[use.InstanceID].Provider, use.Function) {
			known = false
			imp.Notices = append(imp.Notices, "actual function has no declared provider consequence contract")
		}
		if use.Evidence == "" {
			known = false
		}
		imp.Functions = append(imp.Functions, f)
		if len(via) > 1 && !containsString(imp.Dependents, use.InstanceID) {
			imp.Dependents = append(imp.Dependents, use.InstanceID)
		}
	}
	if r.ReplacementID == "" && interruptsProvider(r.Operation) {
		imp.Scenario = "capability-no-longer-requested"
		for _, f := range imp.Functions {
			if f.Required {
				imp.Scenario = "provider-removed-while-required"
				break
			}
		}
		if len(imp.Functions) == 0 {
			imp.Scenario = "unused-provider"
		}
	}
	if r.Operation == "unbind" || r.Operation == "disable" {
		for _, use := range uses {
			if use.InstanceID == r.InstanceID && len(r.Consumers) > 0 && !containsString(r.Consumers, use.Consumer) {
				imp.Scenario = "provider-still-used"
			}
		}
	}
	materialKnown, destructive, materialAttention, err := describeMaterials(&imp, r, fact, o.ObservedAt)
	if err != nil {
		return imp, false, false, false, err
	}
	known = known && materialKnown
	attention = attention || materialAttention || r.Operation == "destroy"
	if r.Operation == "stop" {
		imp.Notices = append(imp.Notices, "temporary stop; retained storage is recovery, not a running delivery endpoint")
	}
	if interruptsProvider(r.Operation) {
		for _, f := range imp.Functions {
			switch f.Function {
			case "identity.validate", "identity.session", "identity.jwks", "identity.refresh", "pki.validate":
				imp.Notices = append(imp.Notices, "existing token/certificate/session validity follows the observed consumer behavior and expiry; future issuance/renewal or refresh is a separate function")
			case "credentials.issue", "credentials.renew", "credentials.revoke":
				imp.Notices = append(imp.Notices, "dynamic credentials are lease-bound; stopping their issuer does not prove downstream validity or successful revocation")
			}
		}
	}
	sort.Strings(imp.Dependents)
	sort.Slice(imp.Functions, func(i, j int) bool {
		a, b := imp.Functions[i], imp.Functions[j]
		return a.Consumer+"/"+a.Function+"/"+a.Via[0] < b.Consumer+"/"+b.Function+"/"+b.Via[0]
	})
	sort.Slice(imp.Materials, func(i, j int) bool { return imp.Materials[i].Before.ID < imp.Materials[j].Before.ID })
	return imp, known, destructive, attention, nil
}

func observedRealization(i capability.ProviderInstance, s ResolutionSnapshot, o ProviderImpactObservation) (ProviderRealization, bool) {
	f, ok := o.Providers[i.ID]
	if !ok {
		return ProviderRealization{Ancillary: s.Facts[i.ID].Ancillary, InstanceID: i.ID, Kind: i.Provider.Kind, Target: s.Target, Scope: i.Scope, Ownership: i.Ownership, Owner: observationOwner(i, s), State: s.Facts[i.ID].State, Locations: []ResourceLocation{}}, false
	}
	return f.Realization, f.Realization.Verified && f.Realization.Evidence != ""
}

func proposedRealization(before ProviderRealization, op string) *ProviderRealization {
	after := before
	// A proposal never asserts execution verification.
	after.Verified = false
	after.Evidence = ""
	switch op {
	case "destroy", "remove-runtime":
		return nil
	case "stop":
		after.State = ProviderStoppedOwned
		after.Access = nil
	case "resume", "add":
		after.State = ProviderRunning
	}
	return &after
}

func interruptsProvider(op string) bool {
	switch op {
	case "stop", "destroy", "remove-runtime", "replace", "rebind", "disable", "unbind":
		return true
	}
	return false
}
func containsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func validateChangeRequest(r ProviderChangeRequest) error {
	if r.InstanceID == "" {
		return &ProviderChangeError{Code: "provider-instance-missing"}
	}
	switch r.Operation {
	case "add", "resume", "stop", "destroy", "remove-runtime", "replace", "update", "rebind", "disable", "unbind":
	default:
		return &ProviderChangeError{Code: "unsupported-change-operation"}
	}
	if r.Selection != "" && r.Selection != "automatic" && r.Selection != "explicit" {
		return fmt.Errorf("invalid provider selection mode")
	}
	if r.Operation == "replace" && r.ReplacementID == "" {
		return fmt.Errorf("replacement instance is required")
	}
	if r.ReplacementID == r.InstanceID {
		return fmt.Errorf("replacement must be a different instance")
	}
	return nil
}
