package application

import (
	"encoding/json"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// providerStateInvalid is the authority sentinel for a realization that ASSERTS
// a concrete runtime state but could not actually VERIFY it (Verified=false or
// empty Evidence): the adapters saw material but proved no state. It is a
// distinct authority class — never equal to a proven or unknown state — so the
// fail-closed stale-impact-consent path fires (0428-119 r2, verdict
// 6997135020).
const providerStateInvalid ProviderState = "\x00invalid"

// ConsentAuthority is the canonical equivalence surface of the consent
// identity. The PlanID digest binds CURRENT AUTHORITY: registry content,
// ownership, dependency graphs, bindings, material inventory, actual consumer
// functions, and absolute validity boundaries. It must not be invalidated by
// transient observation artifacts: the wall-clock sample stamp, the re-derived
// observation revision, or the unverifiable/verified runtime CLASSIFICATION of
// a provider whose underlying facts did not change (hunt 916 A-F1/B2: a failed
// docker/podman inspect folds into the unverifiable Layer-1 default, the
// post-consent revalidation computes a different PlanID while the classifier
// flaps, and the apply livelocks on stale-impact-consent).
//
// The surface splits in two:
//   - StableCore: everything that changes consequences; ANY difference here
//     keeps the existing fail-closed stale-impact-consent behavior, including
//     a real runtime/registry/dependency/inventory/expiry change.
//   - Samples: volatile observation surface (the plan's knowledge fold, the
//     per-provider fact classification). Differences here do NOT invalidate an
//     informed consent while the StableCore is identical.
//
// The raw PlanID remains the acknowledgement identity and digest of the
// displayed plan (machine revisions, receipts and wire contracts keep their
// exact-string semantics); revalidation compares the authority surfaces.
type ConsentAuthority struct {
	// StableCore carries the authority: the canonicalized registry, fact,
	// consumer, binding and ancillary INPUTS the plan was computed from,
	// folded to the observation-proven state classes, plus the consequence
	// projection (Body). Two observations of an unchanged system freeze to
	// identical cores even when their plan ids differ under transient churn;
	// ANY genuine authority difference keeps the existing fail-closed
	// stale-impact-consent behavior.
	StableCore consentAuthorityCore
	// Samples carries the volatile observation surface: the plan's knowledge
	// fold, inventory knowledge and the per-provider fact classification. It
	// may differ between two observations of an unchanged system without
	// invalidating consent, and is never marshaled into the comparison.
	Samples ObservationSamples
}

// ObservationSamples is the volatile observation surface of a plan: fields
// that may differ between two observations of an UNCHANGED system (the plan
// knowledge fold, the per-provider fact classification, the inventory
// knowledge) but which never carry consequence authority.
type ObservationSamples = consentAuthoritySamples

type consentAuthorityCore struct {
	Target                string
	ConfirmationsRequired int
	Destructive           bool
	// Impacts is the canonicalized consequence projection of the plan body:
	// countdown samples stripped (stableValidityImpact), the Layer-1
	// classification replaced by the observation-proven state, and the two
	// classification-derived notices folded out. Authority changes (real
	// state, registry, dependencies, inventory evidence, validity) always
	// move at least one field here, keeping the comparison fail-closed.
	Impacts   []consentAuthorityImpact
	Registry  capability.Registry
	Facts     map[string]consentAuthorityFact
	Consumers map[string]consentAuthorityConsumer
	Required  []string
	Released  []capability.ProviderBinding
	Activity  map[string]bool
	Ancillary []capability.ProviderInstance
}

// consentAuthoritySamples is the volatile observation surface. The derived
// observation revision and the wall-clock sample stamp are deliberately ABSENT
// (never consent authority).
type consentAuthoritySamples struct {
	// Complete is the plan's knowledge fold; it mixes the volatile
	// classification outcome and is therefore a sample, not authority.
	Complete bool
	// InventoryComplete is the observation's material-inventory knowledge.
	// It tracks genuine inventory changes (an adapter that can no longer
	// enumerate materials), not the runtime classifier; it is kept out of the
	// core so a classification flip — which never changes it — is core-equal,
	// while the fail-closed staleness for a real inventory change is asserted
	// through the consequences it produces in the core.
	InventoryComplete bool
}

type consentAuthorityImpact struct {
	Prerequisites      []ProviderPrerequisite
	RequestedConsumers []string
	InstanceID         string
	Operation          string
	Scenario           string
	Selection          string
	Before             consentAuthorityRealization
	After              *consentAuthorityRealization
	Dependencies       []string
	Dependents         []string
	Notices            []string
	Effects            []OperationalConsequence
	Functions          []FunctionConsequence
	Materials          []MaterialConsequence
}

// consentAuthorityRealization carries the AUTHORITATIVE state view of a
// realization. Under the Layer-1 default (app_core_requirement.go:126-128,
// application_shared_backend_projection.go:23-26) an unavailable runtime
// observation projects fact.State = unverifiable while the observation record
// still carries the concrete runtime state the adapters saw; a verified
// realization mirrors the fact exactly (provider_change_validation.go:219).
// The authority state is therefore the last concrete state PROVEN by an
// observation record, and only a provider with NO record at all keeps the
// unknown marker. A verified <-> unverifiable flip with unchanged records
// leaves this surface identical; a real change flips State itself and keeps
// staleness. A realization that ASSERTS a concrete state but could not VERIFY
// it carries the INVALID marker: it is never equal to a proven or unknown
// state, so an unwitnessed running runtime is never treated as equivalent to
// a verified one (0428-119 r2).
type consentAuthorityRealization struct {
	Ancillary  bool
	InstanceID string
	Kind       capability.ProviderKind
	Target     string
	Scope      capability.ProviderScope
	Ownership  capability.ProviderOwnership
	Owner      string
	State      ProviderState
	// Invalid marks an asserted-but-unverified realization: the adapters saw
	// material but proved no state. It is a distinct authority class.
	Invalid   bool
	Locations []ResourceLocation
	Access    *ProviderAccess
	Supported bool
}

// consentAuthorityFact is the canonicalized, observation-folded fact view of
// the digest input (provider_change_digest.go). The runtime State is the
// Layer-1 PROJECTION: a fact that says unverifiable carries no concrete state
// (its raw classification stays in the samples surface); a concrete state must
// be confirmed by an observation record or a genuinely changed fact, so a
// verified <-> unverifiable classification flip is core-equal while a real
// runtime change flips State. Nil versus empty dependency inventories stay
// distinguishable; sampling counters never enter (provider_change_digest.go
// excludes RAM/container counters by design).
type consentAuthorityFact struct {
	Ancillary       bool
	OwnerReference  string
	Target          string
	Core            CoreRequirement
	Dependencies    []string
	DependenciesNil bool
	ProvenState     providerAuthorityState
}

// providerAuthorityState folds a projected fact state together with the
// observation record of the same cycle into the authority class.
type providerAuthorityState struct {
	// State is the concrete state PROVEN this cycle, or "" when the cycle
	// proved nothing (unverifiable classification without an observation
	// witness, or an absent fact).
	State ProviderState
	// Absent marks a provider with no fact at all.
	Absent bool
	// Invalid marks a provider whose observation asserted a concrete runtime
	// state but could not actually VERIFY it (Verified=false or empty
	// Evidence): the cycle observed material but proved no state. It is a
	// distinct authority class that is never equal to a proven or unknown
	// state, so the fail-closed stale-impact-consent path fires (0428-119 r2).
	Invalid bool
}

type consentAuthorityConsumer struct {
	ApplicationID string
	Requirements  []RequirementResolution
}

// BuildConsentAuthority freezes the consent-authority surface of one
// computed plan from the exact inputs it was computed from. The core is
// projected from the canonicalized plan BODY and the canonicalized INPUTS so
// two observations of an unchanged system — whose volatile samples (the
// knowledge fold, the classification) may differ — always freeze equal;
// genuine differences in registry, facts, dependencies, bindings, consumers,
// inventory, evidence or consequence authority keep the surfaces unequal
// (fail-closed).
func BuildConsentAuthority(p ProviderImpactPlan, s ResolutionSnapshot, union ProviderPlan, o ProviderImpactObservation) ConsentAuthority {
	core := consentAuthorityCore{
		Target: p.Target,
		// The plan's knowledge fold is a sample; the core keeps only the
		// consequence fields that a pure classification flip cannot move.
		ConfirmationsRequired: p.ConfirmationsRequired,
		Destructive:           p.Destructive,
		Registry:              stableValidityRegistry(s.Registry),
		Facts:                 map[string]consentAuthorityFact{},
		Consumers:             map[string]consentAuthorityConsumer{},
		Required:              append([]string(nil), union.Required...),
		Released:              append([]capability.ProviderBinding(nil), union.ReleasedBindings...),
		Activity:              map[string]bool{},
	}
	samples := consentAuthoritySamples{Complete: p.Complete, InventoryComplete: o.InventoryComplete}
	// Authority state fold (same rule as consentAuthorityDigestFacts in
	// provider_change_plan.go): the Layer-1 projection "unverifiable" carries
	// no state information; a concrete observation record names the runtime
	// state the adapters saw, and only a provider with NO concrete witness
	// keeps the unknown marker. A concrete state is folded only when the
	// realization was actually VERIFIED with evidence (0428-119 r2, verdict
	// 6997135020) — an asserted-but-unverified realization folds to INVALID, so
	// the digest fold and this fold stay symmetric with no asymmetric
	// silent-success class between them.
	observed := func(id string) (ProviderState, bool) {
		f, ok := o.Providers[id]
		if !ok || f.Realization.State == ProviderUnverifiable {
			return "", false
		}
		if !f.Realization.Verified || f.Realization.Evidence == "" {
			// Asserted but not proven: fold to INVALID, never to a proven
			// running state.
			return providerStateInvalid, true
		}
		return f.Realization.State, true
	}
	for id, f := range s.Facts {
		state := f.State
		invalid := false
		if state == ProviderUnverifiable {
			if witness, has := observed(id); has {
				state = witness
				invalid = witness == providerStateInvalid
			} else {
				state = ""
			}
		}
		deps, depsNil := append([]string(nil), f.Dependencies...), f.Dependencies == nil
		core.Facts[id] = consentAuthorityFact{
			Ancillary: f.Ancillary, OwnerReference: f.OwnerReference, Target: f.Target,
			Core: f.Core, Dependencies: deps, DependenciesNil: depsNil,
			ProvenState: providerAuthorityState{State: state, Invalid: invalid},
		}
	}
	factClassification := map[string]ProviderState{}
	for id, f := range s.Facts {
		factClassification[id] = f.State
	}
	for key, fp := range union.Consumers {
		core.Consumers[key] = consentAuthorityConsumer{fp.ApplicationID, append([]RequirementResolution(nil), fp.Requirements...)}
	}
	for key, active := range union.ConsumerActivity {
		core.Activity[key] = active
	}
	core.Ancillary = append([]capability.ProviderInstance(nil), s.Ancillary...)
	sort.Slice(core.Ancillary, func(i, j int) bool { return core.Ancillary[i].ID < core.Ancillary[j].ID })
	sort.Strings(core.Required)
	provenState := func(id string) (ProviderState, bool) {
		if f, ok := core.Facts[id]; ok {
			return f.ProvenState.State, f.ProvenState.Invalid
		}
		return "", false
	}
	factState := func(id string) ProviderState {
		return factClassification[id]
	}
	for _, impact := range stableValidityImpact(p).Impacts {
		provenBefore, invalidBefore := provenState(impact.InstanceID)
		// Class (ii): an observed realization that ASSERTS a concrete state in
		// the fact but could not VERIFY it (Verified=false or empty Evidence).
		// Only the observed BEFORE realization carries this; the AFTER is the
		// operation proposal, which is never verified by construction
		// (proposedRealization).
		beforeInvalid := invalidBefore
		if factState(impact.InstanceID) != ProviderUnverifiable {
			beforeInvalid = beforeInvalid || (!impact.Before.Verified || impact.Before.Evidence == "")
		}
		ai := consentAuthorityImpact{
			Prerequisites: append([]ProviderPrerequisite(nil), impact.Prerequisites...), RequestedConsumers: append([]string(nil), impact.RequestedConsumers...),
			InstanceID: impact.InstanceID, Operation: impact.Operation, Scenario: impact.Scenario, Selection: impact.Selection,
			Before:       consentAuthorityRealizationOf(impact.Before, provenBefore, beforeInvalid),
			Dependencies: append([]string(nil), impact.Dependencies...), Dependents: append([]string(nil), impact.Dependents...),
			Notices:   classificationDerivedNotices(impact.Notices),
			Effects:   append([]OperationalConsequence(nil), impact.Effects...),
			Functions: append([]FunctionConsequence(nil), impact.Functions...), Materials: append([]MaterialConsequence(nil), impact.Materials...),
		}
		if impact.After != nil {
			proven, _ := provenState(impact.After.InstanceID)
			stable := consentAuthorityRealizationOf(*impact.After, proven, false)
			ai.After = &stable
		}
		core.Impacts = append(core.Impacts, ai)
	}
	return ConsentAuthority{StableCore: core, Samples: samples}
}

func consentAuthorityRealizationOf(r ProviderRealization, proven ProviderState, invalid bool) consentAuthorityRealization {
	// Authority state fold: an impact-before that says "unverifiable" carries
	// no state information of its own — the concrete state proven by fact or
	// record wins; a provider nothing proved at all keeps the unknown marker.
	// The caller decides the classification class and passes the INVALID flag;
	// here we only apply the proven witness to the unverifiable projection and
	// carry the caller's invalid flag onto a concrete projection.
	state := r.State
	if state == ProviderUnverifiable {
		state = proven
		if proven == "" {
			state = ""
			invalid = false
		}
	}
	return consentAuthorityRealization{
		Ancillary: r.Ancillary, InstanceID: r.InstanceID, Kind: r.Kind, Target: r.Target,
		Scope: r.Scope, Ownership: r.Ownership, Owner: r.Owner,
		State:     state,
		Invalid:   invalid,
		Locations: append([]ResourceLocation(nil), r.Locations...), Access: r.Access, Supported: r.Supported,
	}
}

// classificationDerivedNotices are the two notices the impact describer emits
// exactly when the runtime classification folded into the unverifiable Layer-1
// default (provider_change_plan.go:143-149). They are derived presentation of
// the sample, never new authority; removing them from the core comparison
// keeps every genuine consequence notice in full.
func classificationDerivedNotices(notices []string) []string {
	result := make([]string, 0, len(notices))
	for _, n := range notices {
		switch n {
		case "material inventory is incomplete; unlisted material is not claimed preserved or deleted",
			"operational guarantees have not been verified":
			continue
		}
		result = append(result, n)
	}
	return result
}

// equivalentProvenState compares two folded fact states under the three-class
// rule: UNKNOWN (State == "", proven but nothing asserted) contradicts nothing
// and equals any other authority state; INVALID (asserted-but-unverified) is a
// distinct class equal only to another INVALID; a proven concrete state equals
// only the same concrete state. This is what makes a genuinely-unverifiable
// classifier flap (class i) revalidate equal to a verified one (class iii)
// while an asserted-but-unverified realization (class ii) stays fail-closed.
func equivalentProvenState(a, b providerAuthorityState) bool {
	aUnknown := a.State == "" && !a.Invalid
	bUnknown := b.State == "" && !b.Invalid
	if aUnknown || bUnknown {
		return true
	}
	return a.State == b.State && a.Invalid == b.Invalid
}

// equivalentRealization compares two folded realization states with the same
// three-class rule: an UNKNOWN realization (Invalid false, State "") is
// neutral and equals any authority; an INVALID realization equals only another
// INVALID; a proven realization equals only the same proven state.
func equivalentRealization(a, b consentAuthorityRealization) bool {
	aUnknown := a.State == "" && !a.Invalid
	bUnknown := b.State == "" && !b.Invalid
	if aUnknown || bUnknown {
		return true
	}
	return a.State == b.State && a.Invalid == b.Invalid
}

// Equivalent reports whether two authority surfaces carry the same consent
// authority: the StableCore and the frozen Body decide; only the Samples
// surface may differ. UNKNOWN folded states (class i, nothing asserted) are
// neutral and equal to any authority; INVALID (class ii, asserted but unverified)
// stays a distinct fail-closed class; proven concrete states compare exactly.
func (a ConsentAuthority) Equivalent(b ConsentAuthority) (bool, error) {
	if a.StableCore.Target != b.StableCore.Target {
		return false, nil
	}
	if a.StableCore.ConfirmationsRequired != b.StableCore.ConfirmationsRequired ||
		a.StableCore.Destructive != b.StableCore.Destructive {
		return false, nil
	}
	if !equalRegistry(a.StableCore.Registry, b.StableCore.Registry) {
		return false, nil
	}
	if len(a.StableCore.Impacts) != len(b.StableCore.Impacts) {
		return false, nil
	}
	if !equalStringSlices(a.StableCore.Required, b.StableCore.Required) {
		return false, nil
	}
	if len(a.StableCore.Ancillary) != len(b.StableCore.Ancillary) {
		return false, nil
	}
	for id, af := range a.StableCore.Facts {
		bf, ok := b.StableCore.Facts[id]
		if !ok {
			return false, nil
		}
		if af.Ancillary != bf.Ancillary || af.OwnerReference != bf.OwnerReference ||
			af.Target != bf.Target || af.Core != bf.Core || af.DependenciesNil != bf.DependenciesNil ||
			!equalStringSlices(af.Dependencies, bf.Dependencies) {
			return false, nil
		}
		if !equivalentProvenState(af.ProvenState, bf.ProvenState) {
			return false, nil
		}
	}
	for key, ac := range a.StableCore.Consumers {
		bc, ok := b.StableCore.Consumers[key]
		if !ok || ac.ApplicationID != bc.ApplicationID ||
			!equalRequirementResolutions(ac.Requirements, bc.Requirements) {
			return false, nil
		}
	}
	for key, av := range a.StableCore.Activity {
		if bv, ok := b.StableCore.Activity[key]; !ok || av != bv {
			return false, nil
		}
	}
	for i, ai := range a.StableCore.Impacts {
		bi := b.StableCore.Impacts[i]
		if !equivalentImpact(ai, bi) {
			return false, nil
		}
	}
	return true, nil
}

// equalRegistry compares two provider registries by CONTENT, not by count.
// A provider-change that keeps the same number of instances/bindings but
// swaps, retargets or retitles one of them must NOT read as consent-equivalent:
// the stale entry has to block the new request. Order is irrelevant (the
// registry is a set keyed by instance id / binding tuple). Empty registries
// are equal; an empty registry and a non-empty one are not.
func equalRegistry(a, b capability.Registry) bool {
	if !equalInstanceSet(a.Instances, b.Instances) {
		return false
	}
	if !equalBindingSet(a.Bindings, b.Bindings) {
		return false
	}
	return true
}

// equalInstanceSet reports whether two instance slices carry the same set of
// instances, keyed by id. Two instances are equal only when every field that
// the registry carries — the identity (id, provider_id, provider_version,
// provider_protocol), the provider (kind) plus scope, sharing boundary,
// ownership and owner — is identical. A differing version, scope, owner or
// provider kind for the same id is a real registry change.
func equalInstanceSet(a, b []capability.ProviderInstance) bool {
	if len(a) != len(b) {
		return false
	}
	bIndex := make(map[string]struct{}, len(b))
	for _, bi := range b {
		bIndex[bi.ID] = struct{}{}
	}
	for _, ai := range a {
		if _, ok := bIndex[ai.ID]; !ok {
			return false
		}
		if !sameProviderInstanceAuthority(ai, b[findInstanceIndex(b, ai.ID)]) {
			return false
		}
	}
	return true
}

// equalBindingSet reports whether two binding slices carry the same set of
// bindings, keyed by the full binding tuple. A binding retargeted to another
// provider instance (or resource/environment) is a real registry change.
func equalBindingSet(a, b []capability.ProviderBinding) bool {
	if len(a) != len(b) {
		return false
	}
	bIndex := make(map[string]struct{}, len(b))
	for _, bi := range b {
		bIndex[bindingKey(bi)] = struct{}{}
	}
	for _, ai := range a {
		if _, ok := bIndex[bindingKey(ai)]; !ok {
			return false
		}
		if !sameBindingAuthority(ai, b[findBindingIndex(b, ai)]) {
			return false
		}
	}
	return true
}

// sameProviderInstanceAuthority reports whether two instances of the SAME id
// carry identical consent authority: id, provider identity and version,
// protocol, scope, sharing boundary, ownership and owner, the external
// registration Reference and the owner-environment constraint. The registry
// already keys instances by id, so this is the content the stale-impact-consent
// fold must read.
func sameProviderInstanceAuthority(a, b capability.ProviderInstance) bool {
	return a.ID == b.ID &&
		a.ProviderID == b.ProviderID &&
		a.ProviderVersion == b.ProviderVersion &&
		a.ProviderProtocol == b.ProviderProtocol &&
		a.Provider.Kind == b.Provider.Kind &&
		equalProviderCapabilities(a.Provider.Capabilities, b.Provider.Capabilities) &&
		a.Scope == b.Scope &&
		a.SharingBoundary == b.SharingBoundary &&
		a.Ownership == b.Ownership &&
		a.OwnerApplicationID == b.OwnerApplicationID &&
		a.Reference == b.Reference &&
		a.OwnerEnvironment == b.OwnerEnvironment
}

// sameBindingAuthority reports whether two bindings of the SAME tuple carry
// identical consent authority: application, resource, environment and the
// provider instance they bind to.
func sameBindingAuthority(a, b capability.ProviderBinding) bool {
	return a.ApplicationID == b.ApplicationID &&
		a.Resource == b.Resource &&
		a.Environment == b.Environment &&
		a.ProviderInstanceID == b.ProviderInstanceID
}

func findInstanceIndex(list []capability.ProviderInstance, id string) int {
	for i, v := range list {
		if v.ID == id {
			return i
		}
	}
	return -1
}

func findBindingIndex(list []capability.ProviderBinding, want capability.ProviderBinding) int {
	for i, v := range list {
		if bindingKey(v) == bindingKey(want) {
			return i
		}
	}
	return -1
}

func bindingKey(b capability.ProviderBinding) string {
	return b.ApplicationID + "\x00" +
		string(b.Resource.Kind) + "\x00" +
		b.Resource.Name + "\x00" +
		b.Environment + "\x00" +
		b.ProviderInstanceID
}

// equalProviderCapabilities reports whether two capability lists carry the same
// multiset of capabilities (order irrelevant): a provider that lost or gained a
// capability, or duplicated one to mask a missing capability, is a different
// provider for consent purposes. The comparison is by per-kind count, not by
// length-plus-membership: the latter treated [sql, sql] and [sql, secrets] as
// equal because both lengths are two and every left element is present in the
// right set. A multiset match is required because a duplicate entry can hide a
// capability the provider no longer supports.
func equalProviderCapabilities(a, b []capability.Kind) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[capability.Kind]int, len(b))
	for _, c := range b {
		counts[c]++
	}
	for _, c := range a {
		counts[c]--
		if counts[c] < 0 {
			return false
		}
	}
	return true
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalRequirementResolutions(a, b []RequirementResolution) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].InstanceID != b[i].InstanceID || a[i].State != b[i].State || a[i].Requirement != b[i].Requirement {
			return false
		}
	}
	return true
}

func equivalentImpact(a, b consentAuthorityImpact) bool {
	if !equalPrerequisites(a.Prerequisites, b.Prerequisites) ||
		!equalStringSlices(a.RequestedConsumers, b.RequestedConsumers) ||
		a.InstanceID != b.InstanceID || a.Operation != b.Operation ||
		a.Scenario != b.Scenario || a.Selection != b.Selection ||
		!equalStringSlices(a.Dependencies, b.Dependencies) ||
		!equalStringSlices(a.Dependents, b.Dependents) ||
		!equalStringSlices(a.Notices, b.Notices) ||
		!equalOperationalConsequences(a.Effects, b.Effects) ||
		!equalFunctionConsequences(a.Functions, b.Functions) ||
		!equalMaterialConsequences(a.Materials, b.Materials) {
		return false
	}
	return equivalentRealization(a.Before, b.Before)
}

func equalPrerequisites(a, b []ProviderPrerequisite) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa, _ := json.Marshal(a[i])
		bb, _ := json.Marshal(b[i])
		if string(aa) != string(bb) {
			return false
		}
	}
	return true
}

func equalOperationalConsequences(a, b []OperationalConsequence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa, _ := json.Marshal(a[i])
		bb, _ := json.Marshal(b[i])
		if string(aa) != string(bb) {
			return false
		}
	}
	return true
}

func equalFunctionConsequences(a, b []FunctionConsequence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa, _ := json.Marshal(a[i])
		bb, _ := json.Marshal(b[i])
		if string(aa) != string(bb) {
			return false
		}
	}
	return true
}

func equalMaterialConsequences(a, b []MaterialConsequence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aa, _ := json.Marshal(a[i])
		bb, _ := json.Marshal(b[i])
		if string(aa) != string(bb) {
			return false
		}
	}
	return true
}

// ProviderImpactPlansConsentEquivalent reports whether two independently
// computed impact plans carry the SAME consent authority. Callers that already
// froze authority surfaces inside their plans (DescribeProviderChanges sets
// Plan.Authority) are compared on those frozen surfaces; plans without a
// frozen surface fall back to building the surface from the caller-supplied
// inputs. The stabilization rule of the post-consent revalidation: countdown
// samples, the transient unverifiable/verified runtime classification over an
// unchanged system, the re-derived observation revision and the wall-clock
// sample do NOT make an otherwise unchanged mutation plan stale; registry,
// ownership, dependency, binding, inventory, function, absolute-validity and
// real-state changes keep the plans unequal so the existing fail-closed
// stale-impact-consent path still fires.
func ProviderImpactPlansConsentEquivalent(a, b ProviderImpactPlan, sa, sb ResolutionSnapshot, ua, ub ProviderPlan, oa, ob ProviderImpactObservation) (bool, error) {
	if a.Authority != nil && b.Authority != nil {
		return a.Authority.Equivalent(*b.Authority)
	}
	return BuildConsentAuthority(a, sa, ua, oa).Equivalent(BuildConsentAuthority(b, sb, ub, ob))
}

// ValidateProviderImpactRevalidation compares the plan shown for consent
// against the plan the post-consent revalidation computed from fresh inputs.
// An exact plan-id match is the ordinary path: the plan id is the digest of
// the consent authority, so volatile observation churn (classification flip
// of unchanged material, re-derived revision, clock sample) keeps the ids
// equal by construction. When the ids differ, the frozen authority surfaces
// provide the second gate before staleness fires: an unchanged underlying
// authority (display-path differences only) keeps the informed
// acknowledgement valid; any genuine authority difference — and any plan
// that does not carry its frozen surface — stays fail-closed.
func ValidateProviderImpactRevalidation(current, planned ProviderImpactPlan) error {
	if planned.PlanID != "" && current.PlanID == planned.PlanID {
		return nil
	}
	if current.Authority != nil && planned.Authority != nil {
		// Both surfaces were frozen by DescribeProviderChanges from the exact
		// inputs of their computation; they decide the second gate.
		equivalent, err := current.Authority.Equivalent(*planned.Authority)
		if err != nil {
			return err
		}
		if !equivalent {
			return &ProviderChangeError{Code: "stale-impact-consent"}
		}
		return nil
	}
	// A plan that does not carry its frozen surface (hand-built, or computed
	// by an older describer) keeps the fail-closed raw plan-id rule.
	return &ProviderChangeError{Code: "stale-impact-consent"}
}

// stableValidityRegistry canonicalizes registry traversal order for the
// authority surface.
func stableValidityRegistry(r capability.Registry) capability.Registry {
	stable := capability.Registry{
		Instances: append([]capability.ProviderInstance(nil), r.Instances...),
		Bindings:  append([]capability.ProviderBinding(nil), r.Bindings...),
	}
	sort.Slice(stable.Instances, func(i, j int) bool { return stable.Instances[i].ID < stable.Instances[j].ID })
	sort.Slice(stable.Bindings, func(i, j int) bool {
		a, _ := json.Marshal(stable.Bindings[i])
		b, _ := json.Marshal(stable.Bindings[j])
		return string(a) < string(b)
	})
	return stable
}
